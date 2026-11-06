package runlike

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// net_by_id.container.json: real inspect from Docker 29.9.3 (Lima, Linux
// arm64) of `docker create --network 6d4625e584fc --network-alias foo
// alpine:3.20`, then started (it exited). Once started, the daemon lists
// the network twice, under the ID given and under its name; printing both
// would attach the copy to the same network twice, and the name works on
// another daemon too.
func TestNetworkByID(t *testing.T) {
	got, err := Args(Inspect{Container: read(t, "testdata/net_by_id.container.json")}, Daemon{}, Options{NoName: true})
	if err != nil {
		t.Fatal(err)
	}
	opts := slices.DeleteFunc(got.Options, func(a string) bool { return !strings.HasPrefix(a, "--network") })
	want := []string{"--network=xp-net", "--network-alias=foo"}
	if !slices.Equal(opts, want) {
		t.Errorf("got  %q\nwant %q", opts, want)
	}
}

// TestNetworkNoDaemonData edits net_by_id in memory for the cases only
// --stdin data or library use reach: no daemon host name, no NetworkMode.
func TestNetworkNoDaemonData(t *testing.T) {
	edit := func(f func(c map[string]any)) []byte {
		var c map[string]any
		if err := json.Unmarshal(read(t, "testdata/net_by_id.container.json"), &[]any{&c}); err != nil {
			t.Fatal(err)
		}
		f(c)
		b, _ := json.Marshal(c)
		return b
	}
	hc := func(c map[string]any) map[string]any { return c["HostConfig"].(map[string]any) }
	nets := func(c map[string]any) map[string]any {
		return c["NetworkSettings"].(map[string]any)["Networks"].(map[string]any)
	}
	for _, tc := range []struct {
		name       string
		edit       func(c map[string]any)
		d          Daemon
		want, warn string
	}{
		{"host, no daemon host name", func(c map[string]any) {
			hc(c)["NetworkMode"] = "host"
			c["NetworkSettings"].(map[string]any)["Networks"] = map[string]any{"host": map[string]any{}}
			c["Config"].(map[string]any)["Hostname"] = "myhost"
		}, Daemon{}, "", "no daemon host name"},
		{"host, same as daemon", func(c map[string]any) {
			hc(c)["NetworkMode"] = "host"
			c["NetworkSettings"].(map[string]any)["Networks"] = map[string]any{"host": map[string]any{}}
			c["Config"].(map[string]any)["Hostname"] = "lima"
		}, Daemon{HostName: "lima"}, "", ""},
		{"no NetworkMode", func(c map[string]any) {
			hc(c)["NetworkMode"] = ""
			delete(nets(c), "6d4625e584fc")
		}, Daemon{}, "--network=xp-net", ""},
		// Stopped, --stdin: API 1.51 data says no MAC was pinned (or which);
		// API 1.52 data cannot tell, so the first network is warned about.
		{"stopped, API 1.51 data, no MAC", func(c map[string]any) {
			c["Config"].(map[string]any)["MacAddress"] = ""
		}, Daemon{}, "--network=xp-net", ""},
		{"stopped, API 1.51 data, pinned MAC", func(c map[string]any) {
			c["Config"].(map[string]any)["MacAddress"] = "02:42:ac:11:00:55"
		}, Daemon{}, "--mac-address=02:42:ac:11:00:55", ""},
		{"stopped, CLI data, no MAC", func(c map[string]any) {}, Daemon{Platform: "linux/arm64"}, "--network=xp-net", ""},
		{"stopped, API 1.52 data", func(c map[string]any) {}, Daemon{}, "--network=xp-net", ""},
		{"upper case driver-opt on the first network", func(c map[string]any) {
			for _, e := range nets(c) {
				e.(map[string]any)["DriverOpts"] = map[string]string{"k": "V"}
			}
		}, Daemon{}, "--network=name=xp-net,driver-opt=k=V", "lowercases"},
		// The plain --network=name=host would join the host network.
		{"network named name=host", func(c map[string]any) {
			hc(c)["NetworkMode"] = "name=host"
			c["NetworkSettings"].(map[string]any)["Networks"] = map[string]any{"name=host": map[string]any{}}
		}, Daemon{}, "--network=name=name=host", ""},
		{"upper case network name with =, by ID", func(c map[string]any) {
			hc(c)["NetworkMode"] = "Name=Host"
			c["NetworkSettings"].(map[string]any)["Networks"] = map[string]any{"Name=Host": map[string]any{"NetworkID": "c5233ca18735965b9a9608fc9d59dc18503c5a6d03a869d57def710ed71836b8"}}
		}, Daemon{}, "--network=c5233ca18735965b9a9608fc9d59dc18503c5a6d03a869d57def710ed71836b8", "given by ID c5233ca18735"},
		{"upper case network name with =, no ID", func(c map[string]any) {
			hc(c)["NetworkMode"] = "Name=Host"
			c["NetworkSettings"].(map[string]any)["Networks"] = map[string]any{"Name=Host": map[string]any{}}
		}, Daemon{}, "", "docker network disconnect bridge"},
	} {
		rc, err := Args(Inspect{Container: edit(tc.edit)}, tc.d, Options{})
		if err != nil {
			t.Fatal(err)
		}
		hn := slices.ContainsFunc(rc.Options, func(o string) bool { return strings.HasPrefix(o, "--hostname") })
		if hn {
			t.Errorf("%s: unexpected --hostname in %q", tc.name, rc.Options)
		}
		if slices.ContainsFunc(rc.Options, func(o string) bool { return strings.EqualFold(o, "--network=name=host") || o == "--network=Name=Host" }) {
			t.Errorf("%s: the CLI reads %q as the host network", tc.name, rc.Options)
		}
		if tc.want != "" && !slices.Contains(rc.Options, tc.want) {
			t.Errorf("%s: no %q in %q", tc.name, tc.want, rc.Options)
		}
		// No image data here: drop the warning that always says so.
		rc.Warnings = slices.DeleteFunc(rc.Warnings, func(w string) bool { return strings.HasPrefix(w, "no image data") })
		warned := slices.ContainsFunc(rc.Warnings, func(w string) bool { return tc.warn != "" && strings.Contains(w, tc.warn) })
		if warned != (tc.warn != "") || tc.warn == "" && len(rc.Warnings) > 0 {
			t.Errorf("%s: warnings %q, want one containing %q", tc.name, rc.Warnings, tc.warn)
		}
	}
}

// TestMAC: only a pinned MAC is printed. Edits net_by_id (first network
// xp-net) into running, stopped and created containers with a further
// network n2, with CLI data (Platform set: Config.MacAddress was read)
// and --stdin data.
func TestMAC(t *testing.T) {
	const pin, rnd, pin2 = "02:42:ac:11:00:55", "a6:91:00:00:00:01", "02:00:00:00:00:02"
	for _, tc := range []struct {
		name, status, cfgMAC, epMAC, n2MAC string
		cli                                bool
		want                               []string // --network and --mac-address values
		warn                               string   // networks named in a MAC warning (none since 2026-11-06: a known limit)
	}{
		{"running, CLI, not pinned", "running", "", rnd, "", true, []string{"--network=xp-net"}, ""},
		{"running, CLI, pinned", "running", pin, pin, "", true, []string{"--network=xp-net", "--mac-address=" + pin}, ""},
		{"running, API 1.51 stdin, pinned", "running", "stdin:" + pin, pin, "", false, []string{"--network=xp-net", "--mac-address=" + pin}, ""},
		{"running, API 1.52 stdin", "running", "", rnd, "", false, []string{"--network=xp-net"}, ""},
		{"running, CLI, further network", "running", "", rnd, pin2, true, []string{"--network=xp-net", "--network=n2"}, ""},
		{"stopped, CLI, further network connected while stopped", "exited", pin, "", pin2, true, []string{"--network=xp-net", "--mac-address=" + pin, "--network=name=n2,mac-address=" + pin2}, ""},
		{"stopped, CLI, further network", "exited", "", "", "", true, []string{"--network=xp-net", "--network=n2"}, ""},
		{"created, stdin, pinned on both", "created", "", pin, pin2, false, []string{"--network=xp-net", "--mac-address=" + pin, "--network=name=n2,mac-address=" + pin2}, ""},
	} {
		var c map[string]any
		if err := json.Unmarshal(read(t, "testdata/net_by_id.container.json"), &[]any{&c}); err != nil {
			t.Fatal(err)
		}
		c["State"] = map[string]any{"Status": tc.status, "Running": tc.status == "running"}
		c["HostConfig"].(map[string]any)["NetworkMode"] = "xp-net"
		in := Inspect{}
		if m, ok := strings.CutPrefix(tc.cfgMAC, "stdin:"); ok {
			c["Config"].(map[string]any)["MacAddress"] = m
		} else {
			in.ConfigMACAddress = tc.cfgMAC
		}
		nets := map[string]any{"xp-net": map[string]any{"MacAddress": tc.epMAC}}
		if tc.n2MAC != "" || strings.Contains(tc.name, "further") {
			nets["n2"] = map[string]any{"MacAddress": tc.n2MAC}
		}
		c["NetworkSettings"].(map[string]any)["Networks"] = nets
		in.Container, _ = json.Marshal(c)
		d := Daemon{}
		if tc.cli {
			d.Platform = "linux/arm64"
		}
		rc, err := Args(in, d, Options{})
		if err != nil {
			t.Fatal(err)
		}
		got := slices.DeleteFunc(slices.Clone(rc.Options), func(o string) bool {
			return !strings.HasPrefix(o, "--mac-address") && !strings.HasPrefix(o, "--network=")
		})
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
		var w string
		for _, s := range rc.Warnings {
			if n, ok := strings.CutPrefix(s, "Docker does not report whether a MAC address was pinned on network(s) "); ok {
				w, _, _ = strings.Cut(n, ",")
			} else if !strings.HasPrefix(s, "no image data") {
				t.Errorf("%s: unexpected warning %q", tc.name, s)
			}
		}
		if w != tc.warn {
			t.Errorf("%s: MAC warning names %q, want %q", tc.name, w, tc.warn)
		}
	}
}

// net_by_fullid.container.json: real inspect from Docker 29.9.3 (Lima,
// Linux arm64) of `docker create --network <full ID of fb2-n>
// --mac-address 02:42:ac:11:00:97 alpine sleep 1000`, then `docker network
// connect fb2-m`; never started. The daemon lists fb2-n under its name with
// no NetworkID, and API 1.51 has no Config.MacAddress (moby looks it up
// under the ID). The endpoint MAC is the pinned one.
func TestNetworkByFullIDCreated(t *testing.T) {
	const id = "616dce6ad8b9842117e442a172224c62f7eb101e453f5942fddabaa6e152d614"
	in := Inspect{Container: read(t, "testdata/net_by_fullid.container.json")}
	cli := Daemon{Platform: "linux/arm64", NetworkNames: map[string]string{id: "fb2-n"}}
	got, err := Args(in, cli, Options{NoName: true})
	if err != nil {
		t.Fatal(err)
	}
	opts := slices.DeleteFunc(got.Options, func(a string) bool { return !strings.HasPrefix(a, "--network") && !strings.HasPrefix(a, "--mac") })
	want := []string{"--network=fb2-n", "--mac-address=02:42:ac:11:00:97", "--network=fb2-m"}
	if !slices.Equal(opts, want) {
		t.Errorf("got  %q\nwant %q", opts, want)
	}
	// --stdin, one network: it is the first one.
	var c map[string]any
	if err := json.Unmarshal(in.Container, &[]any{&c}); err != nil {
		t.Fatal(err)
	}
	delete(c["NetworkSettings"].(map[string]any)["Networks"].(map[string]any), "fb2-m")
	in.Container, _ = json.Marshal(c)
	if got, err = Args(in, Daemon{}, Options{NoName: true}); err != nil {
		t.Fatal(err)
	}
	opts = slices.DeleteFunc(got.Options, func(a string) bool { return !strings.HasPrefix(a, "--network") && !strings.HasPrefix(a, "--mac") })
	if want := []string{"--network=fb2-n", "--mac-address=02:42:ac:11:00:97"}; !slices.Equal(opts, want) {
		t.Errorf("--stdin: got  %q\nwant %q", opts, want)
	}
}

// TestNetworkLeftOut: a first network that no --network value can name
// (upper case and "=", no NetworkID to give it by: a created container)
// is left out with connect commands, from net_by_fullid edited in memory.
// The copy then joins the default network, or the first further network
// when there are some: only the former needs a disconnect. The pinned MAC
// is named, since docker network connect cannot set it.
func TestNetworkLeftOut(t *testing.T) {
	for _, tc := range []struct {
		name       string
		extra      bool
		want       []string
		disconnect bool
	}{
		{"with a further network", true, []string{"--network=fb2-m"}, false},
		{"alone", false, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var c map[string]any
			if err := json.Unmarshal(read(t, "testdata/net_by_fullid.container.json"), &[]any{&c}); err != nil {
				t.Fatal(err)
			}
			nets := c["NetworkSettings"].(map[string]any)["Networks"].(map[string]any)
			nets["Up=x"] = nets["fb2-n"]
			delete(nets, "fb2-n")
			if !tc.extra {
				delete(nets, "fb2-m")
			}
			c["HostConfig"].(map[string]any)["NetworkMode"] = "Up=x"
			b, _ := json.Marshal(c)
			got, err := Args(Inspect{Container: b}, Daemon{Platform: "linux/arm64"}, Options{})
			if err != nil {
				t.Fatal(err)
			}
			opts := slices.DeleteFunc(got.Options, func(a string) bool { return !strings.HasPrefix(a, "--network") && !strings.HasPrefix(a, "--mac") })
			if !slices.Equal(opts, tc.want) {
				t.Errorf("got  %q\nwant %q", opts, tc.want)
			}
			w := strings.Join(got.Warnings, "\n")
			if !strings.Contains(w, "  docker network connect Up=x fb2-c3") || !strings.Contains(w, "cannot set its MAC address 02:42:ac:11:00:97") ||
				strings.Contains(w, "docker network disconnect bridge fb2-c3") != tc.disconnect {
				t.Errorf("warnings %q", got.Warnings)
			}
		})
	}
}
