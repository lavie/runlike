package roundtrip

// Runtime area: namespaces, restart policy, --rm, runtime, platform.
var _ = add("runtime",
	Case{Name: "default", Args: []string{alpine, "true"}},
	Case{Name: "cgroupns-host", Args: []string{"--cgroupns=host", alpine, "true"}},
	Case{Name: "cgroupns-private", Args: []string{"--cgroupns=private", alpine, "true"}},
	Case{Name: "ipc-private", Args: []string{"--ipc=private", alpine, "true"}},
	Case{Name: "ipc-shareable", Args: []string{"--ipc=shareable", alpine, "true"}},
	Case{Name: "ipc-none", Args: []string{"--ipc=none", alpine, "true"}},
	Case{Name: "ipc-host", Args: []string{"--ipc=host", alpine, "true"}},
	Case{Name: "pid-host", Args: []string{"--pid=host", alpine, "true"}},
	Case{Name: "uts-host", Args: []string{"--uts=host", alpine, "true"}},
	Case{Name: "ns-container",
		Setup:    [][]string{{"run", "-d", "--name", "{p}-peer", "--label", "runlike.roundtrip=1", "--ipc=shareable", alpine, "sleep", "300"}},
		Args:     []string{"--ipc=container:{p}-peer", "--pid=container:{p}-peer", alpine, "true"},
		Teardown: [][]string{{"rm", "-f", "{p}-peer"}}},
	Case{Name: "restart-always", Args: []string{"--restart=always", alpine, "true"}},
	Case{Name: "restart-unless-stopped", Args: []string{"--restart=unless-stopped", alpine, "true"}},
	Case{Name: "restart-on-failure", Args: []string{"--restart=on-failure", alpine, "true"}},
	Case{Name: "restart-on-failure-n", Args: []string{"--restart=on-failure:5", alpine, "true"}},
	Case{Name: "restart-no", Args: []string{"--restart=no", alpine, "true"}},
	Case{Name: "rm", Args: []string{"--rm", alpine, "true"}},
	Case{Name: "runtime-runc", Args: []string{"--runtime=runc", alpine, "true"}},
	Case{Name: "runtime-other", Args: []string{"--runtime=io.containerd.runc.v2", alpine, "true"}},
	platformCase("amd64", "sha256:c64c687cbea9300178b30c95835354e34c4e4febc4badfe27102879de0483b5e"),
	platformCase("arm64", "sha256:45e09956dc667c5eff3583c9d94830261fb1ca0be10a0a7db36266edf5de9e1d"),
	Case{Name: "combo", Args: []string{"--pid=host", "--uts=host", "--ipc=host", "--cgroupns=host", "--restart=on-failure:2", alpine, "true"}},
)

// platformCase: --platform=linux/<arch> on alpine 3.20's image for arch,
// pulled by its manifest digest under a private tag. On each daemon one
// arch is native (runlike omits --platform), the other foreign. Never pull
// --platform into the shared alpine tag: it would move the tag to a
// foreign image under every case running in parallel.
func platformCase(arch, digest string) Case {
	img := "{p}-img:" + arch
	return Case{Name: "platform-" + arch,
		Setup:    [][]string{{"pull", "-q", "alpine@" + digest}, {"tag", "alpine@" + digest, img}},
		Args:     []string{"--platform=linux/" + arch, img, "true"},
		Teardown: [][]string{{"rmi", img}}}
}

// Real, unedited: Docker 29.9.3 on Windows Server 2022 (runlike-win),
// docker create --isolation=hyperv nanoserver:ltsc2022. The daemon stores
// it, but this VM cannot start a Hyper-V container (no usable nested
// hypervisor), so starting hyperv is untested; create also round-trips
// live (cases_win_test.go, isolation-hyperv).
var _ = addSaved(SavedCase{Name: "runtime-isolation-windows", Want: []string{"--isolation=hyperv"}})
