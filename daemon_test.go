package runlike

import (
	"slices"
	"strings"
	"testing"
	"time"

	dockerspec "github.com/moby/docker-image-spec/specs-go/v1"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// TestDaemonData: daemon defaults, container names, image warnings and
// the health and env edge cases, on hand-made inspect data.
func TestDaemonData(t *testing.T) {
	peer := strings.Repeat("ab", 32)
	img := func(cfg dockerspec.DockerOCIImageConfig) *image.InspectResponse {
		return &image.InspectResponse{ID: "sha256:" + strings.Repeat("1", 64), Config: &cfg}
	}
	for _, tc := range []struct {
		name     string
		os       string // container platform; "" is linux
		cfg      container.Config
		hc       container.HostConfig
		img      *image.InspectResponse
		d        Daemon
		want     []string // in the options
		not      []string // not in the options
		warnings []string // substrings, one per warning, in order
	}{
		{name: "json-file on json-file daemon", hc: container.HostConfig{LogConfig: container.LogConfig{Type: "json-file"}},
			d: Daemon{Defaults: DaemonDefaults{LogDriver: "json-file"}}, not: []string{"--log-driver=json-file"}},
		{name: "json-file on local daemon", hc: container.HostConfig{LogConfig: container.LogConfig{Type: "json-file"}},
			d: Daemon{Defaults: DaemonDefaults{LogDriver: "local"}}, want: []string{"--log-driver=json-file"}},
		{name: "local on local daemon", hc: container.HostConfig{LogConfig: container.LogConfig{Type: "local"}},
			d: Daemon{Defaults: DaemonDefaults{LogDriver: "local"}}, want: []string{"--log-driver=local"}},
		{name: "runc on other-runtime daemon", hc: container.HostConfig{Runtime: "runc"},
			d: Daemon{Defaults: DaemonDefaults{Runtime: "crun"}}, want: []string{"--runtime=runc"}},
		{name: "runc, unknown daemon", hc: container.HostConfig{Runtime: "runc"}, not: []string{"--runtime=runc"}},
		{name: "cgroupns private on v1 daemon", hc: container.HostConfig{CgroupnsMode: "private"},
			d: Daemon{Defaults: DaemonDefaults{CgroupnsMode: "host"}}, want: []string{"--cgroupns=private"}},
		{name: "ipc private on shareable daemon", hc: container.HostConfig{IpcMode: "private"},
			d: Daemon{Defaults: DaemonDefaults{IpcMode: "shareable"}}, want: []string{"--ipc=private"}},
		{name: "shm 64m on 128m daemon", hc: container.HostConfig{ShmSize: 64 << 20},
			d: Daemon{Defaults: DaemonDefaults{ShmSize: 128 << 20}}, want: []string{"--shm-size=64m"}},
		{name: "shm 64m, unknown daemon", hc: container.HostConfig{ShmSize: 64 << 20}, not: []string{"--shm-size=64m"}},
		{name: "container targets by name", hc: container.HostConfig{IpcMode: container.IpcMode("container:" + peer), PidMode: container.PidMode("container:" + peer)},
			d: Daemon{ContainerNames: map[string]string{peer: "db"}}, want: []string{"--ipc=container:db", "--pid=container:db"}},
		{name: "container targets by ID", hc: container.HostConfig{IpcMode: container.IpcMode("container:" + peer)},
			want: []string{"--ipc=container:" + peer}},
		{name: "health NONE on NONE image keeps timing",
			cfg:  container.Config{Healthcheck: &container.HealthConfig{Test: []string{"NONE"}, Interval: 5 * time.Second, Retries: 3}},
			img:  img(dockerspec.DockerOCIImageConfig{DockerOCIImageConfigExt: dockerspec.DockerOCIImageConfigExt{Healthcheck: &dockerspec.HealthcheckConfig{Test: []string{"NONE"}}}}),
			want: []string{"--health-interval=5s", "--health-retries=3"}, not: []string{"--no-healthcheck"}},
		{name: "no health check, image has one",
			img:  img(dockerspec.DockerOCIImageConfig{DockerOCIImageConfigExt: dockerspec.DockerOCIImageConfigExt{Healthcheck: &dockerspec.HealthcheckConfig{Test: []string{"CMD-SHELL", "true"}}}}),
			want: []string{"--no-healthcheck"}},
		{name: "exec-form health check, windows", os: "windows",
			cfg:  container.Config{Healthcheck: &container.HealthConfig{Test: []string{"CMD", "ping", "a b"}}},
			want: []string{"--health-cmd=ping 'a b'"}, warnings: []string{"no image data", "this Windows container runs it with cmd /S /C"}},
		{name: "exec-form health check, linux",
			cfg:  container.Config{Healthcheck: &container.HealthConfig{Test: []string{"CMD", "ping", "a b"}}},
			want: []string{"--health-cmd=ping 'a b'"}, warnings: []string{"no image data"}},
		{name: "bare and duplicate env", cfg: container.Config{Env: []string{"A", "B=1", "B=2"}},
			want: []string{"--env=A", "--env=B=1", "--env=B=2"}, warnings: []string{"no image data", "--env=A has no value", "env B is set more than once"}},
		{name: "stdin, no entrypoint", warnings: []string{"cleared with --entrypoint="}},
		{name: "image moved", cfg: container.Config{Env: []string{"V=1"}, Labels: map[string]string{"l": "1"}},
			img: img(dockerspec.DockerOCIImageConfig{ImageConfig: imgcfg(func(c *ocispec.ImageConfig) {
				c.Env = []string{"V=2", "NEW=x"}
				c.Labels = map[string]string{"l": "2", "nl": "y", "desktop.docker.io/ports.scheme": "v2"}
				c.User = "nobody"
			})}),
			want:     []string{"--env=V=1", "--label=l=1"},
			warnings: []string{"image rv:t is now 111111111111, not the container's 000000000000: the copy runs the new image, and values are compared with it; it adds what no flag removes: env NEW, label nl, user nobody"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.cfg.Image = "rv:t"
			c := container.InspectResponse{Config: &tc.cfg, HostConfig: &tc.hc, Image: "sha256:" + strings.Repeat("0", 64), Platform: tc.os}
			cmd := build(c, tc.img, tc.d, Options{}, "", false)
			for _, w := range tc.want {
				if !slices.Contains(cmd.Options, w) {
					t.Errorf("missing %q in %q", w, cmd.Options)
				}
			}
			for _, w := range tc.not {
				if slices.Contains(cmd.Options, w) {
					t.Errorf("unwanted %q in %q", w, cmd.Options)
				}
			}
			if tc.warnings != nil {
				ok := len(cmd.Warnings) == len(tc.warnings)
				for i := 0; ok && i < len(tc.warnings); i++ {
					ok = strings.Contains(cmd.Warnings[i], tc.warnings[i])
				}
				if !ok {
					t.Errorf("warnings %q, want %q", cmd.Warnings, tc.warnings)
				}
			}
		})
	}
}

func imgcfg(f func(*ocispec.ImageConfig)) ocispec.ImageConfig {
	var c ocispec.ImageConfig
	f(&c)
	return c
}

// TestStdinMAC: Config.MacAddress in API 1.51 data reaches ConfigMACAddress.
func TestStdinMAC(t *testing.T) {
	js := `[{"Id":"x","State":{"Running":false},"Config":{"Image":"alpine","MacAddress":"02:00:00:00:00:42"},"HostConfig":{"NetworkMode":"bridge"},` +
		`"NetworkSettings":{"Networks":{"bridge":{"MacAddress":""}}}}]`
	cmd, err := Args(Inspect{Container: []byte(js)}, Daemon{}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(cmd.Options, "--mac-address=02:00:00:00:00:42") {
		t.Errorf("no --mac-address in %q", cmd.Options)
	}
}
