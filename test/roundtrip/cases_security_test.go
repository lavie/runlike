package roundtrip

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/lavie/runlike/v2"
)

// Security area: capabilities, privileges, security options, sysctls,
// user, user namespace.
var _ = add("security",
	Case{Name: "default", Args: []string{alpine, "true"}},
	// capabilities (the daemon stores them as CAP_X, upper case)
	Case{Name: "cap-add", Args: []string{"--cap-add=net_admin", "--cap-add=CAP_SYS_TIME", alpine, "true"}},
	Case{Name: "cap-drop", Args: []string{"--cap-drop=chown", "--cap-drop=CAP_NET_RAW", alpine, "true"}},
	Case{Name: "cap-drop-all-add", Args: []string{"--cap-drop=ALL", "--cap-add=NET_BIND_SERVICE", alpine, "true"}},
	Case{Name: "cap-add-all", Args: []string{"--cap-add=all", "--cap-drop=cap_chown", alpine, "true"}},
	// privileged: the daemon adds label=disable itself; must not be doubled
	Case{Name: "privileged", Args: []string{"--privileged", alpine, "true"}},
	Case{Name: "privileged-label-disable", Args: []string{"--privileged", "--security-opt=label=disable", alpine, "true"}},
	Case{Name: "privileged-cap-add", Args: []string{"--privileged", "--cap-add=SYS_ADMIN", "--security-opt=systempaths=unconfined", alpine, "true"}},
	Case{Name: "read-only", Args: []string{"--read-only", alpine, "true"}},
	// security options
	Case{Name: "secopt-various", Args: []string{"--security-opt=seccomp=unconfined", "--security-opt=apparmor=unconfined",
		"--security-opt=no-new-privileges", "--security-opt=label=disable", alpine, "true"}},
	Case{Name: "secopt-nnp-true", Args: []string{"--security-opt=no-new-privileges=true", "--security-opt", "seccomp:builtin", alpine, "true"}},
	Case{Name: "secopt-label-user", Args: []string{"--security-opt=label=user:USER", "--security-opt=label=level:s0:c100,c200", alpine, "true"}},
	Case{Name: "secopt-systempaths", Args: []string{"--security-opt=systempaths=unconfined", alpine, "true"}},
	Case{Name: "secopt-label-pid-host", Args: []string{"--pid=host", "--security-opt=label=disable", alpine, "true"}},
	// sysctls (namespaced ones only; the daemon rejects others at start)
	Case{Name: "sysctl", Args: []string{"--sysctl=net.ipv4.ip_forward=1", "--sysctl=kernel.msgmax=65536",
		"--sysctl=net.ipv4.ip_local_port_range=1024 65000", alpine, "true"}},
	Case{Name: "group-add", Args: []string{"--group-add=audio", "--group-add=1234", "--group-add=no such", alpine, "true"}},
	Case{Name: "device-cgroup-rule", Args: []string{"--device-cgroup-rule=c 1:3 mr", "--device-cgroup-rule=a *:* rwm", alpine, "true"}},
	// user: left out when equal to the image's USER
	Case{Name: "user", Args: []string{"--user=nobody", alpine, "true"}},
	Case{Name: "user-uid-gid", Args: []string{"-u", "1000:1000", alpine, "true"}},
	Case{Name: "user-odd", Args: []string{"--user=we ird:$(id)", alpine, "true"}},
	Case{Name: "user-image", Setup: userImage, Args: []string{"{p}-img"}, Teardown: [][]string{{"rmi", "{p}-img"}}},
	Case{Name: "user-image-same", Setup: userImage, Args: []string{"--user=nobody", "{p}-img"}, Teardown: [][]string{{"rmi", "{p}-img"}}},
	Case{Name: "user-image-override", Setup: userImage, Args: []string{"--user=root", "{p}-img"}, Teardown: [][]string{{"rmi", "{p}-img"}}},
	Case{Name: "userns-host", Args: []string{"--userns=host", alpine, "true"}},
	Case{Name: "combo", Args: []string{"--cap-drop=ALL", "--cap-add=CHOWN", "--read-only", "--user=1:2",
		"--security-opt=no-new-privileges", "--sysctl=net.core.somaxconn=1024", "--group-add=wheel", alpine, "true"}},
)

// Saved from docker create on Docker 29.9.3 (Lima, linux/arm64) with
// --security-opt seccomp=FILE: the daemon keeps the file's compact JSON,
// which runlike prints as stored, with a warning (docker run then fails
// loudly, see securityOpts in security.go).
var _ = addSaved(SavedCase{Name: "security-seccomp-file", Want: []string{
	`--security-opt=seccomp={"defaultAction":"SCMP_ACT_ALLOW","syscalls":[{"names":["mkdir"],"action":"SCMP_ACT_ERRNO"}]}`,
	"--security-opt=no-new-privileges",
}})

// userImage builds {p}-img: alpine with USER nobody and CMD true.
var userImage = [][]string{
	{"create", "--name", "{p}-src", alpine, "true"},
	{"commit", "--change", "USER nobody", "--change", `CMD ["true"]`, "{p}-src", "{p}-img"},
	{"rm", "{p}-src"},
}

// The old colon form label:... does not stop the daemon adding its own
// label=disable, which must not be printed back.
var _ = addSteps(stepCase{name: "privileged-label-colon", args: []string{"--privileged", "--security-opt", "label:disable", alpine, "true"},
	want: []string{"--security-opt=label:disable"}, notWant: []string{"--security-opt=label=disable"}})

func TestSeccompFileWarning(t *testing.T) {
	cj, err := os.ReadFile("testdata/security-seccomp-file.json")
	if err != nil {
		t.Fatal(err)
	}
	rc, err := runlike.Args(runlike.Inspect{Container: cj}, runlike.Daemon{}, runlike.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(rc.Warnings, func(w string) bool { return strings.Contains(w, "save the JSON") }) {
		t.Errorf("no seccomp file warning in %q", rc.Warnings)
	}
}

// Masked and read-only paths added through the API: docker run cannot
// set them, so runlike warns (the copy would lose that hardening).
var _ = addSteps(stepCase{name: "masked-paths-api", args: []string{alpine, "true"}, noCopy: true,
	api:  `{"HostConfig":{"MaskedPaths":["/proc/acpi","/etc/hostname"],"ReadonlyPaths":["/proc/sys","/etc"]}}`,
	warn: []string{"HostConfig.MaskedPaths adds /etc/hostname", "HostConfig.ReadonlyPaths adds /etc"}})
