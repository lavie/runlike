package runlike

import (
	"slices"
	"testing"

	"github.com/moby/moby/api/types/container"
)

func TestBytesArg(t *testing.T) {
	for n, want := range map[int64]string{
		1: "1", 1000: "1000", 1024: "1k", 1536: "1536", 1 << 20: "1m", 64 << 20: "64m",
		3 << 30: "3g", 1<<30 + 1<<20: "1025m", 5 << 40: "5t", 2 << 50: "2p", 1 << 62: "4096p", -1: "-1",
	} {
		if got := bytesArg(n); got != want {
			t.Errorf("bytesArg(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestCpusArg(t *testing.T) {
	for n, want := range map[int64]string{
		1e9: "1", 15e8: "1.5", 1e6: "0.001", 1: "0.000000001", 3141592653: "3.141592653", 100e9: "100",
	} {
		if got := cpusArg(n); got != want {
			t.Errorf("cpusArg(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestGpusArg(t *testing.T) {
	gpu := [][]string{{"gpu"}}
	for _, c := range []struct {
		r    container.DeviceRequest
		want string // "" means not a --gpus request
	}{
		{container.DeviceRequest{Count: -1, Capabilities: gpu}, "all"},
		{container.DeviceRequest{Count: 1, Capabilities: gpu}, "1"},
		{container.DeviceRequest{DeviceIDs: []string{"0", "2"}, Capabilities: gpu}, `"device=0,2"`},
		{container.DeviceRequest{DeviceIDs: []string{"1"}, Count: 2, Capabilities: gpu}, "device=1,count=2"},
		{container.DeviceRequest{Count: -1, Driver: "nvidia", Capabilities: [][]string{{"compute", "utility", "gpu"}}},
			`all,driver=nvidia,"capabilities=compute,utility"`},
		{container.DeviceRequest{Count: 2, Capabilities: gpu, Options: map[string]string{"b": "x y", "a": "1"}},
			`2,"options=a=1,b=x y"`},
		{container.DeviceRequest{Driver: "cdi", DeviceIDs: []string{"vendor.com/gpu=0"}}, ""}, // --device CDI
		{container.DeviceRequest{Count: 1, Capabilities: [][]string{{"gpu"}, {"compute"}}}, ""},
	} {
		got, ok := gpusArg(c.r)
		if ok != (c.want != "") || got != c.want {
			t.Errorf("gpusArg(%+v) = %q, %v; want %q", c.r, got, ok, c.want)
		}
	}
}

func TestResourcesDefaults(t *testing.T) {
	minus1, f := int64(-1), false
	hc := &container.HostConfig{Resources: container.Resources{
		MemorySwappiness: &minus1, OomKillDisable: &f, PidsLimit: &minus1,
		// Windows-only fields are printed only for a Windows container.
		CPUCount: 2, CPUPercent: 10, IOMaximumIOps: 5, IOMaximumBandwidth: 1 << 20,
	}, ShmSize: builtinShmSize}
	if a := resourcesArgs(&ctx{hc: hc, OS: "linux"}); len(a) != 0 {
		t.Errorf("defaults printed %q", a)
	}
	// A Windows daemon stores ShmSize 0 unless --shm-size is given, so
	// 64m there was set by the user.
	a := resourcesArgs(&ctx{hc: hc, OS: "windows"})
	if want := []string{"--shm-size=64m", "--cpu-count=2", "--cpu-percent=10", "--io-maxbandwidth=1m", "--io-maxiops=5"}; !slices.Equal(a, want) {
		t.Errorf("windows: %q, want %q", a, want)
	}
}
