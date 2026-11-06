package runlike

import (
	"slices"
	"strconv"
	"strings"
	"time"

	dockerspec "github.com/moby/docker-image-spec/specs-go/v1"
)

// healthArgs: the health check, where it differs from the image's. The
// daemon fills each unset field from the image's HEALTHCHECK, so a field
// equal to the image's needs no flag.
func healthArgs(x *ctx) []string {
	var img dockerspec.HealthcheckConfig
	if ih := x.imageCfg().Healthcheck; ih != nil {
		img = *ih
	}
	none := func(t []string) bool { return len(t) > 0 && t[0] == "NONE" }
	h := x.cfg.Healthcheck
	if h == nil {
		// The daemon copies the image's check on create, so none here and
		// one in the image means the image changed (its tag moved).
		if len(img.Test) > 0 && !none(img.Test) {
			return []string{"--no-healthcheck"}
		}
		return nil
	}
	// --no-healthcheck conflicts with --health-*; the daemon refills the
	// other fields from the image. When the image says NONE too, only the
	// timing fields can differ: they follow.
	if none(h.Test) && !none(img.Test) {
		return []string{"--no-healthcheck"}
	}
	var a []string
	if len(h.Test) > 1 && !slices.Equal(h.Test, img.Test) {
		cmd := h.Test[1]
		if h.Test[0] == "CMD" {
			// ponytail: exec form has no flag; the shell form of the same
			// words is the closest (Test becomes CMD-SHELL on recreate).
			q := make([]string, len(h.Test)-1)
			for i, w := range h.Test[1:] {
				q[i], _ = quotePosix(w) // posix quoting never fails
			}
			cmd = strings.Join(q, " ")
			// A Windows container runs CMD-SHELL with cmd /S /C, which has
			// no quoting that keeps every word intact (its metacharacters
			// and quote stripping apply), so no form is exact there.
			if x.OS == "windows" {
				x.warn("health check %q is exec form, which docker run cannot set: printed as a shell command with POSIX quoting, but this Windows container runs it with cmd /S /C; check the quoting", h.Test[1:])
			}
		}
		a = append(a, "--health-cmd="+cmd)
	}
	for _, d := range []struct {
		flag     string
		got, img time.Duration
	}{
		{"interval", h.Interval, img.Interval},
		{"timeout", h.Timeout, img.Timeout},
		{"start-period", h.StartPeriod, img.StartPeriod},
		{"start-interval", h.StartInterval, img.StartInterval},
	} {
		if d.got != 0 && d.got != d.img {
			a = append(a, "--health-"+d.flag+"="+d.got.String())
		}
	}
	if h.Retries != 0 && h.Retries != img.Retries {
		a = append(a, "--health-retries="+strconv.Itoa(h.Retries))
	}
	return a
}
