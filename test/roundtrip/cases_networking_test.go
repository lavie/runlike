package roundtrip

import (
	"fmt"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/lavie/runlike/v2"
	dockercli "github.com/lavie/runlike/v2/internal/daemon"
)

// Networking area: host name, DNS, extra hosts, networks, endpoint
// settings, links, published and exposed ports.
//
// Cases run in parallel and the daemon's default address pools hold only
// about 30 networks, so every case takes its own fixed subnets: nets(n)
// gives n1 10.(100+n).0.0/16 + fd00:n::/64 (IPv6 on), n2 10.(150+n).0.0/16
// and n3 10.(200+n).0.0/16. Use a distinct n per case. Static IPs only
// work in create cases: a running original holds its address, so the
// copies could not start.
var rmNets = [][]string{{"network", "rm", "{p}-n1", "{p}-n2", "{p}-n3"}}

func nets(n int) [][]string {
	return [][]string{
		{"network", "create", "--ipv6", fmt.Sprintf("--subnet=10.%d.0.0/16", 100+n), fmt.Sprintf("--subnet=fd00:%d::/64", n), "{p}-n1"},
		{"network", "create", fmt.Sprintf("--subnet=10.%d.0.0/16", 150+n), "{p}-n2"},
		{"network", "create", fmt.Sprintf("--subnet=10.%d.0.0/16", 200+n), "{p}-n3"},
	}
}

var _ = add("networking",
	// defaults: nothing printed (bridge, auto host name, no DNS, no ports)
	Case{Name: "default", Args: []string{alpine, "true"}},
	Case{Name: "default-run", Run: true, Args: []string{alpine, "sleep", "300"}},
	// hostname and domain name
	Case{Name: "hostname", Args: []string{"--hostname=web-1", alpine}},
	Case{Name: "hostname-fqdn", Args: []string{"-h", "web.example.com", alpine}},
	Case{Name: "hostname-run", Run: true, Args: []string{"--hostname=web-1", alpine, "sleep", "300"}},
	Case{Name: "domainname", Args: []string{"--domainname=example.org", "--hostname=h", alpine}},
	Case{Name: "domainname-only", Args: []string{"--domainname=example.org", alpine}},
	// DNS
	Case{Name: "dns", Args: []string{"--dns=1.1.1.1", "--dns=2606:4700::1111", "--dns=8.8.8.8", alpine}},
	Case{Name: "dns-option", Args: []string{"--dns-option=ndots:2", "--dns-option=timeout:3", "--dns-option=rotate", alpine}},
	Case{Name: "dns-search", Args: []string{"--dns-search=a.example", "--dns-search=.", "--dns-search=b.example", alpine}},
	Case{Name: "dns-all-run", Run: true, Args: []string{"--dns=9.9.9.9", "--dns-option=ndots:1", "--dns-search=x.example", alpine, "sleep", "300"}},
	// extra hosts
	Case{Name: "add-host", Args: []string{"--add-host=h1:10.0.0.1", "--add-host=h1:10.0.0.2", "--add-host=h2=::1", "--add-host=h3:[fd00::2]", "--add-host=gw:host-gateway", alpine}},
	// network modes
	Case{Name: "net-host", Args: []string{"--network=host", alpine}},
	Case{Name: "net-host-run", Run: true, Args: []string{"--network=host", alpine, "sleep", "300"}},
	Case{Name: "net-none", Args: []string{"--network=none", alpine}},
	Case{Name: "net-bridge-explicit", Args: []string{"--network=bridge", alpine}},
	Case{Name: "net-container",
		Setup:    [][]string{{"run", "-d", "--name={p}-peer", "--label=runlike.roundtrip=1", alpine, "sleep", "300"}},
		Teardown: [][]string{{"rm", "-f", "{p}-peer"}},
		Args:     []string{"--network=container:{p}-peer", alpine}},
	Case{Name: "net-user", Setup: nets(3), Teardown: rmNets, Args: []string{"--network={p}-n1", alpine}},
	Case{Name: "net-user-run", Run: true, Setup: nets(4), Teardown: rmNets, Args: []string{"--network={p}-n1", alpine, "sleep", "300"}},
	// endpoint settings on the first network
	Case{Name: "net-alias", Setup: nets(5), Teardown: rmNets,
		Args: []string{"--network={p}-n1", "--network-alias=web", "--network-alias=Web-Upper", alpine}},
	Case{Name: "net-ip", Setup: nets(1), Teardown: rmNets,
		Args: []string{"--network={p}-n1", "--ip=10.101.5.5", "--ip6=fd00:1::55", alpine}},
	Case{Name: "net-link-local", Setup: nets(6), Teardown: rmNets,
		Args: []string{"--network={p}-n1", "--link-local-ip=169.254.10.1", "--link-local-ip=fe80::10", alpine}},
	Case{Name: "mac-bridge", Args: []string{"--mac-address=02:42:ac:11:00:99", alpine}},
	Case{Name: "mac-user", Setup: nets(7), Teardown: rmNets,
		Args: []string{"--network={p}-n1", "--mac-address=92:d0:c6:0a:29:33", alpine}},
	Case{Name: "net-driver-opt", Setup: nets(8), Teardown: rmNets,
		Args: []string{"--network=name={p}-n1,driver-opt=com.docker.network.endpoint.sysctls=net.ipv4.conf.IFNAME.log_martians=1,gw-priority=7", "--network-alias=a", alpine}},
	// several networks
	Case{Name: "net-multi", Setup: nets(9), Teardown: rmNets,
		Args: []string{"--network={p}-n1", "--network={p}-n2", "--network={p}-n3", alpine}},
	Case{Name: "net-multi-settings", Setup: nets(2), Teardown: rmNets,
		Args: []string{"--network=name={p}-n2,alias=two,alias=deux,ip=10.152.0.9,mac-address=02:00:00:00:00:02,gw-priority=3",
			`--network=name={p}-n1,ip6=fd00:2::9,link-local-ip=169.254.3.3,"driver-opt=com.example.list=a,b ""c""",driver-opt=com.example.z=1`,
			"--network={p}-n3", alpine}},
	Case{Name: "net-multi-run", Run: true, Setup: nets(10), Teardown: rmNets,
		Args: []string{"--network={p}-n3", "--network-alias=main", "--network=name={p}-n1,alias=one", alpine, "sleep", "300"}},
	// links
	Case{Name: "link-legacy",
		Setup:    [][]string{{"create", "--name={p}-db", "--label=runlike.roundtrip=1", alpine}, {"create", "--name={p}-cache", "--label=runlike.roundtrip=1", alpine}},
		Teardown: [][]string{{"rm", "-f", "{p}-db", "{p}-cache"}},
		Args:     []string{"--link={p}-db:db", "--link={p}-cache", alpine}},
	Case{Name: "link-user", Setup: append([][]string{{"create", "--name={p}-db", "--label=runlike.roundtrip=1", alpine}}, nets(11)...),
		Teardown: append([][]string{{"rm", "-f", "{p}-db"}}, rmNets...),
		Args:     []string{"--network={p}-n1", "--link={p}-db:db", alpine}},
	// ports
	Case{Name: "publish", Args: []string{"-p", "8080:80", "-p", "127.0.0.1:8443:443", "-p", "[::1]:9090:90/udp", "-p", "53:53/sctp", alpine}},
	Case{Name: "publish-same-port-twice", Args: []string{"-p", "127.0.0.1:18080:80", "-p", "18081:80", "-p", "[::1]:18082:80", alpine}},
	Case{Name: "publish-auto", Args: []string{"-p", "80", "-p", "127.0.0.1::443", "-p", "[::1]::444/udp", alpine}},
	Case{Name: "publish-auto-run", Run: true, Args: []string{"-p", "80", "-p", "127.0.0.1::443", alpine, "sleep", "300"}},
	Case{Name: "publish-ranges", Args: []string{"-p", "15000-15002:16000-16002", "-p", "17000-17010:7000", "-p", "6100-6103", "-p", "127.0.0.1:16200-16201:6200-6201/udp", alpine}},
	Case{Name: "publish-all", Run: true, Args: []string{"-P", "--expose=81", alpine, "sleep", "300"}},
	Case{Name: "publish-all-image", Args: []string{"-P", nginx}},
	Case{Name: "expose", Args: []string{"--expose=3000", "--expose=4000-4003", "--expose=22/sctp", "--expose=53/udp", alpine}},
	Case{Name: "expose-image", Args: []string{"--expose=80", "--expose=81", "-p", "8088:80", nginx}},
	// everything at once
	Case{Name: "combo-run", Run: true, Setup: nets(12), Teardown: rmNets,
		Args: []string{"--hostname=app", "--domainname=corp.example", "--dns=10.0.0.53", "--dns-search=corp.example",
			"--add-host=db:10.0.0.7", "--network={p}-n1", "--network-alias=app.corp",
			"--network=name={p}-n2,alias=app2", "-p", "127.0.0.1::8500", "--expose=9000", alpine, "sleep", "300"}},
)

// Step cases change A after creating it (start and stop, network
// connect), which Case cannot express, and check runlike's warnings. Each
// passes the daemon data the CLI would put in runlike.Options itself.
// B is made with docker create from runlike's argv, gets the same `both`
// steps, and, with connect set, the `docker network connect` commands
// from the warnings; then A and B are compared as in TestRoundTrip.
type stepCase struct {
	name     string
	setup    [][]string // before A; {p} as in Case
	args     []string   // after "docker create" (or "docker run -d" with run)
	api      string     // create A through the Engine API: a JSON merge patch over args' request ({p} as in Case)
	stdin    bool       // run runlike as --stdin does: no daemon or image data
	run      bool
	after    [][]string // on A only; {c} = the container's name
	both     [][]string // on A, then on B
	want     []string   // each a prefix of one of runlike's options
	notWant  []string   // no option starts with one of these
	warn     []string   // each a substring of one warning
	noWarn   []string   // no warning contains one of these
	connect  bool
	noCopy   bool // check argv and warnings only
	ignore   []Ignore
	teardown [][]string
}

var stepCases []stepCase

func addSteps(cs ...stepCase) bool { stepCases = append(stepCases, cs...); return true }

var _ = addSteps(
	// MAC: only a pinned one is printed; API 1.51's Config.MacAddress tells
	// for the first network (on Docker Desktop through docker.raw.sock),
	// nothing for further networks.
	stepCase{name: "mac-stopped-bridge", args: []string{"--mac-address=02:42:ac:11:00:98", alpine, "sleep", "300"},
		both: [][]string{{"start", "{c}"}, {"stop", "-t0", "{c}"}},
		want: []string{"--mac-address=02:42:ac:11:00:98"}, noWarn: []string{"MAC"}},
	stepCase{name: "mac-stopped-plain", args: []string{alpine, "sleep", "300"},
		both:    [][]string{{"start", "{c}"}, {"stop", "-t0", "{c}"}},
		notWant: []string{"--mac-address"}, noWarn: []string{"MAC"}},
	stepCase{name: "mac-stopped-user", setup: nets(13), teardown: rmNets,
		args: []string{"--network={p}-n1", "--mac-address=02:00:00:00:00:33", "--network={p}-n2", alpine, "sleep", "300"},
		both: [][]string{{"start", "{c}"}, {"stop", "-t0", "{c}"}},
		want: []string{"--mac-address=02:00:00:00:00:33"}, noWarn: []string{"MAC"}},
	// --stdin with API 1.52 data: a started container reports no pinned
	// MAC, and the data cannot tell whether one was pinned.
	stepCase{name: "mac-stopped-stdin", args: []string{"--mac-address=02:42:ac:11:00:97", alpine, "sleep", "300"},
		both: [][]string{{"start", "{c}"}, {"stop", "-t0", "{c}"}}, stdin: true, noCopy: true,
		noWarn: []string{"MAC"}},
	// running; no copy: a created copy cannot match a running original
	stepCase{name: "mac-running", run: true, args: []string{alpine, "sleep", "300"}, noCopy: true,
		notWant: []string{"--mac-address"}, noWarn: []string{"MAC"}},
	stepCase{name: "mac-running-pinned", run: true, args: []string{"--mac-address=02:42:ac:11:00:96", alpine, "sleep", "300"}, noCopy: true,
		want: []string{"--mac-address=02:42:ac:11:00:96"}, noWarn: []string{"MAC"}},
	stepCase{name: "mac-running-user", run: true, setup: nets(15), teardown: rmNets, noCopy: true,
		args:    []string{"--network={p}-n1", "--mac-address=02:00:00:00:00:44", "--network=name={p}-n2,mac-address=02:00:00:00:00:45", alpine, "sleep", "300"},
		want:    []string{"--mac-address=02:00:00:00:00:44", "--network={p}-n2"},
		notWant: []string{"--network={p}-n2,mac-address"}, noWarn: []string{"MAC"}},
	stepCase{name: "mac-running-stdin", run: true, args: []string{"--mac-address=02:42:ac:11:00:95", alpine, "sleep", "300"}, stdin: true, noCopy: true,
		notWant: []string{"--mac-address"}, noWarn: []string{"MAC"}},
	// --network <full ID>: a created container lists the network by name
	// without NetworkID, and API 1.51 has no Config.MacAddress, so the
	// endpoint's (pinned) MAC is used. Once started no API reports the
	// pinned MAC (README "Known limits"): none is printed, no warning.
	stepCase{name: "mac-created-netid", setup: nets(16), teardown: rmNets,
		args:    []string{"--network={n1id}", "--mac-address=02:00:00:00:00:46", "--network={p}-n2", alpine, "sleep", "300"},
		want:    []string{"--network={p}-n1", "--mac-address=02:00:00:00:00:46", "--network={p}-n2"},
		notWant: []string{"--network={n1id}"}, noWarn: []string{"MAC"},
		ignore: []Ignore{{"HostConfig.NetworkMode", "the copy names the network the original gave by ID"}}},
	stepCase{name: "mac-running-netid", run: true, setup: nets(17), teardown: rmNets, noCopy: true,
		args:    []string{"--network={n1id}", "--mac-address=02:00:00:00:00:47", alpine, "sleep", "300"},
		want:    []string{"--network={p}-n1"},
		notWant: []string{"--mac-address", "--network={n1id}"}, noWarn: []string{"MAC"}},
	stepCase{name: "bridge-connect", setup: append([][]string{{"create", "--name={p}-db", "--label=" + sweepLabel, alpine}}, nets(14)...),
		teardown: append([][]string{{"rm", "-f", "{p}-db"}}, rmNets...),
		args:     []string{alpine, "true"},
		after:    [][]string{{"network", "connect", "--alias=Foo", "--alias=bar", "--link={p}-db:dbx", "{p}-n1", "{c}"}, {"network", "connect", "{p}-n2", "{c}"}},
		notWant:  []string{"--network=bridge"}, warn: []string{"docker run cannot join the bridge network and others at once"}, connect: true},
	stepCase{name: "upper-connect",
		setup:    [][]string{{"network", "create", "--subnet=10.99.0.0/16", "{p}-n2"}, {"network", "create", "--subnet=10.98.0.0/16", "{p}-Up"}},
		teardown: [][]string{{"network", "rm", "{p}-n2", "{p}-Up"}},
		args:     []string{"--network={p}-n2", alpine, "true"},
		after:    [][]string{{"network", "connect", "--alias=Foo", "--driver-opt=com.example.K=VaL", "{p}-Up", "{c}"}},
		want:     []string{"--network={p}-n2"}, warn: []string{"docker run lowercases the whole --network=name=... value"}, connect: true},
	stepCase{name: "host-hostname", args: []string{"--network=host", "--hostname=myhost", "-p", "8080:80", alpine, "true"},
		want: []string{"--hostname=myhost"}, notWant: []string{"--publish=8080:80"}, warn: []string{"published ports left out"},
		ignore: []Ignore{{"HostConfig.PortBindings", "Docker discards published ports in host mode; runlike leaves them out"}}},
	// Network names the docker CLI reads as --network=key=value fields (a
	// network named "name=host" would join the host network): lower case
	// goes in the advanced form, upper case only by connect commands,
	// which leave the copy's NetworkMode bridge. The CLI lowercases such a
	// name, so the API sets the upper case one.
	stepCase{name: "net-eq-name",
		setup:    [][]string{{"network", "create", "--subnet=10.97.0.0/16", "{p}=host"}, {"network", "create", "--subnet=10.96.0.0/16", "{p}-x=y"}},
		teardown: [][]string{{"network", "rm", "{p}=host", "{p}-x=y"}},
		args:     []string{"--network=name={p}=host", "--network=name={p}-x=y", alpine, "true"},
		want:     []string{"--network=name={p}=host", "--network=name={p}-x=y"}},
	stepCase{name: "net-eq-upper",
		setup:    [][]string{{"network", "create", "--subnet=10.95.0.0/16", "{p}-Up=x"}},
		teardown: [][]string{{"network", "rm", "{p}-Up=x"}},
		args:     []string{alpine, "true"}, api: `{"HostConfig":{"NetworkMode":"{p}-Up=x"}}`,
		notWant: []string{"--network={p}-Up=x"}, warn: []string{"left out (docker run would read network name"}, connect: true,
		ignore: []Ignore{{"HostConfig.NetworkMode", "the copy joins the default network first; the warned commands move it"},
			{"NetworkSettings.Networks.*.Aliases", "network connect stores null, create []"},
			{"NetworkSettings.Networks.*.DNSNames", "network connect on a created container stores none"},
			{"NetworkSettings.Networks.*.DriverOpts", "network connect stores null, create {}"},
			{"NetworkSettings.Networks.*.IPAMConfig", "network connect stores null, create {}"}}},
	stepCase{name: "container-by-name",
		setup:    [][]string{{"run", "-d", "--name={p}-peer", "--label=" + sweepLabel, alpine, "sleep", "300"}},
		teardown: [][]string{{"rm", "-f", "{p}-peer"}},
		args:     []string{"--network=container:{p}-peer", alpine},
		want:     []string{"--network=container:{p}-peer"}},
)

func TestSteps(t *testing.T) {
	needLinux(t)
	for _, c := range stepCases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			stepTrip(t, c)
		})
	}
}

func stepTrip(t *testing.T, c stepCase) {
	p := "rt-step-" + c.name + "-" + randHex()
	a, b := p+"-a", p+"-b"
	sub := func(ss []string, name string) []string {
		out := make([]string, len(ss))
		for i, s := range ss {
			out[i] = strings.ReplaceAll(strings.ReplaceAll(s, "{p}", p), "{c}", name)
			if strings.Contains(out[i], "{n1id}") {
				id, err := docker("network", "inspect", "--format", "{{.Id}}", p+"-n1")
				if err != nil {
					t.Fatal(err)
				}
				out[i] = strings.ReplaceAll(out[i], "{n1id}", strings.TrimSpace(id))
			}
		}
		return out
	}
	steps := func(cmds [][]string, name string) {
		for _, cmd := range cmds {
			if _, err := docker(sub(cmd, name)...); err != nil {
				t.Fatal(err)
			}
		}
	}
	t.Cleanup(func() { steps(c.teardown, "") })
	t.Cleanup(func() { docker("rm", "-f", "-v", a, b) })
	steps(c.setup, "")
	verb := []string{"create"}
	if c.run {
		verb = []string{"run", "-d"}
	}
	if c.api != "" {
		apiCreate(t, a, sub(c.args, a), sub([]string{c.api}, a)[0])
	} else if _, err := docker(slices.Concat(verb, []string{"--name=" + a, "--label=" + sweepLabel}, sub(c.args, a))...); err != nil {
		t.Fatalf("container A: %v", err)
	}
	steps(c.after, a)
	steps(c.both, a)
	// Daemon fields (host name, container names, API 1.51 MAC) as the CLI fills them.
	in, dm, warns, err := dockercli.CLI{Bin: dockerBin}.Inspect(a)
	if err != nil {
		t.Fatal(err)
	}
	jsA := string(in.Container)
	if c.stdin {
		in, dm, warns = runlike.Inspect{Container: in.Container}, runlike.Daemon{}, nil
	}
	rc, err := runlike.Args(in, dm, runlike.Options{})
	if err != nil {
		t.Fatal(err)
	}
	rc.Warnings = append(warns, rc.Warnings...)
	t.Logf("runlike: %q\nwarnings: %q", rc.Options, rc.Warnings)
	for _, w := range sub(c.want, a) {
		if !slices.ContainsFunc(rc.Options, func(o string) bool { return strings.HasPrefix(o, w) }) {
			t.Errorf("no option starts with %q", w)
		}
	}
	for _, w := range sub(c.notWant, a) {
		if slices.ContainsFunc(rc.Options, func(o string) bool { return strings.HasPrefix(o, w) }) {
			t.Errorf("unexpected %q", w)
		}
	}
	for _, w := range c.noWarn {
		if slices.ContainsFunc(rc.Warnings, func(s string) bool { return strings.Contains(s, w) }) {
			t.Errorf("unexpected warning containing %q", w)
		}
	}
	for _, w := range c.warn {
		if !slices.ContainsFunc(rc.Warnings, func(s string) bool { return strings.Contains(s, w) }) {
			t.Errorf("no warning contains %q", w)
		}
	}

	if c.noCopy {
		return
	}
	bc := forB(rc, b, false)
	argv := bc.Argv()
	argv[1] = "create"
	if _, err := docker(argv[1:]...); err != nil {
		t.Fatalf("recreate: %v\nargv %q", err, argv)
	}
	if c.connect {
		for _, w := range rc.Warnings {
			for _, l := range strings.Split(w, "\n") {
				l = strings.TrimSpace(l)
				if !strings.HasPrefix(l, "docker network ") || !strings.HasSuffix(l, " "+a) {
					continue
				}
				l = strings.TrimSuffix(l, a) + b
				if runtime.GOOS == "windows" { // no sh: run the posix line unquoted
					if strings.ContainsAny(l, `'"\$`) {
						t.Skipf("needs sh: %s", l)
					}
					if _, err := docker(strings.Fields(l)[1:]...); err != nil {
						t.Fatalf("%s: %v", l, err)
					}
					continue
				}
				sh := exec.Command("/bin/sh", "-c", l)
				sh.Env = envPath()
				if out, err := sh.CombinedOutput(); err != nil {
					t.Fatalf("%s: %v: %s", l, err, out)
				}
			}
		}
	}
	steps(c.both, b)
	jsB, err := docker("container", "inspect", b)
	if err != nil {
		t.Fatal(err)
	}
	nc := Case{Ignore: c.ignore, Run: c.run}
	if d := diff(normalize(t, jsA, nc), normalize(t, jsB, nc)); d != "" {
		t.Errorf("copy differs from original (- original, + copy):\n%s\nargv: %q", d, argv)
	}
}
