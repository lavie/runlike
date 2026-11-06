package roundtrip

// Process area: environment, entrypoint, command, -i/-t/-d.
var _ = add("process",
	// env
	Case{Name: "env-plain", Args: []string{"-e", "A=1", "-e", "B=two words", alpine, "true"}},
	Case{Name: "env-quotes", Args: []string{"-e", `Q=it's "quoted" \back\slash`, alpine, "true"}},
	Case{Name: "env-shell-chars", Args: []string{"-e", "D=$HOME ${X} $(id) `id` ; | & > < * ? ~ # !", alpine, "true"}},
	Case{Name: "env-newline", Args: []string{"-e", "N=line1\nline2\n", "-e", "T=tab\there", alpine, "true"}},
	Case{Name: "env-utf8", Args: []string{"-e", "U=שלום ünïcödé 日本 🐳", alpine, "true"}},
	Case{Name: "env-edge", Args: []string{"-e", "EMPTY=", "-e", "EQ=a=b=c", "-e", "DASH=-x", "-e", "LEAD==x", alpine, "true"}},
	Case{Name: "env-image-same", Args: []string{fixture}},
	Case{Name: "env-image-override", Args: []string{"-e", "IMG_ENV=changed", "-e", "PATH=/bin", fixture}},
	// entrypoint and command
	Case{Name: "cmd-args", Args: []string{alpine, "echo", "a b", "", "'c'", "-la", "--x=y"}},
	Case{Name: "cmd-image-same", Args: []string{fixture, "echo img"}},
	Case{Name: "cmd-image-override", Args: []string{fixture, "echo $HOME; exit 3"}},
	Case{Name: "entrypoint-single", Args: []string{"--entrypoint=/bin/echo", alpine, "hi", "there"}},
	Case{Name: "entrypoint-no-cmd", Args: []string{"--entrypoint=/bin/true", alpine}},
	Case{Name: "entrypoint-image-multi", Args: []string{fixture}},
	Case{Name: "entrypoint-override-multi", Args: []string{"--entrypoint=/bin/sh", fixture, "-c", "echo x"}},
	Case{Name: "entrypoint-same-as-image-first", Args: []string{"--entrypoint=/bin/sh", fixture}},
	Case{Name: "entrypoint-reset-with-cmd", Args: []string{"--entrypoint=", fixture, "echo", "hi"}},
	Case{Name: "entrypoint-spaces", Args: []string{"--entrypoint=/bin/echo with space", alpine}},
	// -i / -t / -d (create cannot detach; -d cases start the container)
	Case{Name: "attach-default", Args: []string{alpine, "true"}},
	Case{Name: "attach-i", Args: []string{"-i", alpine, "sh"}},
	Case{Name: "attach-t", Args: []string{"-t", alpine, "sh"}},
	Case{Name: "attach-it", Args: []string{"-it", alpine, "sh"}},
	Case{Name: "detach", Run: true, Args: []string{alpine, "true"}},
	Case{Name: "detach-i", Run: true, Args: []string{"-i", alpine, "sh"}},
	Case{Name: "detach-t", Run: true, Args: []string{"-t", alpine, "sleep", "300"}},
	Case{Name: "detach-it", Run: true, Args: []string{"-it", alpine, "sh"}},
)

// imgWith returns Setup/Teardown that commit an alpine image {p}-img with
// the given Dockerfile changes (STOPSIGNAL, WORKDIR, HEALTHCHECK, ...).
func imgWith(changes ...string) (setup, teardown [][]string) {
	commit := []string{"commit"}
	for _, c := range changes {
		commit = append(commit, "--change", c)
	}
	return [][]string{{"create", "--name", "{p}-src", alpine}, append(commit, "{p}-src", "{p}-img")},
		[][]string{{"rm", "-f", "{p}-src"}, {"rmi", "{p}-img"}}
}

var procImgSetup, procImgTeardown = imgWith("STOPSIGNAL SIGQUIT", "WORKDIR /srv")

var _ = add("process",
	// -a: inverse of "no -a = stdout+stderr, -i adds stdin, -d clears all"
	Case{Name: "attach-stdout", Args: []string{"-a", "stdout", alpine, "true"}},
	Case{Name: "attach-stderr", Args: []string{"-a", "stderr", alpine, "true"}},
	Case{Name: "attach-stdin-only", Args: []string{"-a", "stdin", alpine, "true"}},
	Case{Name: "attach-all-no-i", Args: []string{"-a", "stdin", "-a", "stdout", "-a", "stderr", alpine, "true"}},
	Case{Name: "attach-i-stdin", Args: []string{"-i", "-a", "stdin", alpine, "sh"}},
	Case{Name: "attach-i-stdout", Args: []string{"-i", "-a", "stdout", alpine, "sh"}},
	Case{Name: "attach-it-stderr", Args: []string{"-it", "-a", "stderr", alpine, "sh"}},
	Case{Name: "attach-upper", Args: []string{"-a", "STDOUT", alpine, "true"}},
	// --env-file folds into --env
	Case{Name: "env-file", Args: []string{"--env-file", "testdata/process.env", "-e", "EF_X=flag", alpine, "true"}},
	// -e NAME with NAME unset on the client is stored as a bare "NAME"
	Case{Name: "env-bare-name", Args: []string{"-e", "RL_RT_SURELY_UNSET", alpine, "true"}},
	// workdir
	Case{Name: "workdir", Args: []string{"-w", "/tmp", alpine, "true"}},
	Case{Name: "workdir-space", Args: []string{"--workdir=/a b/'c'", alpine, "true"}},
	Case{Name: "workdir-image-same", Setup: procImgSetup, Teardown: procImgTeardown, Args: []string{"-w", "/srv", "{p}-img", "true"}},
	Case{Name: "workdir-image-override", Setup: procImgSetup, Teardown: procImgTeardown, Args: []string{"-w", "/tmp", "{p}-img", "true"}},
	// --init (stored only when given, so false is explicit)
	Case{Name: "init", Args: []string{"--init", alpine, "true"}},
	Case{Name: "init-false", Args: []string{"--init=false", alpine, "true"}},
	// stop signal / timeout
	Case{Name: "stop-signal", Args: []string{"--stop-signal=SIGUSR1", alpine, "true"}},
	Case{Name: "stop-signal-number", Args: []string{"--stop-signal=9", alpine, "true"}},
	Case{Name: "stop-signal-short", Args: []string{"--stop-signal=HUP", alpine, "true"}},
	Case{Name: "stop-signal-image-same", Setup: procImgSetup, Teardown: procImgTeardown, Args: []string{"--stop-signal=SIGQUIT", "{p}-img", "true"}},
	Case{Name: "stop-signal-image-override", Setup: procImgSetup, Teardown: procImgTeardown, Args: []string{"--stop-signal=SIGKILL", "{p}-img", "true"}},
	Case{Name: "stop-timeout", Args: []string{"--stop-timeout=30", alpine, "true"}},
	Case{Name: "stop-timeout-zero", Args: []string{"--stop-timeout=0", alpine, "true"}},
	Case{Name: "stop-timeout-neg", Args: []string{"--stop-timeout=-1", alpine, "true"}},
	// umask: stored as a number, printed in octal
	Case{Name: "umask", Args: []string{"--umask=0027", alpine, "true"}},
	Case{Name: "umask-zero", Args: []string{"--umask=0000", alpine, "true"}},
	Case{Name: "umask-max", Args: []string{"--umask=0777", alpine, "true"}},
	Case{Name: "umask-short", Args: []string{"--umask=22", alpine, "true"}},
	// all together, started
	Case{Name: "combined-run", Run: true, Args: []string{"-i", "--init", "-w", "/tmp", "--stop-signal=SIGINT", "--stop-timeout=5", "--umask=0077",
		"-e", "X=1", "--entrypoint=/bin/sh", alpine, "-c", "sleep 300"}},
)
