package roundtrip

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/lavie/runlike/v2"
)

// Storage area: -v, --mount, --tmpfs, --volumes-from, --volume-driver,
// --storage-opt, --device, --use-api-socket.

// mkdirs makes host directories on the daemon's machine (bind sources must
// exist for --mount).
func mkdirs(dirs ...string) []string {
	return append([]string{"run", "--rm", "-v", "/tmp:/h", alpine, "mkdir", "-p"}, dirs...)
}

// volImage commits {p}-img: alpine with VOLUME /iv and /iv2.
var volImage = [][]string{
	{"create", "--name={p}-from1", alpine},
	{"commit", "--change", `VOLUME ["/iv", "/iv2"]`, "{p}-from1", "{p}-img"},
	{"rm", "{p}-from1"},
}

// bindsOrder: with two or more -v entries the CLI fills HostConfig.Binds
// from a Go map, so their order is random for the same argv. Mounts holds
// the same data (source, destination, mode, RW, propagation), sorted.
var bindsOrder = []Ignore{{"HostConfig.Binds", "random order with 2+ -v entries (CLI iterates a map); Mounts compares the same data"}}

var useVolumeIDIgnore = []Ignore{
	{"HostConfig.Binds", "--use-volume-id turns anonymous volumes into named -v entries by design"},
	{"Config.Volumes", "--use-volume-id turns anonymous volumes into named -v entries by design"},
	{"Mounts.*.Mode", "the daemon sets mode z on named -v volumes, none on anonymous ones"},
}

var _ = add("storage",
	// -v: host paths, named and anonymous volumes, options
	Case{Name: "volume-bind", Args: []string{"-v", "/tmp/{p}:/data", alpine, "true"}},
	Case{Name: "volume-bind-opts", Args: []string{"-v", "/tmp/{p}-1:/a:ro", "-v", "/tmp/{p}-2:/b:rw,z", "-v", "/tmp/{p}-3:/c:Z,rshared", "-v", "/tmp/{p}-4:/d:ro,rslave", alpine, "true"}, Ignore: bindsOrder},
	Case{Name: "volume-bind-spaces", Args: []string{"-v", "/tmp/{p} a b/x y:/in side/x y:ro", "-v", "/tmp/{p}-$HOME 'q' \"d\"`id`:/sh", alpine, "true"}, Ignore: bindsOrder},
	Case{Name: "volume-named", Setup: [][]string{{"volume", "create", "{p}-v"}},
		Args: []string{"-v", "{p}-v:/v", "-v", "{p}-v:/v2:ro,nocopy", alpine, "true"}, Ignore: bindsOrder,
		Teardown: [][]string{{"volume", "rm", "{p}-v"}}},
	Case{Name: "volume-named-auto-create", Args: []string{"-v", "{p}-xauto:/v", alpine, "true"},
		Teardown: [][]string{{"volume", "rm", "{p}-xauto"}}},
	Case{Name: "volume-anon", Args: []string{"-v", "/anon", "-v", "/anon two", alpine, "true"}},
	Case{Name: "volume-image-anon", Setup: volImage, Args: []string{"{p}-img", "true"},
		Teardown: [][]string{{"rmi", "{p}-img"}}},
	Case{Name: "volume-image-anon-plus", Setup: volImage,
		Args:     []string{"-v", "/iv", "-v", "/extra", "-v", "/tmp/{p}:/iv2", "{p}-img", "true"},
		Teardown: [][]string{{"rmi", "{p}-img"}}},
	Case{Name: "volume-image-named-over", Setup: append([][]string{{"volume", "create", "{p}-v"}}, volImage...),
		Args:     []string{"-v", "{p}-v:/iv", "{p}-img", "true"},
		Teardown: [][]string{{"rmi", "{p}-img"}, {"volume", "rm", "{p}-v"}}},
	Case{Name: "use-volume-id-anon", Opts: runlike.Options{UseVolumeID: true},
		Args: []string{"-v", "/anon", alpine, "true"}, Ignore: useVolumeIDIgnore,
		Teardown: [][]string{{"volume", "prune", "-f"}}},
	Case{Name: "use-volume-id-image", Opts: runlike.Options{UseVolumeID: true}, Setup: append([][]string{{"volume", "create", "{p}-v"}}, volImage...),
		Args: []string{"-v", "{p}-v:/iv", "{p}-img", "true"}, Ignore: useVolumeIDIgnore,
		Teardown: [][]string{{"rmi", "{p}-img"}, {"volume", "rm", "{p}-v"}, {"volume", "prune", "-f"}}},
	Case{Name: "use-volume-id-named", Opts: runlike.Options{UseVolumeID: true}, Setup: [][]string{{"volume", "create", "{p}-v"}},
		Args:     []string{"-v", "{p}-v:/v", alpine, "true"},
		Teardown: [][]string{{"volume", "rm", "{p}-v"}}},

	// --mount
	Case{Name: "mount-bind", Args: []string{"--mount", "type=bind,src=/tmp,dst=/b", alpine, "true"}},
	Case{Name: "mount-bind-opts", Args: []string{
		"--mount", "type=bind,src=/tmp,dst=/b1,readonly,bind-propagation=rslave",
		"--mount", "type=bind,src=/tmp,dst=/b2,bind-recursive=disabled",
		"--mount", "type=bind,src=/tmp,dst=/b3,ro,bind-recursive=writable",
		"--mount", "type=bind,src=/tmp,dst=/b4,ro,bind-recursive=readonly,bind-propagation=rprivate",
		"--mount", "type=bind,src=/tmp/{p}-new,dst=/b5,bind-create-src",
		"--mount", "type=bind,src=/tmp,dst=/b6,consistency=cached",
		"--mount", "type=bind,src=/tmp,dst=/b7,bind-create-src=false",
		alpine, "true"}},
	Case{Name: "mount-bind-odd-paths", Setup: [][]string{mkdirs("/h/{p} a:b,c", `/h/{p} "q" 'x' $HOME`)},
		Args: []string{
			"--mount", `type=bind,"source=/tmp/{p} a:b,c","target=/in:side,x"`,
			"--mount", `type=bind,"source=/tmp/{p} ""q"" 'x' $HOME",target=/q`,
			alpine, "true"}},
	Case{Name: "mount-volume", Setup: [][]string{{"volume", "create", "{p}-v"}},
		Args: []string{
			"--mount", "type=volume,src={p}-v,dst=/v1",
			"--mount", "src={p}-v,dst=/v2,readonly,volume-nocopy",
			"--mount", "type=volume,src={p}-v,dst=/v3,volume-subpath=sub",
			"--mount", "type=volume,dst=/anon",
			alpine, "true"},
		Teardown: [][]string{{"volume", "rm", "{p}-v"}}},
	Case{Name: "mount-volume-driver", Args: []string{
		"--mount", "type=volume,src={p}-d,dst=/d,volume-driver=local,volume-opt=type=tmpfs,volume-opt=device=tmpfs,volume-opt=o=size=1m,volume-label=b=2,volume-label=a=1",
		alpine, "true"},
		Teardown: [][]string{{"volume", "rm", "{p}-d"}}},
	Case{Name: "mount-tmpfs", Args: []string{
		"--mount", "type=tmpfs,dst=/t1",
		"--mount", "type=tmpfs,dst=/t2,tmpfs-size=1m,tmpfs-mode=1770",
		"--mount", "type=tmpfs,dst=/t3,tmpfs-size=12345,tmpfs-mode=700,readonly",
		alpine, "true"}},
	Case{Name: "mount-image", Args: []string{
		"--mount", "type=image,source=" + alpine + ",target=/img",
		"--mount", "type=image,source=" + alpine + ",target=/etc2,image-subpath=etc",
		alpine, "true"},
		Ignore: []Ignore{{"Mounts.*.Source", "image mounts get a per-container snapshot path"}}},
	Case{Name: "mount-and-volume", Setup: [][]string{{"volume", "create", "{p}-v"}},
		Args:     []string{"-v", "{p}-v:/v", "-v", "/anon", "--mount", "type=bind,src=/tmp,dst=/b", "--tmpfs", "/t", alpine, "true"},
		Teardown: [][]string{{"volume", "rm", "{p}-v"}}},

	// --tmpfs
	Case{Name: "tmpfs", Args: []string{"--tmpfs", "/t", "--tmpfs", "/run:rw,noexec,nosuid,size=65536k", "--tmpfs", "/a b:mode=1777", alpine, "true"}},

	// --volumes-from
	Case{Name: "volumes-from", Setup: [][]string{{"create", "--name={p}-from1", "-v", "/from", alpine}, {"create", "--name={p}-from2", "-v", "/from2", alpine}},
		Args:     []string{"--volumes-from", "{p}-from1", "--volumes-from", "{p}-from2:ro", alpine, "true"},
		Teardown: [][]string{{"rm", "-v", "{p}-from1", "{p}-from2"}}},
	Case{Name: "volumes-from-rw", Setup: [][]string{{"create", "--name={p}-from1", "-v", "/from", alpine}},
		Args:     []string{"--volumes-from", "{p}-from1:rw", alpine, "true"},
		Teardown: [][]string{{"rm", "-v", "{p}-from1"}}},

	// --volume-driver, --storage-opt
	Case{Name: "volume-driver", Args: []string{"--volume-driver", "local", "-v", "{p}-dv:/v", alpine, "true"},
		Teardown: [][]string{{"volume", "rm", "{p}-dv"}}},
	// overlay2 takes size= only on xfs with pquota (GitHub runners: ext4);
	// Lima's containerd snapshotter stores it. Saved: storage-opt below.
	Case{Name: "storage-opt", Args: []string{"--storage-opt", "size=1G", alpine, "true"},
		Refused: "--storage-opt is supported only for"},

	// --device
	Case{Name: "device", Args: []string{"--device", "/dev/null", "--device", "/dev/zero:/dev/z", "--device", "/dev/full:/dev/f:r", "--device", "/dev/random:rw", "--device", "/dev/urandom:/dev/urandom:rwm", alpine, "true"}},
	// --gpus and a CDI device: create accepts them without a GPU
	Case{Name: "cdi-gpu", Args: []string{"--gpus", "all", "--device", "nvidia.com/gpu=0", alpine, "true"}},
	Case{Name: "device-cdi", Args: []string{"--device", "vendor.com/class=dev0", "--device", "vendor.com/class=all", "--device", "/dev/null", alpine, "true"}},

	// --use-api-socket: with client credentials it adds the socket mount and
	// DOCKER_CONFIG (folded back to the flag); without, only the mount.
	Case{Name: "use-api-socket", Args: []string{"--use-api-socket", alpine, "true"}},
	Case{Name: "api-socket-plain-mount", Args: []string{"--mount", "type=bind,src=/var/run/docker.sock,dst=/var/run/docker.sock", "-e", "DOCKER_CONFIG=/run/secrets/docker", alpine, "true"}},

	// nothing set: nothing printed (with an image that declares volumes)
	Case{Name: "storage-default", Args: []string{alpine, "true"}},
)

// Saved data from Docker 29.9.3 on the Lima test VM (Linux):
//   - storage-image-volumes: image with VOLUME /iv /iv2, container made with
//     -v rt-save-v:/iv -v /extra -v '/tmp/x y:/b:ro' --mount type=volume,dst=/anon --tmpfs /t
//   - storage-api-socket: --use-api-socket, client with registry credentials
//   - storage-opt: docker create --storage-opt size=1G alpine:3.20 true
//
// Real, unedited, from Docker 29.9.3 on Windows Server 2022 (runlike-win):
//   - storage-windows-device: docker create --device=class/5B45201D-...
//     --device='vpci-location-path://PCIROOT(0)#PCI(0100)'; Devices hold
//     PathOnHost only, the value as given. Also live in cases_win_test.go.
var _ = addSaved(
	SavedCase{Name: "storage-image-volumes", Want: []string{"--volume=/extra", "--volume=/tmp/x y:/b:ro"}},
	SavedCase{Name: "storage-api-socket", Want: []string{"--use-api-socket"}},
	SavedCase{Name: "storage-opt", Want: []string{"--storage-opt=size=1G"}},
	SavedCase{Name: "storage-windows-device", Want: []string{"--device=class/5B45201D-F2F2-4F3B-85BB-30FF1F953599", "--device=vpci-location-path://PCIROOT(0)#PCI(0100)"}},
)

// TestStorageExact checks the full set of storage arguments for saved data,
// so it also catches what must NOT be printed (image VOLUMEs, the parts of
// --use-api-socket), which a round trip cannot see.
func TestStorageExact(t *testing.T) {
	isStorage := func(a string) bool {
		for _, p := range []string{"--volume", "--mount=", "--tmpfs=", "--volumes-from=", "--storage-opt=", "--device=", "--use-api-socket", "--env=DOCKER_CONFIG="} {
			if strings.HasPrefix(a, p) {
				return true
			}
		}
		return false
	}
	for _, c := range []struct {
		name    string
		noImage bool // --stdin mode
		opts    runlike.Options
		want    []string
	}{
		{"storage-image-volumes", false, runlike.Options{}, []string{
			"--volume=rt-save-v:/iv", "--volume=/tmp/x y:/b:ro", "--volume=/extra",
			"--mount=type=volume,target=/anon", "--tmpfs=/t"}},
		// Without image data /iv2 cannot be told from -v /iv2, so it is kept;
		// /iv is not repeated (rt-save-v is mounted there).
		{"storage-image-volumes", true, runlike.Options{}, []string{
			"--volume=rt-save-v:/iv", "--volume=/tmp/x y:/b:ro", "--volume=/extra", "--volume=/iv2",
			"--mount=type=volume,target=/anon", "--tmpfs=/t"}},
		{"storage-image-volumes", false, runlike.Options{UseVolumeID: true}, []string{
			"--volume=rt-save-v:/iv", "--volume=/tmp/x y:/b:ro",
			"--volume=3540d1ab923d734b0c03cd6a3de393cba2d7b988ccfe0fcacb906473ddf3c2b8:/extra",
			"--volume=0a1328a5409474730634833f3d7f7b1f9c69083f5a00e1ff208f1c0bed13fee7:/iv2",
			"--mount=type=volume,source=4d2cb052eba3bea77e7f9d514951804ce48e36af5695d33c0f3ea62e9ca24059,target=/anon", "--tmpfs=/t"}},
		{"storage-api-socket", false, runlike.Options{}, []string{"--use-api-socket"}},
	} {
		cj, err := os.ReadFile(filepath.Join("testdata", c.name+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var ij []byte
		if !c.noImage {
			ij, _ = os.ReadFile(filepath.Join("testdata", c.name+".image.json"))
		}
		rc, err := runlike.Args(runlike.Inspect{Container: cj, Image: ij}, runlike.Daemon{}, c.opts)
		if err != nil {
			t.Fatal(err)
		}
		got := slices.DeleteFunc(rc.Options, func(a string) bool { return !isStorage(a) })
		if !slices.Equal(got, c.want) {
			t.Errorf("%s (stdin %v, %+v):\n got %q\nwant %q", c.name, c.noImage, c.opts, got, c.want)
		}
	}
}

// A tmpfs path holding ":" (API only): docker run splits path from
// options there, so runlike warns.
var _ = addSteps(stepCase{name: "tmpfs-colon-api", args: []string{alpine, "true"}, noCopy: true,
	api:  `{"HostConfig":{"Tmpfs":{"/a:b":"rw,noexec"}}}`,
	want: []string{"--tmpfs=/a:b:rw,noexec"}, warn: []string{`--tmpfs path "/a:b" holds ":"`}})
