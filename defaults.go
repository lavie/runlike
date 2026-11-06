package runlike

// Docker v29.9.3's built-in defaults (spec/v29.9.3): a value equal to one
// of these needs no flag, unless the daemon's own default differs (see
// isDefault). Moving the pin starts here.
const (
	builtinLogDriver = "json-file"
	builtinRuntime   = "runc"
	builtinCgroupns  = "private" // on cgroup v2; v1 hosts default to host (Daemon.Defaults)
	builtinIpcMode   = "private"
	builtinShmSize   = 64 << 20 // bytes
	builtinNetwork   = "bridge"

	// Windows daemons (29.9.3 on Windows Server 2022): "default" network is
	// nat; no runtime is stored on create, this one on start. They store
	// no cgroupns, ipc or shm size unless given, and their isolation
	// default depends on the host (Daemon.Defaults.Isolation).
	builtinNetworkWindows = "nat"
	builtinRuntimeWindows = "io.containerd.runhcs.v1"
)

// The paths Docker v29.9.3 masks and makes read-only in a Linux container
// (HostConfig.MaskedPaths, ReadonlyPaths) unless it is privileged or has
// --security-opt=systempaths=unconfined.
var (
	builtinMaskedPaths = []string{"/proc/acpi", "/proc/asound", "/proc/interrupts", "/proc/kcore", "/proc/keys",
		"/proc/latency_stats", "/proc/sched_debug", "/proc/scsi", "/proc/timer_list", "/proc/timer_stats",
		"/sys/devices/virtual/powercap", "/sys/firmware"}
	builtinReadonlyPaths = []string{"/proc/bus", "/proc/fs", "/proc/irq", "/proc/sys", "/proc/sysrq-trigger"}
)

// MACAPIVersion is the newest Docker API version whose container inspect
// still has Config.MacAddress (API 1.52 dropped it). Inspect with
// DOCKER_API_VERSION set to it to fill Inspect.ConfigMACAddress.
const MACAPIVersion = "1.51"
