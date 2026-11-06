package roundtrip

import "github.com/lavie/runlike/v2"

// Windows cases: they run only against a Windows daemon (OS "windows"),
// and the Linux cases skip there. Test daemon: runlike-win, Windows
// Server 2022 (build 20348), Docker 29.9.3, process isolation only
// (Hyper-V isolation needs a hypervisor the VM lacks). Bind sources must
// exist on the Windows host: C:\ProgramData and C:\Program Files do.

func win(cs ...Case) []Case {
	for i := range cs {
		cs[i].OS = "windows"
	}
	return cs
}

var _ = add("windows", win(
	Case{Name: "plain", Args: []string{nano, "cmd", "/c", "ver"}},
	Case{Name: "isolation-process", Args: []string{"--isolation=process", nano}},
	// The daemon stores hyperv even though this host cannot start it.
	Case{Name: "isolation-hyperv", Args: []string{"--isolation=hyperv", nano}},
	Case{Name: "isolation-default", Args: []string{"--isolation=default", nano}},
	Case{Name: "cpu-count", Args: []string{"--cpu-count=2", nano}},
	Case{Name: "cpu-percent", Args: []string{"--cpu-percent=50", nano}},
	Case{Name: "cpus", Args: []string{"--cpus=1.5", nano}},
	Case{Name: "cpu-shares", Args: []string{"--cpu-shares=512", nano}},
	Case{Name: "io-max", Args: []string{"--io-maxbandwidth=10m", "--io-maxiops=300", nano}},
	Case{Name: "memory", Args: []string{"--memory=512m", nano}},
	Case{Name: "env-label", Args: []string{"--env=A=b c", "--env=PATHX=C:\\x;C:\\y", "--label=a=b", "--label=w=C:\\x\\", nano}},
	Case{Name: "entrypoint", Args: []string{"--entrypoint=cmd", nano, "/c", "echo", "hi & bye"}},
	Case{Name: "workdir", Args: []string{"--workdir=C:\\Windows\\Temp", nano}},
	Case{Name: "workdir-space", Args: []string{"--workdir=C:\\Program Files\\x", nano}},
	Case{Name: "user-admin", Args: []string{"--user=ContainerAdministrator", nano}},
	Case{Name: "user-user", Args: []string{"--user=ContainerUser", nano}},
	Case{Name: "bind", Args: []string{"--volume=C:\\ProgramData:C:\\data", "--volume=C:\\Program Files:C:\\pf:ro", nano}},
	Case{Name: "named-volume",
		Setup:    [][]string{{"volume", "create", "{p}-vol"}},
		Args:     []string{"--volume={p}-vol:C:\\data", nano},
		Teardown: [][]string{{"volume", "rm", "{p}-vol"}}},
	Case{Name: "anon-volume", Args: []string{"--volume=C:\\data", nano}},
	Case{Name: "mount-bind", Args: []string{"--mount=type=bind,source=C:\\ProgramData,target=C:\\data,readonly", nano}},
	Case{Name: "mount-volume",
		Setup:    [][]string{{"volume", "create", "{p}-mvol"}},
		Args:     []string{"--mount=type=volume,source={p}-mvol,target=C:\\data", nano},
		Teardown: [][]string{{"volume", "rm", "{p}-mvol"}}},
	Case{Name: "npipe-mount", Args: []string{`--mount=type=npipe,source=\\.\pipe\docker_engine,target=\\.\pipe\docker_engine`, nano}},
	Case{Name: "npipe-volume", Args: []string{`--volume=\\.\pipe\docker_engine:\\.\pipe\docker_engine`, nano}},
	Case{Name: "network-nat", Args: []string{"--network=nat", nano}},
	Case{Name: "network-none", Args: []string{"--network=none", nano}},
	Case{Name: "network-user",
		Setup:    [][]string{{"network", "create", "--driver=nat", "{p}-net"}},
		Args:     []string{"--network={p}-net", "--network-alias=web", nano},
		Teardown: [][]string{{"network", "rm", "{p}-net"}}},
	Case{Name: "publish", Args: []string{"--publish=8080:80", "--publish=127.0.0.1:8081:81/udp", "--expose=90", nano}},
	Case{Name: "dns", Args: []string{"--dns=8.8.8.8", "--dns-search=example.com", "--dns-option=ndots:2", nano}},
	Case{Name: "add-host", Args: []string{"--add-host=foo:1.2.3.4", nano}},
	Case{Name: "restart", Args: []string{"--restart=on-failure:3", nano}},
	Case{Name: "restart-always", Args: []string{"--restart=always", nano}},
	Case{Name: "stop", Args: []string{"--stop-timeout=20", "--stop-signal=SIGTERM", nano}},
	Case{Name: "storage-opt", Args: []string{"--storage-opt=size=30G", nano}},
	Case{Name: "hostname", Args: []string{"--hostname=winhost", "--domainname=example.com", nano}},
	Case{Name: "mac", Args: []string{"--mac-address=00:15:5d:01:02:03", nano}},
	Case{Name: "no-name", Args: []string{"--label=x=y", nano}, Opts: runlike.Options{NoName: true, NoLabels: true}, Ignore: []Ignore{{"Config.Labels", "--no-labels drops them by design"}}},
	Case{Name: "log", Args: []string{"--log-driver=json-file", "--log-opt=max-size=10m", nano}},
	Case{Name: "log-none", Args: []string{"--log-driver=none", nano}},
	Case{Name: "health", Args: []string{"--health-cmd=cmd /c exit 0", "--health-interval=30s", "--health-start-period=5s", nano}},
	Case{Name: "tty", Args: []string{"--tty", "--interactive", nano}},
	Case{Name: "rm", Args: []string{"--rm", nano}},
	Case{Name: "credentialspec", Args: []string{"--security-opt=credentialspec=file://x.json", nano}},
	Case{Name: "annotation", Args: []string{"--annotation=a=b", nano}},
	// Accepted by the Windows daemon on create although Windows ignores
	// or cannot use them; runlike must still print what it stored.
	Case{Name: "network-host", Args: []string{"--network=host", nano}},
	Case{Name: "namespaces", Args: []string{"--ipc=host", "--pid=host", "--uts=host", "--userns=host", "--cgroupns=private", nano}},
	Case{Name: "ipc-shm", Args: []string{"--ipc=private", "--shm-size=64m", nano}},
	Case{Name: "network-host-hostname", Args: []string{"--network=host", "--hostname=winhost", nano}},
	Case{Name: "binds-three", Args: []string{"--volume=C:\\ProgramData:C:\\data", "--volume=C:\\Program Files:C:\\pf:ro", "--volume=C:\\Windows\\Temp:C:\\t", nano}},
	Case{Name: "run-runc", Run: true, Args: []string{"--runtime=runc", nano, "cmd", "/c", "ver"}},
	Case{Name: "runtime-runc", Args: []string{"--runtime=runc", nano}},
	Case{Name: "cap-drop", Args: []string{"--cap-drop=ALL", "--security-opt=no-new-privileges", nano}},
	Case{Name: "shm-oom-quota", Args: []string{"--shm-size=128m", "--oom-score-adj=10", "--cpu-quota=50000", nano}},
	Case{Name: "init", Args: []string{"--init", nano}},
	Case{Name: "tmpfs", Args: []string{"--tmpfs=C:\\t", nano}},
	Case{Name: "group-add", Args: []string{"--group-add=foo", nano}},
	Case{Name: "device-class", Args: []string{"--device=class/5B45201D-F2F2-4F3B-85BB-30FF1F953599", nano}},
	Case{Name: "device-vpci", Args: []string{"--device=vpci-location-path://PCIROOT(0)#PCI(0100)", nano}},
	Case{Name: "gpus", Args: []string{"--gpus=all", nano}},
	// own network: the default nat's subnet differs per host (0.0.0.0/0 on
	// runlike-win, a real one on GitHub's windows-2022, which refuses 172.20.0.5)
	Case{Name: "ip", Setup: [][]string{{"network", "create", "-d", "nat", "--subnet=172.31.250.0/24", "{p}-nat"}},
		Args:     []string{"--network={p}-nat", "--ip=172.31.250.5", "--link-local-ip=169.254.1.1", nano},
		Teardown: [][]string{{"network", "rm", "{p}-nat"}}},
	Case{Name: "volume-driver", Args: []string{"--volume-driver=local", "--volume=C:\\data", nano}},
	Case{Name: "volumes-from",
		Setup: [][]string{{"create", "--name={p}-src", "--label=" + sweepLabel, "--volume=C:\\data", nano}},
		Args:  []string{"--volumes-from={p}-src", nano}},
	Case{Name: "link",
		Setup: [][]string{{"create", "--name={p}-peer", "--label=" + sweepLabel, nano}},
		Args:  []string{"--link={p}-peer:alias", nano}},
	Case{Name: "log-etwlogs", Args: []string{"--log-driver=etwlogs", nano}},
	Case{Name: "log-local", Args: []string{"--log-driver=local", nano}},
	Case{Name: "health-all", Args: []string{"--health-cmd=cmd /c exit 0", "--health-retries=3", "--health-timeout=5s", "--health-start-interval=2s", nano}},
	Case{Name: "no-healthcheck", Args: []string{"--no-healthcheck", nano}},
	Case{Name: "entrypoint-empty", Args: []string{"--entrypoint=", nano, "cmd"}},
	Case{Name: "expose-range", Args: []string{"--expose=90-92/udp", "--publish-all", nano}},
	Case{Name: "platform", Args: []string{"--platform=windows/amd64", nano}},
	Case{Name: "volume-drive", Args: []string{"--volume=C:\\ProgramData:D:", nano}},
	Case{Name: "memory-small", Args: []string{"--memory=4m", "--io-maxbandwidth=1k", nano}},
	Case{Name: "cpus-count", Args: []string{"--cpus=1", "--cpu-count=1", nano}},
	Case{Name: "run", Run: true, Args: []string{"--hostname=winrun", nano, "cmd", "/c", "ver"}},
	Case{Name: "run-plain", Run: true, Args: []string{nano, "cmd", "/c", "ver"}},
)...)
