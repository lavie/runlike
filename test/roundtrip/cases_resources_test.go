package roundtrip

// Resources area: CPU, memory, block IO, PIDs, OOM, shm, ulimits, GPUs.
// Test daemon: Lima VM, Ubuntu kernel 7.0, cgroup v2, block devices
// /dev/vda and /dev/vdb, one NUMA node, 4 CPUs.
var _ = add("resources",
	Case{Name: "defaults", Args: []string{alpine, "true"}},
	Case{Name: "cgroup-parent", Args: []string{"--cgroup-parent=rt-parent.slice", alpine, "true"}},

	// CPU
	Case{Name: "cpus-int", Args: []string{"--cpus=2", alpine, "true"}},
	Case{Name: "cpus-frac", Args: []string{"--cpus=1.5", alpine, "true"}},
	Case{Name: "cpus-tiny", Args: []string{"--cpus=0.001", alpine, "true"}},
	Case{Name: "cpus-nano", Args: []string{"--cpus=0.000000001", alpine, "true"}},
	Case{Name: "cpus-long", Args: []string{"--cpus=3.141592653", alpine, "true"}},
	Case{Name: "cpus-run", Run: true, Args: []string{"--cpus=0.5", alpine, "sleep", "300"}},
	Case{Name: "cpu-shares", Args: []string{"-c", "512", alpine, "true"}},
	Case{Name: "cpu-shares-max", Args: []string{"--cpu-shares=262144", alpine, "true"}},
	Case{Name: "cpu-cfs", Args: []string{"--cpu-period=50000", "--cpu-quota=25000", alpine, "true"}},
	Case{Name: "cpu-quota-minus1", Args: []string{"--cpu-quota=-1", alpine, "true"}},
	Case{Name: "cpu-period-only", Args: []string{"--cpu-period=100000", alpine, "true"}},
	Case{Name: "cpuset", Args: []string{"--cpuset-cpus=0-2", "--cpuset-mems=0", alpine, "true"}},
	Case{Name: "cpuset-list", Args: []string{"--cpuset-cpus=0,3", alpine, "true"}},
	Case{Name: "cpuset-run", Run: true, Args: []string{"--cpuset-cpus=1", "--cpuset-mems=0", alpine, "sleep", "300"}},

	// memory
	Case{Name: "memory-unit", Args: []string{"-m", "64m", alpine, "true"}},
	Case{Name: "memory-odd-bytes", Args: []string{"--memory=10000001", alpine, "true"}},
	Case{Name: "memory-giga", Args: []string{"--memory=2g", alpine, "true"}},
	Case{Name: "memory-kilo", Args: []string{"--memory=6500k", alpine, "true"}},
	Case{Name: "memory-reservation", Args: []string{"--memory-reservation=32m", alpine, "true"}},
	Case{Name: "memory-reservation-with-memory", Args: []string{"-m", "128m", "--memory-reservation=96m", alpine, "true"}},
	Case{Name: "memory-swap-implied", Args: []string{"-m", "64m", alpine, "true"}},
	Case{Name: "memory-swap-explicit-2x", Args: []string{"-m", "64m", "--memory-swap=128m", alpine, "true"}},
	Case{Name: "memory-swap-equal", Args: []string{"-m", "64m", "--memory-swap=64m", alpine, "true"}},
	Case{Name: "memory-swap-more", Args: []string{"-m", "64m", "--memory-swap=1g", alpine, "true"}},
	Case{Name: "memory-swap-unlimited", Args: []string{"-m", "64m", "--memory-swap=-1", alpine, "true"}},
	Case{Name: "memory-swap-unlimited-alone", Args: []string{"--memory-swap=-1", alpine, "true"}},
	Case{Name: "memory-run", Run: true, Args: []string{"-m", "100m", "--memory-swap=200m", "--memory-reservation=50m", alpine, "sleep", "300"}},

	// OOM, PIDs, shm
	Case{Name: "oom-score-adj-neg", Args: []string{"--oom-score-adj=-500", alpine, "true"}},
	Case{Name: "oom-score-adj-max", Args: []string{"--oom-score-adj=1000", alpine, "true"}},
	Case{Name: "pids-limit", Args: []string{"--pids-limit=100", alpine, "true"}},
	Case{Name: "pids-limit-unlimited", Args: []string{"--pids-limit=-1", alpine, "true"}},
	Case{Name: "pids-limit-zero", Args: []string{"--pids-limit=0", alpine, "true"}},
	Case{Name: "pids-run", Run: true, Args: []string{"--pids-limit=50", "--oom-score-adj=200", alpine, "sleep", "300"}},
	Case{Name: "shm-default-explicit", Args: []string{"--shm-size=64m", alpine, "true"}},
	Case{Name: "shm-size", Args: []string{"--shm-size=128m", alpine, "true"}},
	Case{Name: "shm-size-odd", Args: []string{"--shm-size=1000000", alpine, "true"}},

	// block IO (the VM's virtio disks)
	Case{Name: "blkio-weight", Args: []string{"--blkio-weight=300", alpine, "true"}},
	Case{Name: "blkio-weight-device", Args: []string{"--blkio-weight-device=/dev/vda:200", "--blkio-weight-device=/dev/vdb:10", alpine, "true"}},
	Case{Name: "device-bps", Args: []string{"--device-read-bps=/dev/vda:1mb", "--device-write-bps=/dev/vda:1000", "--device-write-bps=/dev/vdb:3g", alpine, "true"}},
	Case{Name: "device-iops", Args: []string{"--device-read-iops=/dev/vda:1000", "--device-write-iops=/dev/vda:50", "--device-read-iops=/dev/vdb:7", alpine, "true"}},
	// create stores any path; start needs the host's real disk
	Case{Name: "device-limits-run", Run: true, Args: []string{"--device-read-bps={blk}:10mb", "--device-write-iops={blk}:100", alpine, "sleep", "300"}},

	// ulimits
	Case{Name: "ulimit-soft-hard", Args: []string{"--ulimit=nofile=1024:2048", alpine, "true"}},
	Case{Name: "ulimit-equal", Args: []string{"--ulimit=nproc=500", alpine, "true"}},
	Case{Name: "ulimit-many", Args: []string{"--ulimit=nofile=10:20", "--ulimit=core=-1", "--ulimit=memlock=-1:-1", "--ulimit=stack=8192", alpine, "true"}},
	Case{Name: "ulimit-run", Run: true, Args: []string{"--ulimit=nofile=4096:8192", alpine, "sleep", "300"}},

	// GPUs: create stores the request though the VM has no GPU (start fails).
	Case{Name: "gpus-all", Args: []string{"--gpus=all", alpine, "true"}},
	Case{Name: "gpus-count", Args: []string{"--gpus=2", alpine, "true"}},
	Case{Name: "gpus-count-one", Args: []string{"--gpus=1", alpine, "true"}},
	Case{Name: "gpus-devices", Args: []string{"--gpus", `"device=0,2"`, alpine, "true"}},
	Case{Name: "gpus-device-uuid", Args: []string{"--gpus=device=GPU-3a23c669-1f69-c64e-cf85-44e9b07e7a2a", alpine, "true"}},
	Case{Name: "gpus-full", Args: []string{"--gpus", `"capabilities=compute,utility",driver=nvidia,count=all`, alpine, "true"}},
	Case{Name: "gpus-device-count", Args: []string{"--gpus", `device=1,count=2`, alpine, "true"}},
	Case{Name: "gpus-options", Args: []string{"--gpus", `2,"options=""a=1,b=x y"""`, alpine, "true"}},
	Case{Name: "gpus-two", Args: []string{"--gpus=1", "--gpus=driver=nvidia,device=3", alpine, "true"}},

	// combination
	Case{Name: "combo", Args: []string{"--cgroup-parent=rt.slice", "--cpus=1.25", "-c", "100", "--cpuset-cpus=0-1", "-m", "256m", "--memory-swap=512m",
		"--memory-reservation=128m", "--pids-limit=64", "--oom-score-adj=-17", "--shm-size=32m", "--blkio-weight=500",
		"--device-read-bps=/dev/vda:5m", "--ulimit=nofile=100:200", "--gpus=all", alpine, "true"}},
)

// Saved-data cases. The Lima daemon (Docker 29.9.3, Ubuntu kernel 7.0,
// cgroup v2) discards --oom-kill-disable and --memory-swappiness and
// refuses --cpu-rt-*; the Windows-only flags are refused against a Linux
// daemon. So the Linux files below are the real inspect JSON of a plain
// `docker create alpine:3.20 true` on that daemon with only the named
// HostConfig fields edited by hand. The Windows flags also round-trip
// live in cases_win_test.go.
var _ = addSaved(
	// SYNTHETIC: not from a cgroup v1 daemon. Hand-edited: OomKillDisable
	// true, MemorySwappiness 0. It keeps CgroupnsMode "private", while a
	// real cgroup v1 daemon defaults to "host" (unchecked); replace with
	// JSON saved from a cgroup v1 host.
	SavedCase{Name: "resources-cgroup-v1", Want: []string{"--oom-kill-disable", "--memory-swappiness=0", "--memory=64m"}},
	// hand-edited: CpuRealtimePeriod 1000000, CpuRealtimeRuntime 950000
	SavedCase{Name: "resources-cpu-rt", Want: []string{"--cpu-rt-period=1000000", "--cpu-rt-runtime=950000"}},
	// Real, unedited: Docker 29.9.3 on Windows Server 2022 (runlike-win),
	// docker create --cpu-count=2 --cpu-percent=50 --io-maxiops=300
	// --io-maxbandwidth=10m --cpus=1.5 --memory=512m nanoserver:ltsc2022.
	// The daemon discards --cpu-percent next to --cpu-count (it warns), so
	// CpuPercent is 0.
	SavedCase{Name: "resources-windows", Want: []string{"--cpu-count=2", "--io-maxiops=300",
		"--io-maxbandwidth=10m", "--cpus=1.5", "--memory=512m", "--isolation=process"}},
)
