package runlike

import (
	"strconv"
	"strings"
)

// runtimeArgs: namespaces, restart policy, --rm, runtime, isolation and
// platform.
func runtimeArgs(x *ctx) []string {
	var a []string
	hc := x.hc
	// The daemon fills cgroupns (private on cgroup v2, Docker's default) and
	// ipc (private) on create; a daemon configured otherwise stores its own
	// value, which is printed so the line works on any daemon, and the
	// built-in value is printed on a daemon whose default differs.
	// A Windows daemon stores none of these unless given (seen on 29.9.3),
	// so there every stored value is printed.
	dd := x.d.Defaults
	win := x.OS == "windows"
	if m := string(hc.CgroupnsMode); m != "" && (win || !isDefault(m, builtinCgroupns, dd.CgroupnsMode)) {
		a = append(a, "--cgroupns="+m)
	}
	// container:<ID> targets print by name when the CLI knows it.
	if m := string(hc.IpcMode); m != "" && (win || !isDefault(m, builtinIpcMode, dd.IpcMode)) {
		a = append(a, "--ipc="+x.containerRef(m))
	}
	if hc.PidMode != "" {
		a = append(a, "--pid="+x.containerRef(string(hc.PidMode)))
	}
	if hc.UTSMode != "" {
		a = append(a, "--uts="+string(hc.UTSMode))
	}
	// Windows only. A Windows daemon stores its default when none is given:
	// process on Windows Server, hyperv on Windows 10/11, so there is no
	// one built-in default; it is left out only when it equals this
	// daemon's (`docker info` Isolation, CLI mode). A Linux daemon accepts
	// only "default".
	if i := string(hc.Isolation); x.OS == "windows" && i != "" && i != "default" && i != dd.Isolation {
		a = append(a, "--isolation="+string(hc.Isolation))
	}
	if p := x.platform(); p != "" {
		a = append(a, "--platform="+p)
	}
	switch rp := hc.RestartPolicy; rp.Name {
	case "", "no":
	case "on-failure":
		if rp.MaximumRetryCount > 0 {
			a = append(a, "--restart=on-failure:"+strconv.Itoa(rp.MaximumRetryCount))
		} else {
			a = append(a, "--restart=on-failure")
		}
	default:
		a = append(a, "--restart="+string(rp.Name))
	}
	if hc.AutoRemove {
		a = append(a, "--rm")
	}
	// A Windows daemon reports no default runtime and stores none on
	// create, then sets io.containerd.runhcs.v1 on start (29.9.3).
	switch r := hc.Runtime; {
	case r == "", win && r == builtinRuntimeWindows, !win && isDefault(r, builtinRuntime, dd.Runtime):
	default:
		a = append(a, "--runtime="+r)
	}
	return a
}

// platform returns os/arch[/variant] of the image manifest the container
// was created from, when it differs from the daemon's native platform.
func (x *ctx) platform() string {
	d := x.c.ImageManifestDescriptor
	if d == nil || d.Platform == nil || d.Platform.OS == "" || d.Platform.Architecture == "" {
		return ""
	}
	p := d.Platform
	// Empty Daemon.Platform (--stdin) means the native platform is unknown:
	// print nothing.
	if x.d.Platform == "" {
		return ""
	}
	s := p.OS + "/" + p.Architecture
	if v := normVariant(p.Architecture, p.Variant); v != "" {
		s += "/" + v
	}
	if s == x.d.Platform {
		return ""
	}
	return s
}

// normVariant treats arm64's default variant v8 as no variant.
func normVariant(arch, variant string) string {
	if arch == "arm64" && strings.EqualFold(variant, "v8") {
		return ""
	}
	return variant
}
