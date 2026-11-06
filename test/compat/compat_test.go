// Package compat is a black-box test of runlike's command-line contract:
// flags, arguments, exit codes, stdin mode, runnable output and the Docker
// image. It drives any runlike build; see README.md.
package compat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
)

const (
	image    = "alpine:3.20"
	suiteLbl = "compat.suite=1" // on every fixture; cleanup removes by it
	base     = "compat-base"    // created, never started
	plain    = "compat-plain"   // no value needs shell quoting
	volume   = "compat-vol"
)

var (
	runlikeCmd []string // RUNLIKE_BIN split on spaces
	impl       string   // RUNLIKE_IMPL: "python" enables the Python-only expected failures
	argvShim   string   // dir with a fake docker that prints its argv
	baseID     string
)

func TestMain(m *testing.M) {
	bin := os.Getenv("RUNLIKE_BIN")
	impl = os.Getenv("RUNLIKE_IMPL")
	if bin == "" {
		wd, _ := os.Getwd()
		bin, impl = filepath.Join(wd, "runlike-py.sh"), "python"
	}
	runlikeCmd = strings.Fields(bin)
	if runtime.GOOS == "windows" {
		fmt.Println("compat: SKIP: runs runlike's output with sh, which Windows lacks")
		os.Exit(0)
	}
	if os.Getenv("DOCKER_CONTEXT") == "" && os.Getenv("DOCKER_HOST") == "" {
		fmt.Println("compat: SKIP: set DOCKER_CONTEXT (e.g. runlike-test-1) or DOCKER_HOST")
		os.Exit(0)
	}
	dir, err := os.MkdirTemp("", "compat-shim")
	if err != nil {
		panic(err)
	}
	argvShim = dir
	os.WriteFile(filepath.Join(dir, "docker"), []byte("#!/bin/sh\nprintf '%s\\0' \"$@\"\n"), 0o755)

	cleanup()
	code := 1
	if err := setup(); err != nil {
		fmt.Fprintln(os.Stderr, "compat setup:", err)
	} else {
		code = m.Run()
	}
	cleanup()
	os.RemoveAll(dir)
	os.Exit(code)
}

func setup() error {
	steps := [][]string{
		{"pull", "-q", image},
		{"create", "--name", base, "-l", suiteLbl, "-l", "compat.label=a b",
			"-v", volume + ":/data", "-e", "COMPAT_ENV=x", image, "echo", "hello world"},
		{"create", "--name", plain, "-l", suiteLbl, "-e", "COMPAT_ENV=x", image, "true"},
	}
	for _, s := range steps {
		if _, err := docker(s...); err != nil {
			return err
		}
	}
	id, err := docker("inspect", "-f", "{{.Id}}", base)
	baseID = strings.TrimSpace(id)
	return err
}

func cleanup() {
	ids, _ := docker("ps", "-aq", "--filter", "label="+suiteLbl)
	if f := strings.Fields(ids); len(f) > 0 {
		docker(append([]string{"rm", "-f"}, f...)...)
	}
	docker("volume", "rm", "-f", volume)
}

func docker(args ...string) (string, error) {
	var out, errb bytes.Buffer
	c := exec.Command("docker", args...)
	c.Stdout, c.Stderr = &out, &errb
	if err := c.Run(); err != nil {
		return out.String(), fmt.Errorf("docker %v: %v: %s", args, err, errb.String())
	}
	return out.String(), nil
}

type result struct {
	stdout, stderr string
	code           int
}

func run(t *testing.T, stdin string, args ...string) result {
	t.Helper()
	c := exec.Command(runlikeCmd[0], append(slices.Clone(runlikeCmd[1:]), args...)...)
	c.Stdin = strings.NewReader(stdin)
	var out, errb bytes.Buffer
	c.Stdout, c.Stderr = &out, &errb
	err := c.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("cannot run runlike: %v", err)
	}
	return result{out.String(), errb.String(), code}
}

// ok runs runlike and requires exit 0 with one "docker run ..." command on stdout.
func ok(t *testing.T, args ...string) string {
	t.Helper()
	return okStdin(t, "", args...)
}

func okStdin(t *testing.T, stdin string, args ...string) string {
	t.Helper()
	r := run(t, stdin, args...)
	if r.code != 0 {
		t.Fatalf("runlike %v: exit %d, stderr %q", args, r.code, r.stderr)
	}
	if !strings.HasPrefix(r.stdout, "docker run ") || !strings.HasSuffix(r.stdout, "\n") {
		t.Fatalf("runlike %v: want one 'docker run ...' line, got %q", args, r.stdout)
	}
	return r.stdout
}

// argv evaluates a runlike output with sh, with a fake docker, and returns
// the arguments docker would get (after "docker"). split=true runs it as
// $(cat file), i.e. word splitting without quote removal.
func argv(t *testing.T, out string, split bool) ([]string, error) {
	t.Helper()
	f := filepath.Join(t.TempDir(), "cmd")
	os.WriteFile(f, []byte(out), 0o644)
	script := `. "$1"`
	if split {
		script = `$(cat "$1")`
	}
	c := exec.Command("sh", "-c", script, "sh", f)
	c.Env = append(os.Environ(), "PATH="+argvShim+":"+os.Getenv("PATH"))
	b, err := c.Output()
	if err != nil {
		return nil, fmt.Errorf("sh: %v", err)
	}
	a := strings.Split(string(b), "\x00")
	return a[:len(a)-1], nil
}

func mustArgv(t *testing.T, out string) []string {
	t.Helper()
	a, err := argv(t, out, false)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// norm is argv with the options before the image sorted: option order is
// not part of the contract (1.6.3 prints labels in random set order).
func norm(t *testing.T, out string) []string {
	t.Helper()
	a := mustArgv(t, out)
	if i := slices.Index(a, image); i > 0 {
		slices.Sort(a[1:i])
	}
	return a
}

// opt returns the values of an option (long name, optional short name) in a
// docker argv, up to the image. It understands --x=v, --x v, -s v, -s=v.
func opt(a []string, long, short string) []string {
	var vals []string
	for i := 1; i < len(a); i++ {
		s := a[i]
		if s == image {
			break
		}
		for _, n := range []string{long, short} {
			if n == "" {
				continue
			}
			if v, found := strings.CutPrefix(s, n+"="); found {
				vals = append(vals, v)
			} else if s == n && i+1 < len(a) {
				vals = append(vals, a[i+1])
			}
		}
	}
	return vals
}

// xfail reports err; for impl "python" with a known 1.6.3 bug it is an
// expected failure (skip), and an unexpected pass is an error.
func xfail(t *testing.T, err error, pyBug string) {
	t.Helper()
	switch {
	case impl == "python" && err != nil:
		t.Skipf("expected fail on Python 1.6.3 (%s): %v", pyBug, err)
	case impl == "python":
		t.Errorf("unexpectedly passed on Python 1.6.3; remove the expected-fail mark (%s)", pyBug)
	case err != nil:
		t.Error(err)
	}
}

// --- interface ---

func TestByNameOneLine(t *testing.T) {
	out := ok(t, base)
	if strings.Count(out, "\n") != 1 {
		t.Errorf("want one line, got %q", out)
	}
	a := mustArgv(t, out)
	if a[0] != "run" || !slices.Contains(a, image) {
		t.Errorf("argv %q: want run ... %s", a, image)
	}
	if v := opt(a, "--name", ""); !reflect.DeepEqual(v, []string{base}) {
		t.Errorf("--name = %q, want %q", v, base)
	}
}

func TestByID(t *testing.T) {
	want := norm(t, ok(t, base))
	for _, id := range []string{baseID, baseID[:12]} {
		if got := norm(t, ok(t, id)); !reflect.DeepEqual(got, want) {
			t.Errorf("by id %s: %q\nby name: %q", id, got, want)
		}
	}
}

func TestOptionsAfterContainer(t *testing.T) {
	for _, f := range []string{"--no-name", "-l", "--no-labels", "-p", "--use-volume-id"} {
		before, after := norm(t, ok(t, f, base)), norm(t, ok(t, base, f))
		if !reflect.DeepEqual(before, after) {
			t.Errorf("%s: before %q, after %q", f, before, after)
		}
	}
}

func TestCombinedShortFlags(t *testing.T) {
	want := norm(t, ok(t, "-p", "-l", base))
	for _, f := range []string{"-pl", "-lp"} {
		if got := norm(t, ok(t, f, base)); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: %q, want %q", f, got, want)
		}
	}
}

func TestDoubleDash(t *testing.T) {
	if got, want := norm(t, ok(t, "--", base)), norm(t, ok(t, base)); !reflect.DeepEqual(got, want) {
		t.Errorf("runlike -- X = %q, want %q", got, want)
	}
}

func TestNoName(t *testing.T) {
	if v := opt(mustArgv(t, ok(t, "--no-name", base)), "--name", ""); len(v) != 0 {
		t.Errorf("--no-name still prints --name %q", v)
	}
}

func TestLabels(t *testing.T) {
	if v := opt(mustArgv(t, ok(t, base)), "--label", "-l"); !slices.Contains(v, "compat.label=a b") {
		t.Errorf("labels %q, want compat.label=a b", v)
	}
	for _, f := range []string{"-l", "--no-labels"} {
		if v := opt(mustArgv(t, ok(t, f, base)), "--label", "-l"); len(v) != 0 {
			t.Errorf("%s still prints labels %q", f, v)
		}
	}
}

func TestUseVolumeID(t *testing.T) {
	a := mustArgv(t, ok(t, "--use-volume-id", base))
	vols := append(opt(a, "--volume", "-v"), opt(a, "--mount", "")...)
	if !slices.ContainsFunc(vols, func(s string) bool { return strings.Contains(s, volume) }) {
		t.Errorf("--use-volume-id: volumes %q do not name %s", vols, volume)
	}
}

func TestPrettySameArgv(t *testing.T) {
	for _, f := range [][]string{{"-p"}, {"--pretty"}} {
		pretty := ok(t, append(f, base)...)
		if strings.Count(pretty, "\n") < 2 {
			t.Errorf("%s: want several lines, got %q", f[0], pretty)
		}
		if got, want := norm(t, pretty), norm(t, ok(t, base)); !reflect.DeepEqual(got, want) {
			t.Errorf("%s argv %q, plain argv %q", f[0], got, want)
		}
	}
}

func TestStdin(t *testing.T) {
	js, err := docker("inspect", base)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"-s", "--stdin"} {
		a := mustArgv(t, okStdin(t, js, f))
		if !slices.Contains(a, image) || !reflect.DeepEqual(opt(a, "--name", ""), []string{base}) {
			t.Errorf("%s: argv %q lacks image or name", f, a)
		}
	}
	// docker container inspect gives the same JSON shape.
	js2, _ := docker("container", "inspect", base)
	okStdin(t, js2, "--stdin", "--no-name", "-p")
}

func TestHelp(t *testing.T) {
	r := run(t, "", "--help")
	if r.code != 0 || !strings.Contains(r.stdout, "Usage:") {
		t.Fatalf("--help: exit %d, stdout %q", r.code, r.stdout)
	}
	for _, f := range []string{"--no-name", "--use-volume-id", "-p, --pretty", "-s, --stdin", "-l, --no-labels", "--help"} {
		if !strings.Contains(r.stdout, f) {
			t.Errorf("--help does not list %q", f)
		}
	}
}

// --version is new in 2.0. It must not change the 1.x flags: -v stays
// unknown, and a container named like a flag still works after --.
func TestVersion(t *testing.T) {
	if r := run(t, "", "-v", base); r.code != 2 {
		t.Errorf("-v: exit %d, want 2 (unknown option, as in 1.x)", r.code)
	}
	if impl == "python" {
		t.Skip("runlike 1.x has no --version")
	}
	r := run(t, "", "--version")
	if r.code != 0 || !strings.HasPrefix(r.stdout, "runlike ") || strings.Count(r.stdout, "\n") != 1 {
		t.Errorf("--version: exit %d, stdout %q; want 0, one line 'runlike <version>'", r.code, r.stdout)
	}
	if got := ok(t, "--no-name", "-p", base); !strings.Contains(got, "\\\n") {
		t.Errorf("--no-name -p: %q", got)
	}
}

// --- errors ---

func TestMissingContainer(t *testing.T) {
	r := run(t, "", "compat-missing")
	if r.code != 1 || r.stdout != "" || !strings.Contains(r.stderr, "No such container compat-missing") {
		t.Errorf("exit %d, stdout %q, stderr %q; want 1, empty, 'No such container compat-missing'", r.code, r.stdout, r.stderr)
	}
}

func TestUsageErrors(t *testing.T) {
	for _, args := range [][]string{{}, {"--bogus", base}, {base, "compat-other"}} {
		r := run(t, "", args...)
		if r.code != 2 || r.stdout != "" || !strings.Contains(r.stderr, "Usage:") {
			t.Errorf("runlike %q: exit %d, stdout %q, stderr %q; want 2, empty, Usage:", args, r.code, r.stdout, r.stderr)
		}
	}
}

func TestStdinGarbage(t *testing.T) {
	for _, in := range []string{"", "not json\n"} {
		if r := run(t, in, "--stdin"); r.code == 0 || r.stdout != "" {
			t.Errorf("--stdin %q: exit %d, stdout %q; want non-zero, empty", in, r.code, r.stdout)
		}
	}
}

// --- runnable output (real docker) ---

type facts struct {
	Name   string
	Config struct {
		Image  string
		Cmd    []string
		Env    []string
		Labels map[string]string
	}
}

func inspect(t *testing.T, name string) facts {
	t.Helper()
	js, err := docker("inspect", name)
	if err != nil {
		t.Fatal(err)
	}
	var f []facts
	if err := json.Unmarshal([]byte(js), &f); err != nil || len(f) != 1 {
		t.Fatalf("inspect %s: %v", name, err)
	}
	return f[0]
}

// recreate captures runlike's output for a fresh copy of base, removes the
// container and runs the output with sh (script is how sh gets it).
func recreate(t *testing.T, name, script string, flags ...string) error {
	if _, err := docker("create", "--name", name, "-l", suiteLbl, "-l", "compat.label=a b",
		"-e", "COMPAT_ENV=x", image, "echo", "hello world"); err != nil {
		t.Fatal(err)
	}
	want := inspect(t, name)
	out := ok(t, append(flags, name)...)
	docker("rm", "-f", name)
	f := filepath.Join(t.TempDir(), "cmd")
	os.WriteFile(f, []byte(out), 0o644)
	if b, err := exec.Command("sh", "-c", script, "sh", f).CombinedOutput(); err != nil {
		return fmt.Errorf("sh -c %s on %q: %v: %s", script, out, err, b)
	}
	got := inspect(t, name)
	if got.Name != want.Name || got.Config.Image != want.Config.Image ||
		!reflect.DeepEqual(got.Config.Cmd, want.Config.Cmd) ||
		!reflect.DeepEqual(got.Config.Labels, want.Config.Labels) ||
		!slices.Contains(got.Config.Env, "COMPAT_ENV=x") {
		return fmt.Errorf("copy differs:\n got %+v\nwant %+v", got, want)
	}
	return nil
}

func TestEvalRecreates(t *testing.T) {
	if err := recreate(t, "compat-eval", `eval "$(cat "$1")"`); err != nil {
		t.Error(err)
	}
}

func TestPrettyPastedIntoSh(t *testing.T) {
	if err := recreate(t, "compat-pretty", `sh "$1"`, "-p"); err != nil {
		t.Error(err)
	}
}

// $(runlike X) unquoted: word splitting without quote removal. Works only if
// nothing that needs no quoting is quoted.
func TestWordSplitting(t *testing.T) {
	out := ok(t, plain)
	want := mustArgv(t, out)
	got, err := argv(t, out, true)
	if err == nil && !reflect.DeepEqual(got, want) {
		err = fmt.Errorf("$(runlike) argv %q\n          eval argv %q", got, want)
	}
	xfail(t, err, "runlike-yak: quotes values that need no quoting, e.g. --shm-size=\"67108864\"")
}

// --- Docker image ---

func TestImage(t *testing.T) {
	img := os.Getenv("RUNLIKE_IMAGE")
	if img == "" {
		t.Skip("RUNLIKE_IMAGE not set")
	}
	want := ok(t, base)
	got, err := docker("run", "--rm", "-v", "/var/run/docker.sock:/var/run/docker.sock", img, base)
	if err != nil || !reflect.DeepEqual(norm(t, got), norm(t, want)) {
		t.Errorf("image: %q (%v), binary: %q", got, err, want)
	}
	js, _ := docker("inspect", base)
	c := exec.Command("docker", "run", "--rm", "-i", img, "--stdin")
	c.Stdin = strings.NewReader(js)
	b, err := c.Output()
	if wantS := okStdin(t, js, "--stdin"); err != nil || !reflect.DeepEqual(norm(t, string(b)), norm(t, wantS)) {
		t.Errorf("image --stdin: %q (%v), binary: %q", b, err, wantS)
	}
	c = exec.Command("docker", "run", "--rm", "-v", "/var/run/docker.sock:/var/run/docker.sock", img, "compat-missing")
	var eb bytes.Buffer
	c.Stderr = &eb
	err = c.Run()
	if ee, isExit := err.(*exec.ExitError); !isExit || ee.ExitCode() != 1 || !strings.Contains(eb.String(), "No such container compat-missing") {
		t.Errorf("image, missing container: %v, stderr %q; want exit 1", err, eb.String())
	}
}
