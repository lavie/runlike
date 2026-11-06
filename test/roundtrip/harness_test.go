// Package roundtrip checks that runlike's output recreates a container:
// create A, run runlike on A, create B from that argv, compare the
// normalized `docker container inspect` of A and B. See README.md.
package roundtrip

import (
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lavie/runlike/v2"
	dockercli "github.com/lavie/runlike/v2/internal/daemon"
)

// Case is one live round trip. Strings in Args, Setup and Teardown may use
// {p}, replaced by the case's unique prefix rt-<area>-<case>-<random>;
// name networks and volumes {p}-something so the sweeper finds them.
// AfterA, BeforeB and Teardown may also use {imageA}, A's image ID.
// {blk} is a whole block device of the daemon's host (see blockDevice);
// a case that uses it skips when there is none.
type Case struct {
	Name     string
	Args     []string   // after "docker create": options, image, command
	Run      bool       // "docker run -d" instead of "docker create" (start matters)
	Setup    [][]string // docker commands (without "docker") run before A
	AfterA   [][]string // docker commands run after A, before runlike (move a tag, delete an image)
	BeforeB  [][]string // docker commands run after runlike, before the copies
	Teardown [][]string // docker commands run after the containers are gone
	Warn     []string   // substrings each of some runlike warning
	Ignore   []Ignore   // extra fields to ignore for this case only
	Opts     runlike.Options
	OS       string // daemon OS the case runs on: "" (linux) or "windows"
	// Refused: when creating A fails with this daemon message, skip: the
	// daemon's setup cannot store the option (say where else it is tested).
	Refused string
}

// Ignore drops an inspect field from the comparison. Path is dot-separated;
// "*" matches any map key or array index. Reason is required.
type Ignore struct{ Path, Reason string }

// SavedCase checks an option the test machines cannot exercise, from a
// saved inspect file testdata/<Name>.json (and testdata/<Name>.image.json
// when present): Want must appear as whole arguments in runlike's argv.
type SavedCase struct {
	Name string
	Want []string
	Opts runlike.Options
}

const (
	sweepLabel = "runlike.roundtrip=1"
	alpine     = "alpine:3.20"
	fixture    = "runlike-rt:fixture" // alpine + ENTRYPOINT/CMD/ENV/LABEL, see makeFixture
	nano       = "mcr.microsoft.com/windows/nanoserver:ltsc2022"
	nginx      = "nginx:alpine"
)

// alwaysIgnore: fields that differ between two containers made from the
// same argv. Keep it minimal: every entry can hide a bug.
// Before comparing, each document's own ID, 12-char ID prefix and name are
// replaced by {ID}, {ID12} and {NAME} (this covers Id, Name, the
// auto-generated Config.Hostname, ResolvConfPath/HostnamePath/HostsPath/
// LogPath and DNSNames without ignoring them), and anonymous volume names
// (64 hex digits) in Mounts by {ANON}; Config.Env is sorted (see normalize).
var alwaysIgnore = []Ignore{
	{"Created", "creation timestamp"},
	{"State", "run state: status, PID, start/finish times, exit code"},
	{"GraphDriver.Data", "overlay2 layer paths hold per-container random IDs"},
	{"NetworkSettings.SandboxID", "network namespace ID, assigned at start"},
	{"NetworkSettings.SandboxKey", "network namespace path, assigned at start"},
	{"NetworkSettings.Networks.*.EndpointID", "assigned at start"},
	{"NetworkSettings.Networks.*.IPAddress", "assigned at start; a requested IP is in IPAMConfig"},
	{"NetworkSettings.Networks.*.GlobalIPv6Address", "assigned at start; a requested IP is in IPAMConfig"},
}

// runIgnore: extra fields for Run cases. A created container leaves these
// empty or as configured, so options that set them must use create cases.
var runIgnore = []Ignore{
	{"NetworkSettings.Networks.*.MacAddress", "random MAC assigned at start (test --mac-address with a create case)"},
	{"NetworkSettings.Ports", "host ports picked at start; the request is in HostConfig.PortBindings"},
	{"RestartCount", "restarts so far"},
	{"ExecIDs", "exec sessions so far"},
	{"NetworkSettings.Networks.*.Gateway", "endpoint data set at start and cleared when the container exits; a short-lived run case is inspected either side of that"},
	{"NetworkSettings.Networks.*.IPPrefixLen", "as Gateway"},
	{"NetworkSettings.Networks.*.IPv6Gateway", "as Gateway"},
	{"NetworkSettings.Networks.*.GlobalIPv6PrefixLen", "as Gateway"},
}

var (
	areas     = map[string][]Case{}
	saved     []SavedCase
	daemon    bool       // DOCKER_CONTEXT or DOCKER_HOST set
	daemonOS  string     // "linux" or "windows" (docker version), when daemon
	desktop   bool       // Docker Desktop (docker info OperatingSystem)
	blockDev  string     // {blk}: /dev/<disk> on the daemon's host, or ""
	dockerBin = "docker" // test/bin/docker when present
	cliBin    string     // built cmd/runlike
	imgCache  sync.Map   // image ID → inspect JSON
)

// add registers live cases for an area: var _ = add("area", Case{...}, ...).
func add(area string, cs ...Case) bool {
	areas[area] = append(areas[area], cs...)
	return true
}

func addSaved(cs ...SavedCase) bool {
	saved = append(saved, cs...)
	return true
}

func TestMain(m *testing.M) {
	daemon = os.Getenv("DOCKER_CONTEXT") != "" || os.Getenv("DOCKER_HOST") != ""
	if p, err := filepath.Abs("../bin/docker"); err == nil {
		if _, err := os.Stat(p); err == nil {
			dockerBin = p
		}
	}
	code := 1
	if err := setup(); err != nil {
		fmt.Fprintln(os.Stderr, "roundtrip setup:", err)
	} else {
		code = m.Run()
	}
	if daemon {
		sweep()
	}
	if cliBin != "" && os.Getenv("RUNLIKE_EXE") == "" {
		os.RemoveAll(filepath.Dir(cliBin))
	}
	os.Exit(code)
}

func setup() error {
	if !daemon {
		return nil
	}
	out, err := docker("version", "--format", "{{.Server.Os}}")
	if err != nil {
		return err
	}
	daemonOS = strings.TrimSpace(out)
	if out, err = docker("info", "--format", "{{.OperatingSystem}}"); err != nil {
		return err
	}
	desktop = strings.TrimSpace(out) == "Docker Desktop"
	sweep()
	if err := buildCLI(); err != nil {
		return err
	}
	if daemonOS == "windows" {
		if _, err := docker("image", "inspect", nano); err != nil {
			_, err = docker("pull", "-q", nano)
			return err
		}
		return nil
	}
	if _, err := docker("image", "inspect", alpine); err != nil {
		if _, err := docker("pull", "-q", alpine); err != nil {
			return err
		}
	}
	// Pull shared tags once, before the cases run in parallel: a pull in
	// a case could move the tag under another case's container.
	if _, err := docker("image", "inspect", nginx); err != nil {
		if _, err := docker("pull", "-q", nginx); err != nil {
			return err
		}
	}
	blockDevice()
	return makeFixture()
}

// blockDevice sets blockDev to the first whole disk of the daemon's host
// that has a device behind it (not loop, ram, zram, dm), read from the
// host's /sys/block in a helper container: vda on Lima, sda or nvme0n1
// elsewhere. Device-limit options on a started container need a real one.
func blockDevice() {
	out, _ := docker("run", "--rm", alpine, "sh", "-c",
		`for d in /sys/block/*; do [ -e "$d/device" ] && echo "${d##*/}"; done`)
	if f := strings.Fields(out); len(f) > 0 {
		blockDev = "/dev/" + f[0]
	}
}

// makeFixture commits an image with a two-element ENTRYPOINT, a CMD, an
// ENV and a LABEL, so cases can test "same as the image" logic. docker
// commit needs no builder (the pinned static CLI has no buildx).
// buildCLI builds cmd/runlike into cliBin, or uses RUNLIKE_EXE when set
// (a host without Go, such as the Windows test VM).
func buildCLI() error {
	if p := os.Getenv("RUNLIKE_EXE"); p != "" {
		cliBin = p
		return nil
	}
	dir, err := os.MkdirTemp("", "roundtrip-cli")
	if err != nil {
		return err
	}
	cliBin = filepath.Join(dir, "runlike")
	if runtime.GOOS == "windows" {
		cliBin += ".exe"
	}
	if b, err := exec.Command("go", "build", "-o", cliBin, "github.com/lavie/runlike/v2/cmd/runlike").CombinedOutput(); err != nil {
		os.RemoveAll(dir)
		return fmt.Errorf("build cli: %v: %s", err, b)
	}
	return nil
}

func makeFixture() error {
	if _, err := docker("image", "inspect", fixture); err == nil {
		return nil
	}
	src := "rt-fixture-src"
	docker("rm", "-f", src)
	if _, err := docker("create", "--name", src, alpine); err != nil {
		return err
	}
	defer docker("rm", "-f", src)
	_, err := docker("commit",
		"--change", `ENTRYPOINT ["/bin/sh", "-c"]`,
		"--change", `CMD ["echo img"]`,
		"--change", "ENV IMG_ENV=img",
		"--change", "LABEL img.label=img",
		src, fixture)
	return err
}

// sweep removes leftovers of earlier runs: containers labelled or named
// rt-*, and networks and volumes named rt-*. One run per daemon at a time.
func sweep() {
	var ids []string
	for _, f := range []string{"label=" + sweepLabel, "name=^/?rt-"} {
		out, _ := docker("ps", "-aq", "--filter", f)
		ids = append(ids, strings.Fields(out)...)
	}
	if len(ids) > 0 {
		docker(append([]string{"rm", "-f", "-v"}, ids...)...)
	}
	for _, kind := range []string{"network", "volume"} {
		out, _ := docker(kind, "ls", "--format", "{{.Name}}", "--filter", "name=rt-")
		var names []string
		for _, n := range strings.Fields(out) {
			if strings.HasPrefix(n, "rt-") {
				names = append(names, n)
			}
		}
		if len(names) > 0 {
			docker(append([]string{kind, "rm", "-f"}, names...)...)
		}
	}
}

func docker(args ...string) (string, error) {
	out, err := docker1(args...)
	if err != nil && strings.Contains(err.Error(), "error during connect") {
		return recoverDrop(args, out, err, func() (string, error) { return docker1(args...) })
	}
	return out, err
}

// recoverDrop handles a dropped daemon connection ("error during connect
// ... EOF", seen on GitHub's Windows runners when a create takes over a
// minute). The daemon may still finish the request, so for a create or
// run it waits up to two minutes for the named container to appear and
// then uses it; otherwise it runs the command once more.
func recoverDrop(argv []string, out string, err error, again func() (string, error)) (string, error) {
	name := ""
	if len(argv) > 0 && (argv[0] == "create" || argv[0] == "run") {
		for _, a := range argv {
			if n, ok := strings.CutPrefix(a, "--name="); ok {
				name = n
			}
		}
	}
	if name == "" {
		time.Sleep(3 * time.Second)
		return again()
	}
	for deadline := time.Now().Add(2 * time.Minute); time.Now().Before(deadline); time.Sleep(3 * time.Second) {
		if id, e := docker1("container", "inspect", "-f", "{{.Id}}", name); e == nil {
			return id, nil
		}
	}
	return again()
}

func docker1(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var out, errb bytes.Buffer
	c := exec.CommandContext(ctx, dockerBin, args...)
	c.Stdout, c.Stderr = &out, &errb
	if err := c.Run(); err != nil {
		return out.String(), fmt.Errorf("docker %q: %v: %s", args, err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}

// apiCreate creates container name through the Engine API, for state
// the docker CLI refuses to set: it creates a template with docker create
// and args (so the request holds the CLI's own defaults), takes its
// inspect Config and HostConfig as the create request, applies the JSON
// merge patch, and posts that. Skips unless the daemon is on a unix socket.
func apiCreate(t *testing.T, name string, args []string, patch string) {
	t.Helper()
	host := os.Getenv("DOCKER_HOST")
	if host == "" {
		out, err := docker("context", "inspect", "-f", "{{.Endpoints.docker.Host}}")
		if err != nil {
			t.Fatal(err)
		}
		host = strings.TrimSpace(out)
	}
	sock, ok := strings.CutPrefix(host, "unix://")
	if !ok {
		t.Skipf("API create needs a daemon on a unix socket, not %s", host)
	}
	tmpl := name + "-tmpl"
	if _, err := docker(slices.Concat([]string{"create", "--name=" + tmpl, "--label=" + sweepLabel}, args)...); err != nil {
		t.Fatal(err)
	}
	out, err := docker("container", "inspect", tmpl)
	docker("rm", tmpl)
	if err != nil {
		t.Fatal(err)
	}
	var in []struct{ Config, HostConfig map[string]any }
	if err := json.Unmarshal([]byte(out), &in); err != nil {
		t.Fatal(err)
	}
	req := in[0].Config
	delete(req, "Hostname") // the template's ID prefix
	req["HostConfig"] = in[0].HostConfig
	var p map[string]any
	if err := json.Unmarshal([]byte(patch), &p); err != nil {
		t.Fatal(err)
	}
	mergePatch(req, p)
	b, _ := json.Marshal(req)
	c := http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", sock)
	}}}
	resp, err := c.Post("http://docker/containers/create?name="+url.QueryEscape(name), "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		var e bytes.Buffer
		e.ReadFrom(resp.Body)
		t.Fatalf("API create %s: %s: %s", name, resp.Status, e.String())
	}
}

// mergePatch applies JSON merge patch p (RFC 7386) to m.
func mergePatch(m, p map[string]any) {
	for k, v := range p {
		sub, isMap := v.(map[string]any)
		cur, curMap := m[k].(map[string]any)
		switch {
		case v == nil:
			delete(m, k)
		case isMap && curMap:
			mergePatch(cur, sub)
		default:
			m[k] = v
		}
	}
}

// clientShell is the shell the S copy's quoted line runs in: sh, or
// pwsh on Windows (runlike's PowerShell lines refuse to run in 5.1).
var clientShell = runlike.Posix

func init() {
	if runtime.GOOS == "windows" {
		clientShell = runlike.PowerShell
	}
}

var errSkip = errors.New("skipped")

// shellCmd runs line with sh -c, or on Windows as a pwsh script; nil when
// pwsh is missing.
func shellCmd(t *testing.T, line string) *exec.Cmd {
	if clientShell == runlike.Posix {
		return exec.Command("/bin/sh", "-c", line)
	}
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		return nil
	}
	p := filepath.Join(t.TempDir(), "line.ps1")
	if err := os.WriteFile(p, []byte(line+"\nexit $LASTEXITCODE\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-File", p)
}

// envPath puts dockerBin's directory first on PATH, for sh and the CLI.
func envPath() []string {
	return append(os.Environ(), "PATH="+filepath.Dir(dockerBin)+string(os.PathListSeparator)+os.Getenv("PATH"))
}

var safeName = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*$`)

// needLinux skips a live test unless the daemon runs Linux containers.
func needLinux(t *testing.T) {
	t.Helper()
	if !daemon {
		t.Skip("set DOCKER_CONTEXT (e.g. runlike-test-1) or DOCKER_HOST for live round trips")
	}
	if daemonOS != "linux" {
		t.Skipf("needs a Linux daemon, this one runs %s", daemonOS)
	}
}

func TestRoundTrip(t *testing.T) {
	if !daemon {
		t.Skip("set DOCKER_CONTEXT (e.g. runlike-test-1) or DOCKER_HOST for live round trips")
	}
	for _, area := range sortedKeys(areas) {
		t.Run(area, func(t *testing.T) {
			seen := map[string]bool{}
			for i, c := range areas[area] {
				if !safeName.MatchString(c.Name) || seen[c.Name] {
					t.Fatalf("case name %q: want unique, lowercase [a-z0-9.-]", c.Name)
				}
				seen[c.Name] = true
				t.Run(c.Name, func(t *testing.T) {
					if os := cmp.Or(c.OS, "linux"); os != daemonOS {
						t.Skipf("%s case, daemon runs %s", os, daemonOS)
					}
					t.Parallel()
					roundTrip(t, area, c, i == 0)
				})
			}
		})
	}
}

func roundTrip(t *testing.T, area string, c Case, checkCLI bool) {
	p := fmt.Sprintf("rt-%s-%s-%s", area, c.Name, randHex())
	imageA := "{imageA}" // A's image ID, once A exists
	if blockDev == "" && slices.ContainsFunc(c.Args, func(a string) bool { return strings.Contains(a, "{blk}") }) {
		t.Skip("no whole block device found on the daemon's host (/sys/block/*/device)")
	}
	sub := func(ss []string) []string {
		out := make([]string, len(ss))
		for i, s := range ss {
			out[i] = strings.NewReplacer("{p}", p, "{imageA}", imageA, "{blk}", blockDev).Replace(s)
		}
		return out
	}
	// Cleanups run last-in first-out: containers go before Teardown.
	t.Cleanup(func() {
		for _, cmd := range c.Teardown {
			if _, err := docker(sub(cmd)...); err != nil {
				t.Errorf("teardown: %v", err)
			}
		}
	})
	var ids []string
	t.Cleanup(func() {
		if len(ids) > 0 {
			docker(append([]string{"rm", "-f", "-v"}, ids...)...)
		}
	})
	steps := func(what string, cmds [][]string) {
		for _, cmd := range cmds {
			if _, err := docker(sub(cmd)...); err != nil {
				t.Fatalf("%s: %v", what, err)
			}
		}
	}
	steps("setup", c.Setup)

	// A
	verb := []string{"create"}
	if c.Run {
		verb = []string{"run", "-d"}
	}
	out, err := docker(slices.Concat(verb, []string{"--name=" + p + "-a", "--label=" + sweepLabel}, sub(c.Args))...)
	idA := strings.TrimSpace(out)
	if idA != "" {
		ids = append(ids, idA)
	}
	if err != nil {
		if c.Refused != "" && strings.Contains(err.Error(), c.Refused) {
			t.Skipf("daemon refuses the option here: %v", err)
		}
		t.Fatalf("container A: %v", err)
	}
	out, err = docker("container", "inspect", "-f", "{{.Image}}", idA)
	if err != nil {
		t.Fatal(err)
	}
	imageA = strings.TrimSpace(out)
	steps("after A", c.AfterA)
	// The CLI's own gathering: image the tag names, daemon defaults,
	// container names, legacy MAC.
	in, dm, warns, err := dockercli.CLI{Bin: dockerBin}.Inspect(idA)
	if err != nil {
		t.Fatal(err)
	}
	jsA := string(in.Container)
	rc, err := runlike.Args(in, dm, c.Opts)
	if err != nil {
		t.Fatalf("runlike.Args: %v", err)
	}
	warns = append(warns, rc.Warnings...)
	line, err := runlike.Format(rc, runlike.FormatOptions{})
	if err != nil {
		t.Fatalf("runlike.Format: %v", err)
	}
	t.Logf("runlike: %s", line)
	for _, w := range warns {
		t.Logf("warning: %s", w)
	}
	for _, w := range c.Warn {
		if !slices.ContainsFunc(warns, func(s string) bool { return strings.Contains(s, w) }) {
			t.Errorf("no warning with %q in %q", w, warns)
		}
	}
	if checkCLI || c.Warn != nil {
		checkCLIOutput(t, idA, c.Opts, line)
	}
	steps("before B", c.BeforeB)

	// B: argv executed directly; S: the quoted line through sh -c, or
	// through PowerShell on a Windows client.
	bCmd, sCmd := forB(rc, p+"-b", c.Run), forB(rc, p+"-s", c.Run)
	bArgv, sArgv := bCmd.Argv(), sCmd.Argv()
	sLine, err := runlike.Format(sCmd, runlike.FormatOptions{Shell: clientShell})
	if err != nil {
		t.Fatal(err)
	}
	if !c.Run { // Format always writes "docker run", first
		bArgv[1], sArgv[1] = "create", "create"
		sLine = strings.Replace(sLine, "docker run", "docker create", 1)
	}
	want := normalize(t, jsA, c)
	for _, v := range []struct {
		what string
		argv []string
		run  func() (string, error)
	}{
		{"exec", bArgv, func() (string, error) { return docker(bArgv[1:]...) }},
		{string(clientShell), sArgv, func() (string, error) {
			cmd := shellCmd(t, sLine)
			if cmd == nil {
				t.Log("no pwsh: quoted line not run")
				return "", errSkip
			}
			cmd.Env = envPath()
			var o, e bytes.Buffer
			cmd.Stdout, cmd.Stderr = &o, &e
			err := cmd.Run()
			if err != nil {
				err = fmt.Errorf("%v: %s", err, strings.TrimSpace(e.String()))
			}
			return o.String(), err
		}},
	} {
		out, err := v.run()
		if err != nil && err != errSkip && strings.Contains(err.Error(), "error during connect") {
			out, err = recoverDrop(v.argv[1:], out, err, v.run) // argv[0] is "docker"
		}
		id := strings.TrimSpace(out)
		if id != "" {
			ids = append(ids, id)
		}
		if err == errSkip {
			continue
		}
		if err != nil {
			t.Errorf("%s: recreate failed: %v\nargv: %q", v.what, err, v.argv)
			continue
		}
		js, err := docker("container", "inspect", id)
		if err != nil {
			t.Errorf("%s: %v", v.what, err)
			continue
		}
		if d := diff(want, normalize(t, js, c)); d != "" {
			t.Errorf("%s: copy differs from original (- original, + copy):\n%s\nargv: %q", v.what, d, v.argv)
		}
	}
}

// forB turns runlike's command into the one for the copy: the new name,
// and for create cases no --detach (docker create has none).
func forB(rc runlike.Command, name string, run bool) runlike.Command {
	rc.Options = slices.Clone(rc.Options)
	for i, a := range rc.Options {
		if strings.HasPrefix(a, "--name=") {
			rc.Options[i] = "--name=" + name
		}
	}
	if !run {
		rc.Options = slices.DeleteFunc(rc.Options, func(a string) bool { return a == "--detach" })
	}
	return rc
}

func checkCLIOutput(t *testing.T, id string, o runlike.Options, want string) {
	t.Helper()
	args := []string{}
	if o.NoName {
		args = append(args, "--no-name")
	}
	if o.NoLabels {
		args = append(args, "--no-labels")
	}
	if o.UseVolumeID {
		args = append(args, "--use-volume-id")
	}
	cmd := exec.Command(cliBin, append(args, id)...)
	cmd.Env = envPath()
	b, err := cmd.Output()
	if err != nil {
		t.Errorf("cli: %v", err)
	} else if got := strings.TrimSuffix(string(b), "\n"); got != want {
		t.Errorf("cli output differs from library:\n cli %s\n lib %s", got, want)
	}
}

func imageJSON(id string) (string, error) {
	if v, ok := imgCache.Load(id); ok {
		return v.(string), nil
	}
	js, err := docker("image", "inspect", id)
	if err == nil {
		imgCache.Store(id, js)
	}
	return js, err
}

func randHex() string {
	b := make([]byte, 3)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// nameToken matches runs of container-name characters.
var nameToken = regexp.MustCompile(`[A-Za-z0-9_.-]+`)

var anonVolume = regexp.MustCompile(`\b[0-9a-f]{64}\b`)

// normalize decodes one inspect result, substitutes per-container values
// (see alwaysIgnore) and drops the ignored fields.
func normalize(t *testing.T, js string, c Case) any {
	t.Helper()
	var list []map[string]any
	if err := json.Unmarshal([]byte(js), &list); err != nil || len(list) != 1 {
		t.Fatalf("inspect output: %v", err)
	}
	doc := list[0]
	id, _ := doc["Id"].(string)
	name, _ := doc["Name"].(string)
	name = strings.TrimPrefix(name, "/")
	if mounts, ok := doc["Mounts"].([]any); ok {
		for _, m := range mounts {
			if m, ok := m.(map[string]any); ok && anonVolume.MatchString(fmt.Sprint(m["Name"])) {
				for _, k := range []string{"Name", "Source"} {
					if s, ok := m[k].(string); ok {
						m[k] = anonVolume.ReplaceAllString(s, "{ANON}")
					}
				}
			}
		}
	}
	// The daemon merges -e values with the image's env through a map, so
	// the same argv gives Config.Env in random order (seen on 29.9.3). It
	// has already removed duplicate names, so sorting hides nothing.
	if cfg, ok := doc["Config"].(map[string]any); ok {
		if env, ok := cfg["Env"].([]any); ok {
			slices.SortFunc(env, func(a, b any) int { return strings.Compare(fmt.Sprint(a), fmt.Sprint(b)) })
		}
	}
	// The daemon reports --link entries in random order (seen on 29.9.3:
	// same argv, Links reversed in about 3 of 8 runs); compare as a set.
	if hc, ok := doc["HostConfig"].(map[string]any); ok {
		if l, ok := hc["Links"].([]any); ok {
			slices.SortFunc(l, func(a, b any) int { return strings.Compare(fmt.Sprint(a), fmt.Sprint(b)) })
		}
	}
	// A Windows daemon stores -v entries in a random rotation of the given
	// order (seen on 29.9.3: same argv, different Binds order), so the
	// copy's order cannot match; compare them as a set.
	if hc, ok := doc["HostConfig"].(map[string]any); ok && daemonOS == "windows" {
		if b, ok := hc["Binds"].([]any); ok {
			slices.SortFunc(b, func(a, b any) int { return strings.Compare(fmt.Sprint(a), fmt.Sprint(b)) })
		}
	}
	// The name only as a whole token: A's name {p}-a must not touch
	// {p}-a-peer or {p}-ab.
	r := strings.NewReplacer(id, "{ID}", id[:12], "{ID12}")
	var v any = walk(doc, func(s string) string {
		return nameToken.ReplaceAllStringFunc(r.Replace(s), func(w string) string {
			if w == name {
				return "{NAME}"
			}
			return w
		})
	})
	ign := slices.Concat(alwaysIgnore, c.Ignore)
	if c.Run {
		ign = append(ign, runIgnore...)
	}
	for _, i := range ign {
		if i.Reason == "" {
			t.Fatalf("ignore %q has no reason", i.Path)
		}
		drop(v, strings.Split(i.Path, "."))
	}
	return v
}

func walk(v any, f func(string) string) any {
	switch x := v.(type) {
	case string:
		return f(x)
	case []any:
		for i := range x {
			x[i] = walk(x[i], f)
		}
	case map[string]any:
		for k, e := range x {
			x[k] = walk(e, f)
		}
	}
	return v
}

func drop(v any, path []string) {
	switch x := v.(type) {
	case map[string]any:
		if len(path) == 1 {
			if path[0] == "*" {
				clear(x)
			}
			delete(x, path[0])
			return
		}
		for k, e := range x {
			if path[0] == "*" || path[0] == k {
				drop(e, path[1:])
			}
		}
	case []any:
		for i, e := range x {
			if path[0] == "*" || path[0] == fmt.Sprint(i) {
				if len(path) == 1 {
					x[i] = nil
				} else {
					drop(e, path[1:])
				}
			}
		}
	}
}

// diff lists the leaf fields that differ, one "path: value" pair per line.
func diff(a, b any) string {
	fa, fb := map[string]string{}, map[string]string{}
	flatten("", a, fa)
	flatten("", b, fb)
	var lines []string
	for _, k := range sortedKeys(mergeKeys(fa, fb)) {
		va, oka := fa[k]
		vb, okb := fb[k]
		if oka && okb && va == vb {
			continue
		}
		if !oka {
			va = "(absent)"
		}
		if !okb {
			vb = "(absent)"
		}
		lines = append(lines, fmt.Sprintf("  %s\n    - %s\n    + %s", k, va, vb))
	}
	return strings.Join(lines, "\n")
}

func flatten(prefix string, v any, out map[string]string) {
	join := func(k string) string {
		if prefix == "" {
			return k
		}
		return prefix + "." + k
	}
	switch x := v.(type) {
	case map[string]any:
		if len(x) == 0 {
			out[prefix] = "{}"
		}
		for k, e := range x {
			flatten(join(k), e, out)
		}
	case []any:
		if len(x) == 0 {
			out[prefix] = "[]"
		}
		for i, e := range x {
			flatten(join(fmt.Sprint(i)), e, out)
		}
	default:
		var b bytes.Buffer
		e := json.NewEncoder(&b)
		e.SetEscapeHTML(false)
		e.Encode(x)
		out[prefix] = strings.TrimSuffix(b.String(), "\n")
	}
}

func mergeKeys(a, b map[string]string) map[string]bool {
	m := map[string]bool{}
	for k := range a {
		m[k] = true
	}
	for k := range b {
		m[k] = true
	}
	return m
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// TestSaved runs the saved-data cases. It needs no daemon: it checks the
// expected flags are in runlike's argv and, except on Windows, that sh
// turns the quoted line back into exactly that argv.
func TestSaved(t *testing.T) {
	for _, c := range saved {
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			cj, err := os.ReadFile(filepath.Join("testdata", c.Name+".json"))
			if err != nil {
				t.Fatal(err)
			}
			ij, _ := os.ReadFile(filepath.Join("testdata", c.Name+".image.json")) // optional
			rc, err := runlike.Args(runlike.Inspect{Container: cj, Image: ij}, runlike.Daemon{}, c.Opts)
			if err != nil {
				t.Fatal(err)
			}
			argv := rc.Argv()
			for _, w := range c.Want {
				if !slices.Contains(argv, w) {
					t.Errorf("missing %q in %q", w, argv)
				}
			}
			line, err := runlike.Format(rc, runlike.FormatOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if runtime.GOOS == "windows" {
				return // no sh; the PowerShell quoting is checked in package runlike
			}
			b, err := exec.Command("/bin/sh", "-c", `docker() { printf '%s\0' docker "$@"; }; `+line).Output()
			if err != nil {
				t.Fatalf("sh: %v", err)
			}
			if got := strings.Split(strings.TrimSuffix(string(b), "\x00"), "\x00"); !slices.Equal(got, argv) {
				t.Errorf("sh argv %q\n want %q", got, argv)
			}
		})
	}
}
