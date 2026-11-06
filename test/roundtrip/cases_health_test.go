package roundtrip

var healthImgSetup, healthImgTeardown = imgWith(`HEALTHCHECK --interval=30s --retries=4 CMD ["/bin/true", "x"]`)

// Health area: --health-*, --no-healthcheck.
var _ = add("health",
	Case{Name: "none-default", Args: []string{alpine, "true"}},
	Case{Name: "cmd", Args: []string{"--health-cmd=true", alpine, "true"}},
	Case{Name: "cmd-quoting", Args: []string{"--health-cmd=wget -q -O- 'http://x/a b' || exit \"1\"; echo $HOME `id`", alpine, "true"}},
	Case{Name: "all-fields", Args: []string{"--health-cmd=true", "--health-interval=1m30s", "--health-timeout=500ms",
		"--health-start-period=1h", "--health-start-interval=2s", "--health-retries=7", alpine, "true"}},
	Case{Name: "odd-durations", Args: []string{"--health-interval=1.5s", "--health-timeout=90m", "--health-start-period=1ms", "--health-start-interval=3h2m1s", alpine, "true"}},
	Case{Name: "no-cmd", Args: []string{"--health-retries=2", alpine, "true"}},
	Case{Name: "no-healthcheck", Args: []string{"--no-healthcheck", alpine, "true"}},
	Case{Name: "image-same", Setup: healthImgSetup, Teardown: healthImgTeardown, Args: []string{"{p}-img", "true"}},
	Case{Name: "image-same-explicit", Setup: healthImgSetup, Teardown: healthImgTeardown, Args: []string{"--health-interval=30s", "--health-retries=4", "{p}-img", "true"}},
	Case{Name: "image-override-cmd", Setup: healthImgSetup, Teardown: healthImgTeardown, Args: []string{"--health-cmd=exit 0", "{p}-img", "true"}},
	Case{Name: "image-override-timing", Setup: healthImgSetup, Teardown: healthImgTeardown, Args: []string{"--health-timeout=5s", "--health-retries=1", "{p}-img", "true"}},
	Case{Name: "image-no-healthcheck", Setup: healthImgSetup, Teardown: healthImgTeardown, Args: []string{"--no-healthcheck", "{p}-img", "true"}},
)

// Exec-form (CMD) health check without image data (--stdin): no flag sets
// the exec form, so runlike prints the shell form of the same words.
// Saved from Docker 29.9.3 (Lima, Linux arm64): docker create of an
// image with HEALTHCHECK CMD ["/bin/echo","a b"].
var _ = addSaved(SavedCase{Name: "health-exec-stdin", Want: []string{"--health-cmd=/bin/echo 'a b'", "--health-interval=30s", "--health-retries=4"}})
