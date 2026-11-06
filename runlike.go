// Package runlike turns `docker inspect` data into the `docker run`
// argument list that recreates the container.
//
// The result is a Command (argument vectors), never a shell string; quote
// it with Format as the last step.
//
// Output order (fixed, so goldens and users can rely on it):
//
//	docker run --name=NAME <area options...> IMAGE [COMMAND...]
//
// Area options come from the handlers in the areas slice below, in that
// order; within one area, in the order its handler appends them. Every
// option is one argument that starts with "-" (use "--flag=value", never
// "--flag", "value"), so each option is one line in pretty output.
package runlike

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"unicode"

	dockerspec "github.com/moby/docker-image-spec/specs-go/v1"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"
)

// Options mirror runlike's command-line switches.
type Options struct {
	NoName      bool // leave out --name
	UseVolumeID bool // keep auto-assigned volume names
	NoLabels    bool // leave out --label
}

// Inspect is the inspect data of one container.
type Inspect struct {
	// Container is `docker container inspect` output: an array or a single
	// object; only the first container is used.
	Container []byte
	// Image is `docker image inspect` output for the image the copy runs
	// (Config.Image). Empty: no image data, so values cannot be compared
	// with the image's and are kept.
	Image []byte
	// ConfigMACAddress is Config.MacAddress as reported by API 1.51 (API
	// 1.52 dropped it): the MAC pinned on the first network (--mac-address),
	// for a created, running or stopped container. Empty: none was pinned
	// when Daemon.Platform is set (the CLI read it), else read from
	// Container when that is API <= 1.51 data.
	ConfigMACAddress string
}

// Daemon holds facts about the daemon the container lives on. The CLI
// fills them from `docker version`, `docker info` and the containers it
// references; the zero value means unknown (--stdin mode), and then only
// Docker's built-in defaults count. One Daemon serves every container of
// that daemon.
type Daemon struct {
	// Platform is the daemon's native "os/arch", e.g. "linux/arm64". The
	// CLI always sets it; empty means data without a daemon (--stdin):
	// then a MAC pinned on a started container and --cidfile are warned about.
	Platform string
	// Defaults are this daemon's own defaults. A value is left out only
	// when it equals both Docker's built-in default and this one
	// (AGENTS.md ruling); an empty field means unknown.
	Defaults DaemonDefaults
	// HostName is the daemon host's name (`docker info` Name), to tell a
	// user-set --hostname from the host's under --network host.
	HostName string
	// Desktop: the daemon is Docker Desktop (`docker info`
	// OperatingSystem). Its desktop.docker.io/ labels are its own (its
	// raw socket shows them), so --label leaves them out.
	Desktop bool
	// ContainerNames maps full container IDs to names, so container:<id>
	// targets (network, ipc, pid) print by name. It needs to hold only the
	// containers the target container references.
	ContainerNames map[string]string
	// NetworkNames maps a network ID used as NetworkMode (--network <ID>)
	// to the network's name. A created container lists such a network
	// under its name with no NetworkID, so only this links the two.
	NetworkNames map[string]string
}

// DaemonDefaults are the defaults a daemon applies when a flag is absent.
// The CLI fills what `docker info` reports: LogDriver (LoggingDriver),
// Runtime (DefaultRuntime), and CgroupnsMode "host" on cgroup v1 hosts
// (the built-in default there; a configured default-cgroupns-mode is not
// reported). No API reports default-ipc-mode or default-shm-size, so the
// CLI leaves IpcMode and ShmSize empty. Daemon default-ulimits need no
// field: Docker's built-in default is none, so every --ulimit is printed.
// Windows daemons report Isolation (process on Server, hyperv on client
// Windows).
type DaemonDefaults struct {
	LogDriver    string // e.g. "json-file", "local"
	Runtime      string // e.g. "runc"
	CgroupnsMode string // "private" or "host"
	IpcMode      string // "private" or "shareable"
	ShmSize      int64  // bytes; 0 when unknown
	Isolation    string // Windows daemons: "process" or "hyperv"
}

// isDefault reports whether v needs no flag: it equals Docker's built-in
// default and this daemon's default, when that is known (daemon != "").
func isDefault(v, builtin, daemon string) bool {
	return v == builtin && (daemon == "" || daemon == builtin)
}

// containerRef turns "container:<full ID>" into "container:<name>" when
// Daemon.ContainerNames knows the ID (CLI mode); otherwise it returns
// mode unchanged. For --network, --ipc and --pid.
func (x *ctx) containerRef(mode string) string {
	if id, ok := strings.CutPrefix(mode, "container:"); ok {
		if name := x.d.ContainerNames[id]; name != "" {
			return "container:" + name
		}
	}
	return mode
}

// Command is the docker run command that recreates a container.
type Command struct {
	Options []string // between "docker run" and the image
	Image   string
	Args    []string // command and arguments after the image
	// Warnings name what the command cannot reproduce exactly. The CLI
	// prints them to stderr; they are never part of the command. Control,
	// format and line-break characters in the container data they quote
	// are escaped (\n); a newline in a warning is runlike's own, before
	// each suggested command.
	Warnings []string
}

// Argv returns the whole argument vector: docker, run, options, image, args.
func (c Command) Argv() []string {
	return append(append(append([]string{"docker", "run"}, c.Options...), c.Image), c.Args...)
}

// ctx is what every option handler gets.
type ctx struct {
	c    *container.InspectResponse
	cfg  *container.Config                // never nil
	hc   *container.HostConfig            // never nil
	img  *image.InspectResponse           // nil in --stdin mode
	icfg *dockerspec.DockerOCIImageConfig // image config; nil in --stdin mode (no image data)
	OS   string                           // daemon/container OS: "linux" or "windows"
	opts Options
	d    Daemon
	mac  string // Inspect.ConfigMACAddress
	// macKnown: mac is the MAC pinned on the first network, "" = none
	macKnown bool

	apiSocket bool // --use-api-socket detected; see foldAPISocket
	warnings  []string
}

// warn records something the printed command cannot reproduce exactly.
// String arguments are container data: line breaks and other control or
// format characters in them are escaped (\n, \u2028), so a value cannot
// start a fake warning line. Only format and raw arguments (text runlike
// built, e.g. posixCmd lines) keep their newlines.
func (x *ctx) warn(format string, a ...any) {
	for i, v := range a {
		switch v := v.(type) {
		case string:
			a[i] = data(v)
		case raw:
			a[i] = string(v)
		}
	}
	x.warnings = append(x.warnings, fmt.Sprintf(format, a...))
}

// raw is warn text runlike built itself, with its data already escaped.
type raw string

// data prints a string with %s escaped (see escape), with %q quoted.
type data string

func (d data) Format(f fmt.State, verb rune) {
	if verb == 'q' {
		io.WriteString(f, strconv.Quote(string(d)))
	} else {
		io.WriteString(f, escape(string(d)))
	}
}

// escape writes every rune that is not printable (newline, CR, tab, ESC,
// C1 controls, bidi and other format characters, U+2028) as \n or \u2028.
func escape(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsPrint(r) {
			b.WriteRune(r)
		} else {
			q := strconv.QuoteRune(r)
			b.WriteString(q[1 : len(q)-1])
		}
	}
	return b.String()
}

// areas fixes the output order of option groups. Each handler lives in
// the file named after its area.
var areas = []func(*ctx) []string{
	processArgs,
	networkingArgs,
	storageArgs,
	resourcesArgs,
	securityArgs,
	runtimeArgs,
	loggingArgs,
	healthArgs,
	metadataArgs,
}

// Args returns the docker run command that recreates the container in
// in, on daemon d (zero value: unknown), with the user's switches opts.
func Args(in Inspect, d Daemon, opts Options) (Command, error) {
	var c container.InspectResponse
	if err := decodeFirst(in.Container, &c); err != nil {
		return Command{}, err
	}
	if c.Config == nil || c.Config.Image == "" {
		return Command{}, errors.New("inspect data has no Config.Image: not a docker container inspect object")
	}
	// docker run would read it as an option; a daemon never stores one.
	if strings.HasPrefix(c.Config.Image, "-") {
		return Command{}, fmt.Errorf("Config.Image %q is not an image reference", c.Config.Image)
	}
	mac := in.ConfigMACAddress
	// The MAC pinned on the first network is known when given, when the
	// CLI gathered the data (it reads API 1.51 itself and warns when it
	// cannot), or when the data is API <= 1.51 (--stdin): there
	// Config.MacAddress, which moby's Go type no longer has, is the pinned
	// MAC, and its absence means none was pinned. Such data has the key,
	// or the top-level NetworkSettings.MacAddress that API 1.52 dropped
	// (Docker 29 adds that one only on the default bridge).
	macKnown := mac != "" || d.Platform != ""
	if mac == "" {
		var old struct {
			Config          struct{ MacAddress *string }
			NetworkSettings struct{ MacAddress *string }
		}
		if decodeFirst(in.Container, &old) == nil {
			if old.Config.MacAddress != nil {
				mac, macKnown = *old.Config.MacAddress, true
			}
			macKnown = macKnown || old.NetworkSettings.MacAddress != nil
		}
	}
	var img *image.InspectResponse
	if len(bytes.TrimSpace(in.Image)) > 0 {
		img = new(image.InspectResponse)
		if err := decodeFirst(in.Image, img); err != nil {
			return Command{}, err
		}
	}
	return build(c, img, d, opts, mac, macKnown), nil
}

func decodeFirst(data []byte, v any) error {
	data = bytes.TrimSpace(data)
	if len(data) > 0 && data[0] == '[' {
		var list []json.RawMessage
		if err := json.Unmarshal(data, &list); err != nil {
			return err
		}
		if len(list) == 0 {
			return errors.New("inspect data holds no object")
		}
		data = list[0]
	}
	return json.Unmarshal(data, v)
}

// build returns the docker run command for c. img may be nil.
func build(c container.InspectResponse, img *image.InspectResponse, d Daemon, opts Options, mac string, macKnown bool) Command {
	x := &ctx{c: &c, cfg: c.Config, hc: c.HostConfig, img: img, OS: c.Platform, opts: opts, d: d, mac: mac, macKnown: macKnown}
	if x.cfg == nil {
		x.cfg = &container.Config{}
	}
	if x.hc == nil {
		x.hc = &container.HostConfig{}
	}
	if x.OS == "" {
		x.OS = "linux"
	}
	if img != nil {
		x.icfg = img.Config
		if x.icfg == nil {
			x.icfg = &dockerspec.DockerOCIImageConfig{}
		}
	}

	foldAPISocket(x)
	x.imageWarnings()

	var cmd Command
	if !opts.NoName {
		if name := strings.TrimPrefix(c.Name, "/"); name != "" {
			cmd.Options = append(cmd.Options, "--name="+name)
		}
	}
	for _, area := range areas {
		cmd.Options = append(cmd.Options, area(x)...)
	}
	cmd.Image = x.cfg.Image
	cmd.Args = x.command()
	cmd.Warnings = x.warnings
	return cmd
}

// notInImage returns the container values the image does not already
// set, keeping their order. Without image data it returns all of them.
func notInImage(container, image []string) []string {
	var out []string
	for _, v := range container {
		if !slices.Contains(image, v) {
			out = append(out, v)
		}
	}
	return out
}

// eqKeys warns about keys of m holding "=": the docker CLI splits each
// key=value of flag at the first "=", so no form can write such a key.
func (x *ctx) eqKeys(flag string, m map[string]string) {
	for _, k := range sortedKeys(m) {
		if before, _, ok := strings.Cut(k, "="); ok {
			x.warn("%s key %q holds \"=\", where docker run splits key from value: the copy gets key %q", flag, k, before)
		}
	}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// imageCfg returns the image config, or an empty one in --stdin mode, so
// handlers can read fields without nil checks. Use x.icfg == nil to tell
// "no image data" from "image sets nothing".
func (x *ctx) imageCfg() *dockerspec.DockerOCIImageConfig {
	if x.icfg == nil {
		return &dockerspec.DockerOCIImageConfig{}
	}
	return x.icfg
}

// imageWarnings names what the image data leaves open: without it, values
// cannot be compared; with an image other than the container's (the tag
// moved), the copy runs that image, and what it adds no flag removes.
func (x *ctx) imageWarnings() {
	if x.img == nil {
		msg := "no image data: values the image already sets are printed too"
		if len(x.cfg.Entrypoint) == 0 {
			msg += "; an entrypoint cleared with --entrypoint= cannot be told from none, so the copy gets the image's entrypoint back"
		}
		x.warn("%s", msg)
		return
	}
	if x.img.ID == "" || x.c.Image == "" || x.img.ID == x.c.Image {
		return
	}
	var added []string
	have := map[string]bool{}
	for _, e := range x.cfg.Env {
		k, _, _ := strings.Cut(e, "=")
		have[k] = true
	}
	for _, e := range x.icfg.Env {
		if k, _, _ := strings.Cut(e, "="); !have[k] {
			added = append(added, "env "+k)
		}
	}
	for _, k := range sortedKeys(x.icfg.Labels) {
		// Docker Desktop's API proxy adds desktop.docker.io/ labels to
		// every container and hides them from container inspect; docker
		// commit bakes them into images. The original had it, the copy
		// gets it again.
		if _, ok := x.cfg.Labels[k]; !ok && !strings.HasPrefix(k, desktopLabels) {
			added = append(added, "label "+k)
		}
	}
	for _, k := range sortedKeys(x.icfg.Volumes) {
		if _, ok := x.cfg.Volumes[k]; !ok {
			added = append(added, "volume "+k)
		}
	}
	ports := map[string]bool{}
	for p := range x.cfg.ExposedPorts {
		ports[p.String()] = true
	}
	for _, k := range sortedKeys(x.icfg.ExposedPorts) {
		if !ports[k] {
			added = append(added, "exposed port "+k)
		}
	}
	if x.icfg.User != "" && x.cfg.User == "" {
		added = append(added, "user "+x.icfg.User)
	}
	if x.icfg.WorkingDir != "" && x.cfg.WorkingDir == "" {
		added = append(added, "workdir "+x.icfg.WorkingDir)
	}
	msg := fmt.Sprintf("image %s is now %s, not the container's %s: the copy runs the new image, and values are compared with it",
		x.cfg.Image, shortID(x.img.ID), shortID(x.c.Image))
	if len(added) > 0 {
		msg += "; it adds what no flag removes: " + strings.Join(added, ", ")
	}
	x.warn("%s", msg)
}

// desktopLabels prefixes the labels Docker Desktop adds to containers.
const desktopLabels = "desktop.docker.io/"

func shortID(id string) string {
	id = strings.TrimPrefix(id, "sha256:")
	if len(id) > 12 {
		id = id[:12]
	}
	return id
}
