package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// With RUNLIKE_FAKE_DOCKER set to the testdata directory, the test binary
// is a fake docker: container "web" (and "old", a stopped container on a
// daemon older than API 1.51) exist, anything else is missing.
func TestMain(m *testing.M) {
	if td := os.Getenv("RUNLIKE_FAKE_DOCKER"); td != "" {
		os.Exit(fakeDocker(td, os.Args[1:]))
	}
	os.Exit(m.Run())
}

func fakeDocker(td string, args []string) int {
	if os.Getenv("DOCKER_API_VERSION") == "1.51" && len(args) > 3 && args[3] == "old" {
		fmt.Fprintln(os.Stderr, "Error response from daemon: client version 1.51 is too new. Maximum supported API version is 1.41")
		return 1
	}
	arg := func(i int) string {
		if i < len(args) {
			return args[i]
		}
		return ""
	}
	cat := func(f string) int {
		b, err := os.ReadFile(filepath.Join(td, f))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		os.Stdout.Write(b)
		return 0
	}
	switch a, d := arg(0), arg(3); {
	case a == "container" && d == "web":
		return cat("env.container.json")
	case a == "container" && d == "old":
		return cat("plain.container.json")
	case a == "version":
		fmt.Println("linux/arm64")
	case a == "info":
		fmt.Println(`{"Name":"h","LoggingDriver":"json-file","DefaultRuntime":"runc","CgroupVersion":"2"}`)
	case a == "image":
		return cat("skel_img.image.json")
	default:
		fmt.Fprintln(os.Stderr, "Error response from daemon: No such container: "+d)
		return 1
	}
	return 0
}

func TestCLI(t *testing.T) {
	// Fake docker: this test binary, copied into PATH as docker (see
	// TestMain), on every OS.
	dir := t.TempDir()
	td, _ := filepath.Abs("../../testdata")
	exe, _ := os.Executable()
	b, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	name := "docker"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if err := os.WriteFile(filepath.Join(dir, name), b, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RUNLIKE_FAKE_DOCKER", td)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	stdinJSON, _ := os.ReadFile(td + "/plain.container.json")

	const web = `docker run --name=runlike_skel_env '--env=FOO=a b' '--env=Q=it'\''s $HOME;` + "`x`" + `' --env=IMG_VAR=override --detach --log-driver=local '--label=x=y z' runlike_skel_img`
	for _, tc := range []struct {
		argv     string
		code     int
		out, err string // substrings
	}{
		{"web", 0, web, ""},
		{"web --no-name", 0, "docker run '--env=FOO", ""},
		{"--no-name web", 0, "docker run '--env=FOO", ""},
		{"-l web", 0, "--detach --log-driver=local runlike_skel_img", ""},
		{"web --no-labels", 0, "--detach --log-driver=local runlike_skel_img", ""},
		{"-pl web", 0, "docker run \\\n\t--name=runlike_skel_env \\\n", ""},
		{"web -lp --use-volume-id", 0, "\\\n\t--log-driver=local \\\n\trunlike_skel_img\n", ""},
		{"-s", 0, "--entrypoint=/bin/sh", ""},
		{"--stdin --no-name", 0, "docker run --entrypoint=/bin/sh", ""},
		{"-sp", 0, "\\\n\t-c 'sleep 1000'\n", ""},
		{"web --shell posix", 0, web, ""},
		{"web --shell powershell", 0, "docker run --name=runlike_skel_env '--env=FOO=a b' ", ""},
		{"web --shell cmd", 0, `docker run --name=runlike_skel_env ^"--env=FOO=a b^" `, ""},
		{"web --shell fish", 1, "", "unknown shell"},
		{"--pretty=true web", 2, "", "Error: Option '--pretty' does not take a value."},
		{"--no-name=false web", 2, "", "Error: Option '--no-name' does not take a value."},
		{"--stdin=1", 2, "", "Error: Option '--stdin' does not take a value."},
		{"--no-labels= web", 2, "", "Error: Option '--no-labels' does not take a value."},
		{"-p=1 web", 2, "", "Error: No such option: -="},
		{"-sp=1", 2, "", "Error: No such option: -="},
		{"--shell=posix web", 0, web, ""},
		{"web -- --pretty=1", 2, "", "unexpected extra argument"},
		{"gone", 1, "", "No such container gone\n"},
		{"old", 0, "docker run --name=", "warning: whether a MAC address was pinned (--mac-address) cannot be read from this daemon"},
		{"", 2, "", "Error: usage error"},
		{"a b", 2, "", "unexpected extra argument (b)"},
		{"--bogus web", 2, "", "Error:"},
		{"-h", 2, "", "Error:"},
		{"--help", 0, "--no-labels", ""},
	} {
		var out, errOut bytes.Buffer
		code := run(strings.Fields(tc.argv), bytes.NewReader(stdinJSON), &out, &errOut)
		if code != tc.code || tc.code != 0 && out.Len() > 0 || !strings.Contains(out.String(), tc.out) || !strings.Contains(errOut.String(), tc.err) {
			t.Errorf("runlike %s: code %d, want %d\nstdout: %q\nstderr: %q", tc.argv, code, tc.code, out.String(), errOut.String())
		}
	}

	// Warnings quote container data: escape sequences and bidi overrides
	// reach the terminal escaped.
	var out, errOut bytes.Buffer
	evil := `[{"Config":{"Image":"alpine","Env":["\u001b[2J\u001b[HBARE","\u202eEVIL","\u009b31m"]},"HostConfig":{}}]`
	if code := run([]string{"-s"}, strings.NewReader(evil), &out, &errOut); code != 0 {
		t.Fatalf("hostile env: code %d, stderr %q", code, errOut.String())
	}
	if e := errOut.String(); strings.ContainsAny(e, "\x1b\u202e\u009b") || !strings.Contains(e, `--env=\x1b[2J\x1b[HBARE has no value`) ||
		!strings.Contains(e, `--env=\u202eEVIL`) || !strings.Contains(e, `--env=\u009b31m`) {
		t.Errorf("stderr not escaped: %q", e)
	}
	if strings.ContainsAny(out.String(), "\x1b\u202e\u009b") {
		t.Errorf("stdout: %q", out.String())
	}

	// A Config.Image that docker run would read as an option.
	out.Reset()
	errOut.Reset()
	crafted := `[{"Config":{"Image":"-v","Cmd":["/:/host","alpine","sh","-c","cat /host/etc/shadow"]},"HostConfig":{}}]`
	if code := run([]string{"-s"}, strings.NewReader(crafted), &out, &errOut); code != 1 || out.Len() > 0 || !strings.Contains(errOut.String(), "not an image reference") {
		t.Errorf("image -v: code %d, stdout %q, stderr %q", code, out.String(), errOut.String())
	}

	// runlike --stdin reads inspect data of Docker 20.10, 24.0 and 26.1.
	old, _ := filepath.Glob(td + "/olddaemons/*/*.container.json")
	for _, f := range old {
		b, _ := os.ReadFile(f)
		out.Reset()
		errOut.Reset()
		if code := run([]string{"-s"}, bytes.NewReader(b), &out, &errOut); code != 0 || !strings.HasPrefix(out.String(), "docker run --name=") {
			t.Errorf("runlike -s < %s: code %d, stdout %q, stderr %q", f, code, out.String(), errOut.String())
		}
	}
	if len(old) == 0 {
		t.Error("no old-daemon data")
	}

	// Input without Config.Image is not container inspect data.
	out.Reset()
	errOut.Reset()
	if code := run([]string{"-s"}, strings.NewReader("{}"), &out, &errOut); code != 1 || out.Len() > 0 || !strings.Contains(errOut.String(), "Config.Image") {
		t.Errorf("echo {} | runlike -s: code %d, stdout %q, stderr %q", code, out.String(), errOut.String())
	}
}
