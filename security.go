package runlike

import (
	"slices"
	"strings"
)

// securityArgs: capabilities, privileges, security options, sysctls,
// user and user namespace.
func securityArgs(x *ctx) []string {
	var a []string
	hc := x.hc
	// The daemon stores capabilities normalised (CAP_ prefix, upper case,
	// "ALL"); docker run accepts that form back.
	for _, c := range hc.CapAdd {
		a = append(a, "--cap-add="+c)
	}
	for _, c := range hc.CapDrop {
		a = append(a, "--cap-drop="+c)
	}
	for _, r := range hc.DeviceCgroupRules {
		a = append(a, "--device-cgroup-rule="+r)
	}
	for _, g := range hc.GroupAdd {
		a = append(a, "--group-add="+g)
	}
	if hc.Privileged {
		a = append(a, "--privileged")
	}
	if hc.ReadonlyRootfs {
		a = append(a, "--read-only")
	}
	for _, o := range x.securityOpts() {
		a = append(a, "--security-opt="+o)
	}
	x.eqKeys("--sysctl", hc.Sysctls)
	for _, k := range sortedKeys(hc.Sysctls) {
		a = append(a, "--sysctl="+k+"="+hc.Sysctls[k])
	}
	// The daemon copies the image's USER into Config.User when -u is not
	// given, so only a different value comes from -u.
	if u := x.cfg.User; u != "" && u != x.imageCfg().User {
		a = append(a, "--user="+u)
	}
	if hc.UsernsMode != "" {
		a = append(a, "--userns="+string(hc.UsernsMode))
	}
	return a
}

// securityOpts returns the --security-opt values to print.
//
//   - The daemon adds "label=disable" by itself to a privileged container
//     and to one with --ipc=host or --pid=host, unless the user gave a
//     "label=" option (the old "label:" form does not count: seen on
//     29.9.3); it adds it again on recreate, so leave it out then.
//   - --security-opt=systempaths=unconfined is handled by the CLI: it is
//     stored as empty (not null) MaskedPaths and ReadonlyPaths.
//   - seccomp=FILE is stored as "seccomp=<the file's compact JSON>". The
//     file is gone, and docker run reads the value as a file name, so this
//     prints the JSON as stored: docker run then fails loudly ("opening
//     seccomp profile ... failed") instead of silently running without the
//     profile. Save the JSON to a file and pass its path to recreate.
func (x *ctx) securityOpts() []string {
	hc := x.hc
	opts := hc.SecurityOpt
	var labels []string
	for _, o := range opts {
		if strings.HasPrefix(o, "label=") {
			labels = append(labels, o)
		}
	}
	if (hc.Privileged || hc.IpcMode.IsHost() || hc.PidMode.IsHost()) &&
		len(labels) == 1 && labels[0] == "label=disable" {
		var kept []string
		for _, o := range opts {
			if o != "label=disable" {
				kept = append(kept, o)
			}
		}
		opts = kept
	}
	for _, o := range opts {
		if strings.HasPrefix(o, "seccomp={") {
			x.warn("--security-opt=seccomp=... holds the profile's JSON, but docker run expects a file name: save the JSON after \"seccomp=\" to a file and pass its path instead")
		}
	}
	if hc.MaskedPaths != nil && len(hc.MaskedPaths) == 0 &&
		hc.ReadonlyPaths != nil && len(hc.ReadonlyPaths) == 0 {
		opts = append(opts, "systempaths=unconfined")
	} else {
		// The API can add paths; docker run cannot, so the copy would lose
		// them. Fewer paths (older daemons' defaults) only make it stricter.
		for _, l := range []struct {
			field string
			paths []string
			def   []string
		}{{"MaskedPaths", hc.MaskedPaths, builtinMaskedPaths}, {"ReadonlyPaths", hc.ReadonlyPaths, builtinReadonlyPaths}} {
			var more []string
			for _, p := range l.paths {
				if !slices.Contains(l.def, p) {
					more = append(more, p)
				}
			}
			if len(more) > 0 {
				x.warn("HostConfig.%s adds %s, which docker run cannot set: the copy leaves them out", l.field, strings.Join(more, ", "))
			}
		}
	}
	return opts
}
