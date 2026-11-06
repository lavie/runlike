// Package daemon gathers what runlike needs from a Docker daemon by
// running the docker CLI, so contexts and DOCKER_HOST apply. The CLI and
// the round-trip tests share it.
package daemon

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/lavie/runlike/v2"
)

// CLI runs the docker binary Bin ("docker" when empty).
type CLI struct{ Bin string }

// Inspect returns the container's inspect data, with the inspect JSON of
// the image a copy would run (none when no image is left), and the facts
// about its daemon. Warnings name what could not be read; a missing image
// is never an error.
func (d CLI) Inspect(name string) (in runlike.Inspect, dm runlike.Daemon, warnings []string, err error) {
	fail := func(err error) (runlike.Inspect, runlike.Daemon, []string, error) {
		return runlike.Inspect{}, runlike.Daemon{}, nil, err
	}
	in.Container, err = d.run(nil, "container", "inspect", "--", name)
	if err != nil {
		if strings.Contains(err.Error(), "No such container") {
			return fail(fmt.Errorf("No such container %s", name))
		}
		return fail(err)
	}
	var cs []struct {
		ID     string `json:"Id"`
		Image  string
		Config struct{ Image string }
		State  struct {
			Running bool
			Status  string
		}
		HostConfig      struct{ NetworkMode, IpcMode, PidMode string }
		NetworkSettings struct{ Networks map[string]json.RawMessage }
	}
	if err := json.Unmarshal(in.Container, &cs); err != nil || len(cs) == 0 {
		return fail(fmt.Errorf("unexpected docker container inspect output: %v", err))
	}
	c := cs[0]

	// The copy runs Config.Image (the tag), so compare with that image;
	// runlike.Args warns when it is not the container's own.
	in.Image, err = d.run(nil, "image", "inspect", "--", c.Config.Image)
	if err != nil && c.Image != "" {
		if in.Image, err = d.run(nil, "image", "inspect", "--", c.Image); err == nil {
			warnings = append(warnings, fmt.Sprintf("image %q not found: compared with the container's image %s; docker run will pull %q",
				c.Config.Image, short(c.Image), c.Config.Image))
		}
	}
	if err != nil {
		in.Image = nil
		// %q: the name and docker's message are container data (no raw newline).
		warnings = append(warnings, fmt.Sprintf("image %q not found, nor the container's image: %q", c.Config.Image, err.Error()))
	}

	p, err := d.run(nil, "version", "--format", "{{.Server.Os}}/{{.Server.Arch}}")
	if err != nil {
		return fail(err)
	}
	dm.Platform = strings.TrimSpace(string(p))

	var info struct {
		ID, Name, LoggingDriver, DefaultRuntime, CgroupVersion, Isolation, OperatingSystem string
	}
	b, err := d.run(nil, "info", "--format", "{{json .}}")
	if err == nil {
		err = json.Unmarshal(b, &info)
	}
	if err != nil {
		return fail(fmt.Errorf("docker info: %v", err))
	}
	dm.HostName = info.Name
	dm.Desktop = info.OperatingSystem == "Docker Desktop"
	dm.Defaults.LogDriver = info.LoggingDriver
	dm.Defaults.Runtime = info.DefaultRuntime
	dm.Defaults.Isolation = info.Isolation
	if info.CgroupVersion == "1" {
		dm.Defaults.CgroupnsMode = "host" // built-in default on cgroup v1
	}

	// Names of the container: targets only; a target that is gone keeps
	// its ID.
	var ids []string
	for _, m := range []string{c.HostConfig.NetworkMode, c.HostConfig.IpcMode, c.HostConfig.PidMode} {
		if id, ok := strings.CutPrefix(m, "container:"); ok && id != "" && !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	for _, id := range ids {
		if b, err := d.run(nil, "container", "inspect", "--format", "{{.Name}}", "--", id); err == nil {
			if dm.ContainerNames == nil {
				dm.ContainerNames = map[string]string{}
			}
			dm.ContainerNames[id] = strings.TrimPrefix(strings.TrimSpace(string(b)), "/")
		}
	}

	// --network <ID>: a created container lists the network by name only.
	if m := c.HostConfig.NetworkMode; m != "" && c.NetworkSettings.Networks[m] == nil && !strings.HasPrefix(m, "container:") {
		if b, err := d.run(nil, "network", "inspect", "--format", "{{.Name}}", "--", m); err == nil {
			dm.NetworkNames = map[string]string{m: strings.TrimSpace(string(b))}
		}
	}

	// API 1.52 dropped Config.MacAddress; 1.51 still returns the MAC pinned
	// on the first network (moby's DesiredMacAddress), for a created,
	// running or stopped container; the key is absent when none was pinned
	// (checked on 29.9.3). A daemon older than API 1.51 refuses the
	// request, but its own inspect (in.Container) is API <= 1.51 data,
	// which runlike.Args reads; a created container shows its pinned MAC
	// in NetworkSettings anyway.
	env := []string{"DOCKER_API_VERSION=" + runlike.MACAPIVersion}
	// Docker Desktop's API proxy (docker.sock) drops Config.MacAddress once
	// a container has started; the daemon's own socket keeps it (checked
	// on Desktop 29.9.2). Use that socket when it serves this very daemon.
	raw := ""
	if dm.Desktop {
		raw = d.desktopRawSocket(info.ID)
		if raw != "" {
			env = append(env, "DOCKER_HOST=unix://"+raw)
		}
	}
	b, err = d.run(env, "container", "inspect", "--", c.ID)
	var old []struct{ Config struct{ MacAddress string } }
	switch {
	case err == nil && json.Unmarshal(b, &old) == nil && len(old) > 0:
		in.ConfigMACAddress = old[0].Config.MacAddress
		if dm.Desktop && raw == "" && c.State.Status != "created" && in.ConfigMACAddress == "" {
			warnings = append(warnings, "Docker Desktop's API hides a MAC address pinned with --mac-address once the container has started, so none is printed; if one was pinned, add --mac-address=...")
		}
	case !oldAPIData(in.Container) && c.State.Status != "created":
		warnings = append(warnings, fmt.Sprintf("whether a MAC address was pinned (--mac-address) cannot be read from this daemon, so none is printed (API %s inspect: %v)", runlike.MACAPIVersion, err))
	}
	return in, dm, warnings, nil
}

// desktopRawSocket returns the path of Docker Desktop's own daemon socket
// on macOS when it serves the daemon with ID id, else "".
func (d CLI) desktopRawSocket(id string) string {
	home, err := os.UserHomeDir()
	if err != nil || id == "" {
		return ""
	}
	raw := filepath.Join(home, "Library/Containers/com.docker.docker/Data/docker.raw.sock")
	if _, err := os.Stat(raw); err != nil {
		return ""
	}
	b, err := d.run([]string{"DOCKER_HOST=unix://" + raw}, "info", "--format", "{{.ID}}")
	if err != nil || strings.TrimSpace(string(b)) != id {
		return ""
	}
	return raw
}

// oldAPIData reports whether inspect JSON is API <= 1.51 output (which
// the CLI negotiates with an older daemon): it has Config.MacAddress or
// the top-level NetworkSettings.MacAddress, both dropped in API 1.52.
func oldAPIData(b []byte) bool {
	var v []struct{ Config, NetworkSettings map[string]json.RawMessage }
	if json.Unmarshal(b, &v) != nil || len(v) == 0 {
		return false
	}
	_, ok := v[0].Config["MacAddress"]
	_, ok2 := v[0].NetworkSettings["MacAddress"]
	return ok || ok2
}

func (d CLI) run(env []string, args ...string) ([]byte, error) {
	bin := d.Bin
	if bin == "" {
		bin = "docker"
	}
	var out, errOut bytes.Buffer
	cmd := exec.Command(bin, args...)
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(errOut.String()); msg != "" {
			return nil, errors.New(msg)
		}
		return nil, fmt.Errorf("docker %s: %w", strings.Join(args, " "), err)
	}
	return out.Bytes(), nil
}

func short(id string) string {
	id = strings.TrimPrefix(id, "sha256:")
	return id[:min(12, len(id))]
}
