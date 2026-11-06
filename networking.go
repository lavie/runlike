package runlike

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/moby/moby/api/types/network"
)

// networkingArgs: host name, DNS, networks and their endpoint settings,
// links, published and exposed ports.
func networkingArgs(x *ctx) []string {
	var a []string
	mode := string(x.hc.NetworkMode)

	// The daemon sets the host name to the ID prefix; in host mode it
	// copies the daemon host's name, so a value that differs from that came
	// from --hostname; container: mode rejects it.
	switch h := x.cfg.Hostname; {
	case h == "" || strings.HasPrefix(mode, "container:"):
	case mode == "host" && x.OS != "windows": // Windows keeps the ID prefix
		if x.d.HostName == "" {
			x.warn("--network=host: no daemon host name to compare with, so host name %q (the host's, or set with --hostname) is left out", h)
		} else if h != x.d.HostName {
			a = append(a, "--hostname="+h)
		}
	case !(len(x.c.ID) >= 12 && h == x.c.ID[:12]):
		a = append(a, "--hostname="+h)
	}
	if x.cfg.Domainname != "" {
		a = append(a, "--domainname="+x.cfg.Domainname)
	}
	for _, d := range x.hc.DNS {
		a = append(a, "--dns="+d.String())
	}
	for _, d := range x.hc.DNSOptions {
		a = append(a, "--dns-option="+d)
	}
	for _, d := range x.hc.DNSSearch {
		a = append(a, "--dns-search="+d)
	}
	for _, h := range x.hc.ExtraHosts {
		a = append(a, "--add-host="+h)
	}
	a = append(a, x.networkArgs(mode)...)
	// Legacy links (default bridge) are stored as "/target:/self/alias".
	for _, l := range x.hc.Links {
		target, alias, _ := strings.Cut(l, ":")
		target = strings.TrimPrefix(target, "/")
		alias = alias[strings.LastIndex(alias, "/")+1:]
		if alias != target {
			target += ":" + alias
		}
		a = append(a, "--link="+target)
	}
	if mode == "host" { // Docker discards published ports in host mode
		if len(x.hc.PortBindings) > 0 || x.hc.PublishAllPorts {
			x.warn("--network=host: published ports left out; Docker discards them in host mode")
		}
	} else {
		a = append(a, publishArgs(x.hc.PortBindings)...)
		if x.hc.PublishAllPorts {
			a = append(a, "--publish-all")
		}
	}
	if !strings.HasPrefix(mode, "container:") { // that mode rejects --expose
		a = append(a, x.exposeArgs()...)
	}
	return a
}

// networkArgs prints the container's networks. The first --network is
// HostConfig.NetworkMode and takes the plain flags (--network-alias, --ip,
// --ip6, --link-local-ip, --mac-address, --link), which keep upper case.
// Every further network uses the advanced form --network=name=N,alias=...;
// the docker CLI lowercases that whole value, and it has no per-network
// link field: both cases are warned about with the `docker network
// connect` command that restores them. The default bridge cannot be
// combined with other networks in docker run, so those are left out and
// given as connect commands too.
//
// MAC addresses: only a pinned MAC is printed. A created container
// reports its pinned MACs per network; a running one the MACs it has now
// (random or pinned, the network fields cannot tell); a stopped one none,
// except on a network connected while stopped. The MAC pinned on the first
// network comes from API 1.51's Config.MacAddress (x.mac); for further
// networks no API version reports it (moby keeps it in DesiredMacAddress,
// never shown), so a started container's are warned about once.
func (x *ctx) networkArgs(mode string) []string {
	if peer, ok := strings.CutPrefix(mode, "container:"); ok {
		if n := x.d.ContainerNames[peer]; n != "" {
			peer = n
		}
		return []string{"--network=container:" + peer}
	}
	if mode == "" && x.c.NetworkSettings == nil {
		return nil
	}
	var nets map[string]*network.EndpointSettings
	if x.c.NetworkSettings != nil {
		nets = x.c.NetworkSettings.Networks
	}
	// Daemons before API 1.45 (Docker 26) add the container's short ID to
	// Aliases on user networks; a copy gets its own, so leave it out.
	if len(x.c.ID) >= 12 {
		for _, e := range nets {
			if e != nil {
				e.Aliases = slices.DeleteFunc(slices.Clone(e.Aliases), func(al string) bool { return al == x.c.ID[:12] })
			}
		}
	}
	// "default" is bridge on Linux, nat on Windows (the daemon stores nat).
	def := builtinNetwork
	if x.OS == "windows" {
		def = builtinNetworkWindows
	}
	primary := mode
	if primary == "" || primary == "default" {
		primary = def
		if mode == "" && nets[primary] == nil && len(nets) > 0 { // saved data without NetworkMode
			primary = sortedKeys(nets)[0]
		}
	}
	ep := nets[primary]
	if ep == nil && mode != "" {
		for _, name := range sortedKeys(nets) { // NetworkMode may hold a network ID
			if e := nets[name]; e != nil && e.NetworkID != "" && strings.HasPrefix(e.NetworkID, mode) {
				primary, ep = name, e
				break
			}
		}
	}
	if ep == nil && mode != "" {
		// Created with --network <full ID>: listed under the name, no NetworkID yet.
		if n := x.d.NetworkNames[mode]; nets[n] != nil {
			primary, ep = n, nets[n]
		} else if len(nets) == 1 {
			for n, e := range nets {
				primary, ep = n, e
			}
		}
	}
	if ep == nil {
		ep = &network.EndpointSettings{}
	}
	// Given by ID: once started, inspect also lists the network by name.
	if ep.NetworkID != "" && strings.HasPrefix(ep.NetworkID, primary) {
		for _, name := range sortedKeys(nets) {
			if e := nets[name]; e != nil && e.NetworkID == ep.NetworkID && !strings.HasPrefix(e.NetworkID, name) {
				primary = name
				break
			}
		}
	}
	builtin := primary == def || primary == "none" || primary == "host"
	st := x.c.State
	running := st != nil && st.Running

	// Further networks docker run cannot express go to connect, with why.
	var a, extra, connect, why []string
	// pinned: e's MAC is the one set by the user (see above).
	pinned := func(e *network.EndpointSettings) bool { return !running && len(e.MacAddress) > 0 }
	because := func(r string) {
		if !slices.Contains(why, r) {
			why = append(why, r)
		}
	}
	for _, name := range sortedKeys(nets) {
		if e := nets[name]; e != nil {
			x.eqKeys("--network driver-opt", e.DriverOpts)
		}
	}
	for _, name := range sortedKeys(nets) {
		e := nets[name]
		// Started containers attached by network ID list the network
		// twice, under the ID and under its name.
		if name == primary || e == nil || e.NetworkID != "" && e.NetworkID == ep.NetworkID {
			continue
		}
		if !pinned(e) {
			e.MacAddress = nil
		}
		f := endpointFields(e)
		switch {
		case builtin:
			because("docker run cannot join the " + primary + " network and others at once")
		case len(f) > 0 && hasUpper("", f):
			because("docker run lowercases the whole --network=name=... value")
		case len(e.Links) > 0:
			because("docker run cannot set links on a further network")
		default:
			if v, r := x.netValue(name, e.NetworkID, f); r != "" {
				because(r)
			} else {
				extra = append(extra, "--network="+v)
				continue
			}
		}
		connect = append(connect, x.connectCmd(name, e))
		if len(e.MacAddress) > 0 {
			because("docker network connect cannot set network " + name + "'s MAC address " + e.MacAddress.String())
		}
	}
	if len(connect) > 0 {
		x.warn("networks left out (%s); after creating the copy, run (POSIX shell quoting):\n  %s", strings.Join(why, "; "), raw(strings.Join(connect, "\n  ")))
	}
	// A started container's further networks (and, without API 1.51
	// data or when NetworkMode is a network ID, its first network) do not
	// say whether their MAC was pinned (API 1.51 looks the pinned MAC up
	// under NetworkMode, but a started container lists the network by name):
	// Docker keeps that only internally. runlike prints no MAC there and
	// does not warn, which would fire on nearly every multi-network
	// container (Assaf, 2026-11-06). README "Known limits" documents it.
	mac := x.mac
	if mac == "" && pinned(ep) {
		mac = ep.MacAddress.String()
	}
	var adv []string // fields only the advanced form can carry
	for _, k := range sortedKeys(ep.DriverOpts) {
		adv = append(adv, "driver-opt="+k+"="+ep.DriverOpts[k])
	}
	if ep.GwPriority != 0 {
		adv = append(adv, "gw-priority="+strconv.Itoa(ep.GwPriority))
	}
	if len(adv) > 0 || primary != def || len(extra) > 0 {
		if hasUpper("", adv) {
			x.warn("docker run lowercases the --network=name=... form, so upper case in network %s's driver options is lost", primary)
		}
		v, r := x.netValue(primary, ep.NetworkID, adv)
		if r != "" {
			// The copy joins the default network, or with extra networks
			// the first of those; the commands attach it to this one.
			cmds := x.connectCmd(primary, ep)
			if len(extra) == 0 {
				cmds = posixCmd("docker", "network", "disconnect", def, x.copyName()) + "\n  " + cmds
			}
			if mac != "" {
				r += "; docker network connect cannot set its MAC address " + mac
			}
			x.warn("network %s left out (%s); after creating the copy, run (POSIX shell quoting):\n  %s", primary, r, raw(cmds))
			return append(a, extra...)
		}
		a = append(a, "--network="+v)
	}
	for _, al := range ep.Aliases {
		a = append(a, "--network-alias="+al)
	}
	if c := ep.IPAMConfig; c != nil {
		if c.IPv4Address.IsValid() {
			a = append(a, "--ip="+c.IPv4Address.String())
		}
		if c.IPv6Address.IsValid() {
			a = append(a, "--ip6="+c.IPv6Address.String())
		}
		for _, ip := range c.LinkLocalIPs {
			a = append(a, "--link-local-ip="+ip.String())
		}
	}
	if mac != "" {
		a = append(a, "--mac-address="+mac)
	}
	for _, l := range ep.Links {
		a = append(a, "--link="+l)
	}
	return append(a, extra...)
}

// connectCmd returns the `docker network connect` command that attaches
// the copy to network name with e's settings (it keeps upper case).
func (x *ctx) connectCmd(name string, e *network.EndpointSettings) string {
	cmd := []string{"docker", "network", "connect"}
	for _, al := range e.Aliases {
		cmd = append(cmd, "--alias="+al)
	}
	if c := e.IPAMConfig; c != nil {
		if c.IPv4Address.IsValid() {
			cmd = append(cmd, "--ip="+c.IPv4Address.String())
		}
		if c.IPv6Address.IsValid() {
			cmd = append(cmd, "--ip6="+c.IPv6Address.String())
		}
		for _, ip := range c.LinkLocalIPs {
			cmd = append(cmd, "--link-local-ip="+ip.String())
		}
	}
	for _, k := range sortedKeys(e.DriverOpts) {
		cmd = append(cmd, "--driver-opt="+k+"="+e.DriverOpts[k])
	}
	if e.GwPriority != 0 {
		cmd = append(cmd, "--gw-priority="+strconv.Itoa(e.GwPriority))
	}
	for _, l := range e.Links {
		cmd = append(cmd, "--link="+l)
	}
	return posixCmd(append(cmd, name, x.copyName())...)
}

// copyName is the copy's name in suggested commands.
func (x *ctx) copyName() string {
	c := strings.TrimPrefix(x.c.Name, "/")
	if c == "" || x.opts.NoName {
		c = "CONTAINER"
	}
	return c
}

// posixCmd quotes a suggested command for a POSIX shell (warnings say so),
// on one line: quotePosix keeps a newline literal inside single quotes, so
// it is written as $'\n' (POSIX.1-2024; bash, zsh, ksh, busybox) instead.
// dash 0.5.12 reads it as "$\n": the command then names no such network
// and fails, which is safe.
func posixCmd(cmd ...string) string {
	for i, s := range cmd {
		q, _ := quotePosix(s)
		cmd[i] = strings.ReplaceAll(q, "\n", `'$'\n''`)
	}
	return strings.Join(cmd, " ")
}

func hasUpper(name string, fields []string) bool {
	return strings.ToLower(name) != name || strings.ToLower(strings.Join(fields, ",")) != strings.Join(fields, ",")
}

// longSyntax is the docker CLI's test (opts/network.go) for the advanced
// --network=key=value,... form; it applies to any value, names included.
var longSyntax = regexp.MustCompile(`\w+=\w+(,\w+=\w+)*`)

// netValue returns the --network value that attaches to network name (ID
// id, may be empty) with advanced fields f, or why none does. A name the
// CLI would read as the advanced form (a network named "name=host" would
// join the host network) goes in the advanced form, which lowercases and
// trims it; when that changes it too, the network is given by ID.
func (x *ctx) netValue(name, id string, f []string) (string, string) {
	if len(f) == 0 && !longSyntax.MatchString(name) {
		return name, ""
	}
	n := name
	if n != strings.ToLower(n) || n != strings.TrimSpace(n) || strings.Contains(n, "\r") {
		why := fmt.Sprintf("docker run's --network=name=... form lowercases and trims network name %q", name)
		if len(f) == 0 {
			why = fmt.Sprintf("docker run would read network name %q as --network=key=value fields, whose form lowercases and trims it", name)
		}
		if id == "" {
			return "", why
		}
		x.warn("network %q is given by ID %s: %s", name, shortID(id), why)
		n = id
	}
	if len(f) == 0 && !longSyntax.MatchString(n) {
		return n, ""
	}
	return csvLine(append([]string{"name=" + n}, f...)), ""
}

// endpointFields returns the advanced --network fields for an endpoint.
func endpointFields(e *network.EndpointSettings) []string {
	var f []string
	for _, al := range e.Aliases {
		f = append(f, "alias="+al)
	}
	if c := e.IPAMConfig; c != nil {
		if c.IPv4Address.IsValid() {
			f = append(f, "ip="+c.IPv4Address.String())
		}
		if c.IPv6Address.IsValid() {
			f = append(f, "ip6="+c.IPv6Address.String())
		}
		for _, ip := range c.LinkLocalIPs {
			f = append(f, "link-local-ip="+ip.String())
		}
	}
	if len(e.MacAddress) > 0 {
		f = append(f, "mac-address="+e.MacAddress.String())
	}
	for _, k := range sortedKeys(e.DriverOpts) {
		f = append(f, "driver-opt="+k+"="+e.DriverOpts[k])
	}
	if e.GwPriority != 0 {
		f = append(f, "gw-priority="+strconv.Itoa(e.GwPriority))
	}
	return f
}

// csvLine joins fields the way the docker CLI's csv reader splits them
// (a field holding a comma or quote gets quoted).
func csvLine(fields []string) string {
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	w.Write(fields)
	w.Flush()
	return strings.TrimSuffix(b.String(), "\n")
}

// sortedPorts orders ports by protocol, then number.
func sortedPorts[V any](m map[network.Port]V) []network.Port {
	ps := make([]network.Port, 0, len(m))
	for p := range m {
		ps = append(ps, p)
	}
	slices.SortFunc(ps, func(a, b network.Port) int {
		if c := strings.Compare(string(a.Proto()), string(b.Proto())); c != 0 {
			return c
		}
		return int(a.Num()) - int(b.Num())
	})
	return ps
}

// portSpan is a run of consecutive container ports with the same protocol
// and host IP whose host ports are all empty or consecutive numbers.
type portSpan struct {
	ip, proto  string
	lo, hi     int
	host       string // host port (or range "a-b") of lo; "" = picked by the daemon
	hostRange  bool   // host is a range string; never merged
	singleBind bool   // the port has one binding, so the span may grow
}

func (s portSpan) hostAt(n int) string {
	if s.host == "" || s.hostRange {
		return s.host
	}
	h, _ := strconv.Atoi(s.host)
	return strconv.Itoa(h + n - s.lo)
}

func (s portSpan) grows(n portSpan) bool {
	if !s.singleBind || !n.singleBind || s.hostRange || n.hostRange ||
		s.ip != n.ip || s.proto != n.proto || n.lo != s.hi+1 {
		return false
	}
	if s.host == "" || n.host == "" {
		return s.host == n.host
	}
	return s.hostAt(n.lo) == n.host
}

func rangeStr(lo, hi int) string {
	if lo == hi {
		return strconv.Itoa(lo)
	}
	return strconv.Itoa(lo) + "-" + strconv.Itoa(hi)
}

// publishArgs prints HostConfig.PortBindings, the requested bindings
// (NetworkSettings.Ports holds the ports the daemon picked at start, which
// must not be pinned). Consecutive ports collapse into ranges.
func publishArgs(pb network.PortMap) []string {
	var spans []portSpan
	for _, p := range sortedPorts(pb) {
		bs := pb[p]
		if len(bs) == 0 {
			bs = []network.PortBinding{{}}
		}
		for _, b := range bs {
			s := portSpan{proto: string(p.Proto()), lo: int(p.Num()), hi: int(p.Num()),
				host: b.HostPort, hostRange: strings.Contains(b.HostPort, "-"), singleBind: len(bs) == 1}
			if b.HostIP.IsValid() {
				s.ip = b.HostIP.String()
				if b.HostIP.Is6() {
					s.ip = "[" + s.ip + "]"
				}
			}
			if n := len(spans); n > 0 && spans[n-1].grows(s) {
				spans[n-1].hi = s.lo
				continue
			}
			spans = append(spans, s)
		}
	}
	var a []string
	for _, s := range spans {
		spec := rangeStr(s.lo, s.hi)
		if s.host != "" && !s.hostRange && s.hi > s.lo {
			spec = s.host + "-" + s.hostAt(s.hi) + ":" + spec
		} else if s.host != "" || s.ip != "" {
			spec = s.host + ":" + spec
		}
		if s.ip != "" {
			spec = s.ip + ":" + spec
		}
		if s.proto != "tcp" {
			spec += "/" + s.proto
		}
		a = append(a, "--publish="+spec)
	}
	return a
}

// exposeArgs prints Config.ExposedPorts the image does not expose and no
// --publish already implies, collapsing consecutive ports into ranges.
func (x *ctx) exposeArgs() []string {
	var a []string
	lo, hi, proto := -1, -1, ""
	flush := func() {
		if lo >= 0 {
			e := rangeStr(lo, hi)
			if proto != "tcp" {
				e += "/" + proto
			}
			a = append(a, "--expose="+e)
		}
	}
	for _, p := range sortedPorts(x.cfg.ExposedPorts) {
		if _, ok := x.imageCfg().ExposedPorts[p.String()]; ok {
			continue
		}
		if _, ok := x.hc.PortBindings[p]; ok && x.hc.NetworkMode != "host" { // --publish is left out there
			continue
		}
		n := int(p.Num())
		if string(p.Proto()) == proto && n == hi+1 {
			hi = n
			continue
		}
		flush()
		lo, hi, proto = n, n, string(p.Proto())
	}
	flush()
	return a
}
