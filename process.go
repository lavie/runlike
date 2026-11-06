package runlike

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// processArgs: entrypoint, environment, working directory, attach/tty/stdin,
// init, stop signal and timeout, umask.
func processArgs(x *ctx) []string {
	var a []string
	if ep, ok := x.entrypoint(); ok {
		a = append(a, "--entrypoint="+ep)
	}
	// --env-file lands here too: the client reads the file into Config.Env.
	seen := map[string]bool{}
	for _, e := range notInImage(x.cfg.Env, x.imageCfg().Env) {
		k, _, hasValue := strings.Cut(e, "=")
		if !hasValue {
			x.warn("--env=%s has no value: docker run copies %s from the caller's environment when it is set there", k, k)
		}
		if seen[k] {
			x.warn("env %s is set more than once; the last value wins", k)
		}
		seen[k] = true
		a = append(a, "--env="+e)
	}
	// The daemon copies the image's WORKDIR when -w is not given.
	if w := x.cfg.WorkingDir; w != "" && w != x.imageCfg().WorkingDir {
		a = append(a, "--workdir="+w)
	}
	a = append(a, x.attachArgs()...)
	if x.cfg.OpenStdin {
		a = append(a, "--interactive")
	}
	if x.cfg.Tty {
		a = append(a, "--tty")
	}
	// The CLI stores Init only when --init is given, so false is explicit.
	if i := x.hc.Init; i != nil {
		a = append(a, map[bool]string{true: "--init", false: "--init=false"}[*i])
	}
	// The daemon copies the image's STOPSIGNAL when --stop-signal is not given.
	if s := x.cfg.StopSignal; s != "" && s != x.imageCfg().StopSignal {
		a = append(a, "--stop-signal="+s)
	}
	if t := x.cfg.StopTimeout; t != nil {
		a = append(a, "--stop-timeout="+strconv.Itoa(*t))
	}
	if u := x.hc.Umask; u != nil {
		a = append(a, fmt.Sprintf("--umask=%04o", *u)) // stored as a number: 0027 is 23
	}
	return a
}

// attachArgs inverts the CLI's attach logic: no -a attaches stdout and
// stderr, -i adds stdin, -d clears all three.
func (x *ctx) attachArgs() []string {
	in, out, err := x.cfg.AttachStdin, x.cfg.AttachStdout, x.cfg.AttachStderr
	if !in && !out && !err {
		return []string{"--detach"}
	}
	if out && err && in == x.cfg.OpenStdin {
		return nil
	}
	var a []string
	for _, s := range []struct {
		on   bool
		name string
	}{{in, "stdin"}, {out, "stdout"}, {err, "stderr"}} {
		if s.on {
			a = append(a, "--attach="+s.name)
		}
	}
	return a
}

// entrypoint returns the --entrypoint value when one must be printed.
// docker run --entrypoint takes one string, and setting it drops the
// image's CMD, so command() moves the remaining entrypoint elements and
// the full Cmd after the image.
func (x *ctx) entrypoint() (string, bool) {
	if !x.overrideEntrypoint() {
		return "", false
	}
	if len(x.cfg.Entrypoint) == 0 {
		return "", true // the image has one; "--entrypoint=" resets it
	}
	return x.cfg.Entrypoint[0], true
}

func (x *ctx) overrideEntrypoint() bool {
	if x.icfg == nil {
		return len(x.cfg.Entrypoint) > 0
	}
	if !slices.Equal(x.cfg.Entrypoint, x.icfg.Entrypoint) {
		return true
	}
	// Same entrypoint, but CMD cleared: only an --entrypoint flag does that.
	return len(x.cfg.Cmd) == 0 && len(x.icfg.Cmd) > 0
}

// command returns the words after the image.
func (x *ctx) command() []string {
	if x.overrideEntrypoint() {
		var words []string
		if len(x.cfg.Entrypoint) > 1 {
			words = append(words, x.cfg.Entrypoint[1:]...)
		}
		return append(words, x.cfg.Cmd...)
	}
	if x.icfg != nil && slices.Equal(x.cfg.Cmd, x.icfg.Cmd) {
		return nil
	}
	return x.cfg.Cmd
}
