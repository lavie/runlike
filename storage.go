package runlike

import (
	"encoding/csv"
	"slices"
	"strconv"
	"strings"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
)

// What --use-api-socket adds (docker/cli v29.9.3 cli/command/container/create.go):
// a bind mount of the engine socket with empty, non-nil BindOptions, and,
// when the client has registry credentials, DOCKER_CONFIG pointing at the
// config file it copies into the container after create.
const (
	apiSocket    = "/var/run/docker.sock"
	apiSocketEnv = "DOCKER_CONFIG=/run/secrets/docker"
)

// storageArgs: --volume, --mount, --tmpfs, --volumes-from, --volume-driver,
// --storage-opt, --device, --use-api-socket.
//
// -v entries live in HostConfig.Binds (stored as typed, with :ro,z,nocopy,
// propagation; relative paths made absolute by the CLI), --mount entries in
// HostConfig.Mounts, so each comes back in the form the user chose.
// Anonymous -v /path volumes are in Config.Volumes, which the daemon merges
// with the image's VOLUME list.
func storageArgs(x *ctx) []string {
	var a []string
	if x.apiSocket {
		a = append(a, "--use-api-socket")
	}
	for _, b := range x.hc.Binds {
		a = append(a, "--volume="+b)
	}
	a = append(a, x.anonVolumes()...)
	for _, m := range x.hc.Mounts {
		if x.apiSocket && isAPISocketMount(m) {
			continue
		}
		if x.opts.UseVolumeID && m.Type == mount.TypeVolume && m.Source == "" {
			m.Source = x.volumeName(m.Target)
		}
		if v := m.VolumeOptions; v != nil {
			x.eqKeys("--mount volume-label", v.Labels)
			if v.DriverConfig != nil {
				x.eqKeys("--mount volume-opt", v.DriverConfig.Options)
			}
		}
		a = append(a, "--mount="+mountArg(m))
	}
	for _, p := range sortedKeys(x.hc.Tmpfs) {
		if strings.Contains(p, ":") {
			x.warn("--tmpfs path %q holds \":\", where docker run splits path from options: the copy mounts at %q instead", p, p[:strings.Index(p, ":")])
		}
		if o := x.hc.Tmpfs[p]; o != "" {
			p += ":" + o
		}
		a = append(a, "--tmpfs="+p)
	}
	for _, v := range x.hc.VolumesFrom {
		a = append(a, "--volumes-from="+v)
	}
	if x.hc.VolumeDriver != "" {
		a = append(a, "--volume-driver="+x.hc.VolumeDriver)
	}
	x.eqKeys("--storage-opt", x.hc.StorageOpt)
	for _, k := range sortedKeys(x.hc.StorageOpt) {
		a = append(a, "--storage-opt="+k+"="+x.hc.StorageOpt[k])
	}
	for _, d := range x.hc.Devices {
		a = append(a, "--device="+x.deviceArg(d))
	}
	// CDI names given to --device land in a device request with driver cdi.
	for _, r := range x.hc.DeviceRequests {
		if r.Driver == "cdi" && len(r.Capabilities) == 0 {
			for _, id := range r.DeviceIDs {
				a = append(a, "--device="+id)
			}
		}
	}
	return a
}

// anonVolumes prints the anonymous volumes added with -v /path: entries of
// Config.Volumes the image does not declare (all of them in --stdin mode),
// skipping paths a -v, --mount or --tmpfs entry already covers.
// With --use-volume-id, every anonymous volume (image ones too) keeps its
// generated name, as Python 1.6.3 did, so the copy reuses the data.
func (x *ctx) anonVolumes() []string {
	var a []string
	for _, p := range sortedKeys(x.cfg.Volumes) {
		n, ok := x.anonVolumeAt(p)
		switch {
		case !ok:
		case x.opts.UseVolumeID && n != "":
			a = append(a, "--volume="+n+":"+p)
		case x.icfg != nil && hasKey(x.icfg.Volumes, p):
		default:
			a = append(a, "--volume="+p)
		}
	}
	return a
}

func hasKey[V any](m map[string]V, k string) bool { _, ok := m[k]; return ok }

// anonVolumeAt reports whether p holds no -v, --mount or --tmpfs entry,
// and the generated name of the volume there, if any.
func (x *ctx) anonVolumeAt(p string) (string, bool) {
	if hasKey(x.hc.Tmpfs, p) || slices.ContainsFunc(x.hc.Mounts, func(m mount.Mount) bool { return m.Target == p }) {
		return "", false
	}
	for _, m := range x.c.Mounts {
		if m.Destination != p {
			continue
		}
		if m.Type != mount.TypeVolume || slices.ContainsFunc(x.hc.Binds, func(b string) bool { return strings.HasPrefix(b, m.Name+":") }) {
			return "", false
		}
		return m.Name, true
	}
	return "", true
}

// volumeName returns the name of the volume mounted at dst, if any.
func (x *ctx) volumeName(dst string) string {
	for _, m := range x.c.Mounts {
		if m.Type == mount.TypeVolume && m.Destination == dst {
			return m.Name
		}
	}
	return ""
}

// mountArg inverts the CLI's --mount parser (opts/mount.go): one CSV
// record of key=value fields, quoted where a value holds a comma or quote.
func mountArg(m mount.Mount) string {
	f := []string{"type=" + string(m.Type)}
	add := func(k, v string) {
		if v != "" {
			f = append(f, k+"="+v)
		}
	}
	add("source", m.Source)
	add("target", m.Target)
	if m.ReadOnly {
		f = append(f, "readonly")
	}
	add("consistency", string(m.Consistency))
	if b := m.BindOptions; b != nil {
		add("bind-propagation", string(b.Propagation))
		switch {
		case b.NonRecursive:
			add("bind-recursive", "disabled")
		case b.ReadOnlyNonRecursive:
			add("bind-recursive", "writable")
		case b.ReadOnlyForceRecursive:
			add("bind-recursive", "readonly")
		}
		if b.CreateMountpoint {
			f = append(f, "bind-create-src")
		}
		if *b == (mount.BindOptions{}) {
			// Only an explicit false sets empty, non-nil BindOptions.
			add("bind-create-src", "false")
		}
	}
	if v := m.VolumeOptions; v != nil {
		add("volume-subpath", v.Subpath)
		if v.NoCopy {
			f = append(f, "volume-nocopy")
		}
		for _, k := range sortedKeys(v.Labels) {
			add("volume-label", k+"="+v.Labels[k])
		}
		if d := v.DriverConfig; d != nil {
			add("volume-driver", d.Name)
			for _, k := range sortedKeys(d.Options) {
				add("volume-opt", k+"="+d.Options[k])
			}
		}
	}
	if i := m.ImageOptions; i != nil {
		add("image-subpath", i.Subpath)
	}
	if t := m.TmpfsOptions; t != nil {
		if t.SizeBytes != 0 {
			add("tmpfs-size", strconv.FormatInt(t.SizeBytes, 10))
		}
		if t.Mode != 0 {
			add("tmpfs-mode", strconv.FormatUint(uint64(t.Mode), 8))
		}
	}
	var sb strings.Builder
	w := csv.NewWriter(&sb)
	w.Write(f)
	w.Flush()
	return strings.TrimSuffix(sb.String(), "\n")
}

// deviceArg inverts parseDevice: HOST[:CONTAINER][:PERMS], leaving out
// CONTAINER when it equals HOST and PERMS when they are the default rwm.
// Windows stores the value as given (class/GUID or IDType://ID).
func (x *ctx) deviceArg(d container.DeviceMapping) string {
	s := d.PathOnHost
	if x.OS == "windows" {
		return s
	}
	if d.PathInContainer != "" && d.PathInContainer != d.PathOnHost {
		s += ":" + d.PathInContainer
	}
	if d.CgroupPermissions != "" && d.CgroupPermissions != "rwm" {
		s += ":" + d.CgroupPermissions
	}
	return s
}

func isAPISocketMount(m mount.Mount) bool {
	return m.Type == mount.TypeBind && m.Source == apiSocket && m.Target == apiSocket &&
		!m.ReadOnly && m.BindOptions != nil && *m.BindOptions == (mount.BindOptions{})
}

// foldAPISocket detects --use-api-socket (its socket mount plus the
// DOCKER_CONFIG it sets; ruling in AGENTS.md). It sets x.apiSocket and
// hides that variable from the env so --env does not print it. Without the
// variable (no credentials on the client) the mount prints as --mount.
func foldAPISocket(x *ctx) {
	if !slices.Contains(x.cfg.Env, apiSocketEnv) || !slices.ContainsFunc(x.hc.Mounts, isAPISocketMount) {
		return
	}
	cfg := *x.cfg
	cfg.Env = slices.DeleteFunc(slices.Clone(cfg.Env), func(e string) bool { return e == apiSocketEnv })
	x.cfg, x.apiSocket = &cfg, true
}
