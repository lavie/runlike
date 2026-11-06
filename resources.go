package runlike

import (
	"bytes"
	"encoding/csv"
	"math/big"
	"strconv"
	"strings"

	"github.com/moby/moby/api/types/blkiodev"
	"github.com/moby/moby/api/types/container"
)

// resourcesArgs: cgroup parent, CPU, memory, block IO, PIDs, OOM, shm,
// ulimits, GPUs, and the Windows-only CPU and IO limits.
func resourcesArgs(x *ctx) []string {
	var a []string
	h := x.hc
	str := func(flag, v string) {
		if v != "" {
			a = append(a, flag+"="+v)
		}
	}
	num := func(flag string, v int64) {
		if v != 0 {
			a = append(a, flag+"="+strconv.FormatInt(v, 10))
		}
	}
	size := func(flag string, v int64) {
		if v != 0 {
			a = append(a, flag+"="+bytesArg(v))
		}
	}

	str("--cgroup-parent", h.CgroupParent)

	// CPU
	if h.NanoCPUs != 0 {
		a = append(a, "--cpus="+cpusArg(h.NanoCPUs))
	}
	num("--cpu-shares", h.CPUShares)
	num("--cpu-period", h.CPUPeriod)
	num("--cpu-quota", h.CPUQuota)
	num("--cpu-rt-period", h.CPURealtimePeriod)
	num("--cpu-rt-runtime", h.CPURealtimeRuntime)
	str("--cpuset-cpus", h.CpusetCpus)
	str("--cpuset-mems", h.CpusetMems)

	// Memory. With --memory and no --memory-swap the daemon stores
	// MemorySwap = 2 * Memory: that is Docker's default, so leave it out.
	size("--memory", h.Memory)
	size("--memory-reservation", h.MemoryReservation)
	switch {
	case h.MemorySwap == -1:
		a = append(a, "--memory-swap=-1")
	case h.MemorySwap != 0 && h.MemorySwap != 2*h.Memory:
		a = append(a, "--memory-swap="+bytesArg(h.MemorySwap))
	}
	// -1 is the CLI's "not set"; cgroup v2 daemons store null.
	if p := h.MemorySwappiness; p != nil && *p != -1 {
		a = append(a, "--memory-swappiness="+strconv.FormatInt(*p, 10))
	}
	if p := h.OomKillDisable; p != nil && *p {
		a = append(a, "--oom-kill-disable")
	}
	num("--oom-score-adj", int64(h.OomScoreAdj))
	// The daemon stores 0 and -1 (unlimited, the default) as null.
	if p := h.PidsLimit; p != nil && *p > 0 {
		a = append(a, "--pids-limit="+strconv.FormatInt(*p, 10))
	}
	// Windows stores 0 unless --shm-size is given, so print what it stores.
	if d := x.d.Defaults.ShmSize; h.ShmSize != 0 && (x.OS == "windows" || h.ShmSize != builtinShmSize || d != 0 && d != builtinShmSize) {
		a = append(a, "--shm-size="+bytesArg(h.ShmSize))
	}

	// Block IO
	num("--blkio-weight", int64(h.BlkioWeight))
	for _, d := range h.BlkioWeightDevice {
		a = append(a, "--blkio-weight-device="+d.Path+":"+strconv.FormatUint(uint64(d.Weight), 10))
	}
	throttle := func(flag string, list []*blkiodev.ThrottleDevice, bps bool) {
		for _, d := range list {
			rate := strconv.FormatUint(d.Rate, 10)
			if bps && d.Rate <= 1<<63-1 {
				rate = bytesArg(int64(d.Rate))
			}
			a = append(a, flag+"="+d.Path+":"+rate)
		}
	}
	throttle("--device-read-bps", h.BlkioDeviceReadBps, true)
	throttle("--device-write-bps", h.BlkioDeviceWriteBps, true)
	throttle("--device-read-iops", h.BlkioDeviceReadIOps, false)
	throttle("--device-write-iops", h.BlkioDeviceWriteIOps, false)

	// Ulimits. HostConfig holds only --ulimit values until the container
	// starts; then the daemon merges its default-ulimits in. Docker's
	// built-in default is none, so every entry is printed.
	for _, u := range h.Ulimits {
		v := strconv.FormatInt(u.Soft, 10)
		if u.Hard != u.Soft {
			v += ":" + strconv.FormatInt(u.Hard, 10)
		}
		a = append(a, "--ulimit="+u.Name+"="+v)
	}

	for _, r := range h.DeviceRequests {
		if g, ok := gpusArg(r); ok {
			x.eqKeys("--gpus options", r.Options)
			a = append(a, "--gpus="+g)
		}
	}

	// Windows only: the CLI refuses these against a Linux daemon.
	if x.OS == "windows" {
		num("--cpu-count", h.CPUCount)
		num("--cpu-percent", h.CPUPercent)
		if h.IOMaximumBandwidth != 0 && h.IOMaximumBandwidth <= 1<<63-1 {
			a = append(a, "--io-maxbandwidth="+bytesArg(int64(h.IOMaximumBandwidth)))
		}
		if h.IOMaximumIOps != 0 {
			a = append(a, "--io-maxiops="+strconv.FormatUint(h.IOMaximumIOps, 10))
		}
	}
	return a
}

// bytesArg prints a byte count in the largest unit that holds it exactly
// ("64m", "1536k", "1000"); the CLI's RAMInBytes reads these units as
// powers of 1024.
func bytesArg(n int64) string {
	if n <= 0 {
		return strconv.FormatInt(n, 10)
	}
	for _, u := range []struct {
		s     string
		shift uint
	}{{"p", 50}, {"t", 40}, {"g", 30}, {"m", 20}, {"k", 10}} {
		if n&(1<<u.shift-1) == 0 {
			return strconv.FormatInt(n>>u.shift, 10) + u.s
		}
	}
	return strconv.FormatInt(n, 10)
}

// cpusArg prints NanoCPUs as an exact decimal number of CPUs ("1.5").
func cpusArg(nano int64) string {
	s := big.NewRat(nano, 1e9).FloatString(9)
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

// gpusArg inverts the CLI's --gpus parser (opts.GpuOpts). It takes only
// requests with the gpu capability; CDI device requests (driver cdi, from
// --device) are not GPU requests. The value is one CSV record, so fields
// holding commas (device=0,1) come out quoted, as the reference shows:
// --gpus '"device=0,1"'.
func gpusArg(r container.DeviceRequest) (string, bool) {
	if len(r.Capabilities) != 1 {
		return "", false
	}
	caps := r.Capabilities[0]
	if len(caps) == 0 || caps[len(caps)-1] != "gpu" {
		return "", false // the CLI appends "gpu" to every capability set
	}
	// Without device IDs the count is a bare field ("all", "2"); with them,
	// count defaults to 0.
	var f []string
	count := strconv.Itoa(r.Count)
	if r.Count == -1 {
		count = "all"
	}
	if r.DeviceIDs == nil {
		f = append(f, count)
	} else {
		f = append(f, "device="+strings.Join(r.DeviceIDs, ","))
		if r.Count != 0 {
			f = append(f, "count="+count)
		}
	}
	if r.Driver != "" {
		f = append(f, "driver="+r.Driver)
	}
	if c := caps[:len(caps)-1]; len(c) > 0 {
		f = append(f, "capabilities="+strings.Join(c, ","))
	}
	if len(r.Options) > 0 {
		var kv []string
		for _, k := range sortedKeys(r.Options) {
			kv = append(kv, k+"="+r.Options[k])
		}
		f = append(f, "options="+csvRecord(kv))
	}
	return csvRecord(f), true
}

func csvRecord(fields []string) string {
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	_ = w.Write(fields)
	w.Flush()
	return strings.TrimSuffix(b.String(), "\n")
}
