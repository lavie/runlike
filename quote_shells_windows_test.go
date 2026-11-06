package runlike

// Live checks of the PowerShell and cmd dialects in the real shells. They
// run only on Windows (this file's name is a GOOS constraint); build the
// test binary elsewhere with
//
//	GOOS=windows GOARCH=amd64 go test -c -o runlike.test.exe .
//
// and run it on the Windows machine (test/roundtrip/README.md). The test
// binary doubles as a fake docker.exe that records its arguments.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// When RUNLIKE_ARGV_OUT is set, the binary is the fake docker: it appends
// its arguments as one JSON line to that file and exits.
func init() {
	out := os.Getenv("RUNLIKE_ARGV_OUT")
	if out == "" {
		return
	}
	f, err := os.OpenFile(out, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err == nil {
		b, _ := json.Marshal(append([]string{"docker"}, os.Args[1:]...))
		_, err = f.Write(append(b, '\n'))
		f.Close()
	}
	if err != nil {
		os.Exit(3)
	}
	os.Exit(0)
}

// winShell runs one script (the lines joined) in one shell and dir as the
// working directory, with PATH starting at dir.
type winShell struct {
	name string
	run  func(t *testing.T, dir string, env []string, lines []string) // lines run in order; failures of one line must not stop the rest
	// skip names lines this way of running cannot take: cmd /c takes one
	// line; cmd reading a pipe decodes it in the console code page, unlike
	// typed or pasted text, so it gets ASCII lines only.
	skip func(line string) bool
}

func psRun(exe string) func(*testing.T, string, []string, []string) {
	return func(t *testing.T, dir string, env []string, lines []string) {
		var b strings.Builder
		for _, l := range lines {
			// try/catch: the 5.1 guard throws, and the next line must still run.
			fmt.Fprintf(&b, "try {\n%s\n} catch { [Console]::Error.WriteLine($_) }\n", l)
		}
		p := filepath.Join(dir, "lines.ps1")
		if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		c := exec.Command(exe, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", p)
		c.Dir, c.Env = dir, env
		if out, err := c.CombinedOutput(); err != nil {
			t.Logf("%s: %v: %s", exe, err, out)
		}
	}
}

// cmdC runs each line as cmd.exe /d /s /c "LINE": /s strips the outer
// quotes and keeps the rest as typed.
func cmdC(t *testing.T, dir string, env []string, lines []string) {
	for _, l := range lines {
		c := exec.Command("cmd.exe")
		c.SysProcAttr = &syscall.SysProcAttr{CmdLine: `cmd.exe /d /s /c "` + l + `"`}
		c.Dir, c.Env = dir, env
		c.Run() // the recorded argv is the result
	}
}

// cmdPrompt feeds the lines to one interactive cmd.exe on stdin, as typed
// at the prompt (pretty lines continue with ^ at the end of a line).
func cmdPrompt(t *testing.T, dir string, env []string, lines []string) {
	c := exec.Command("cmd.exe", "/d", "/q", "/k", "chcp 65001 >nul")
	c.Dir, c.Env = dir, env
	in := strings.Join(append(lines, "exit", ""), "\n")
	c.Stdin = strings.NewReader(strings.ReplaceAll(in, "\n", "\r\n"))
	if out, err := c.CombinedOutput(); err != nil {
		t.Logf("cmd prompt: %v: %s", err, out)
	}
}

func winShells(t *testing.T, sh Shell) []winShell {
	var out []winShell
	switch sh {
	case PowerShell:
		for _, exe := range []string{"pwsh.exe", "powershell.exe"} {
			if p, err := exec.LookPath(exe); err == nil {
				out = append(out, winShell{exe, psRun(p), nil})
			} else {
				t.Errorf("%s not found", exe)
			}
		}
	case Cmd:
		out = append(out,
			winShell{"cmd /c", cmdC, func(l string) bool { return strings.Contains(l, "\n") }},
			winShell{"cmd prompt", cmdPrompt, func(l string) bool { return !ascii(l) }})
	}
	return out
}

// hostile pieces for random values: every PowerShell and cmd
// metacharacter, quote look-alikes PowerShell treats as quotes (‘ ’ “ ”)
// and dashes it treats as - (– —), backslash runs, non-ASCII, controls.
var hostile = []string{"'", `"`, "`", "$", "$x", "${x}", "$(calc)", "%", "%PATH%", "%x", "^", "&", "|", "<", ">", "(", ")", "!", "!PATH!",
	";", ",", " ", "  ", "\t", "\n", "\r\n", `\`, `\\`, `\"`, "@", "@(", "#", "{", "}", "[", "]", "=", ":", "*", "?", "~", "-", "--", "--%",
	"\u2018", "\u2019", "\u201c", "\u201d", "\u2013", "\u2014", "é", "ち", "😀", "\u00a0", "\x1b", "\x7f", "a", "B", "0", "C:", ".", "/c", "&&", "||", "2>"}

// randValue joins random hostile pieces. tame values are what Windows
// PowerShell 5.1 runs unguarded: printable ASCII, no double quote, no
// trailing backslash.
func randValue(r *rand.Rand, tame bool) string {
	var b strings.Builder
	for n := r.IntN(6); n >= 0; {
		p := hostile[r.IntN(len(hostile))]
		if tame && (!printable(p) || strings.Contains(p, `"`)) {
			continue
		}
		b.WriteString(p)
		n--
	}
	if tame && (b.Len() == 0 || strings.HasSuffix(b.String(), `\`)) {
		b.WriteString("a")
	}
	return b.String()
}

// TestWindowsShellsFuzz formats random hostile arguments for each dialect
// and runs every line in the real shells, with docker a recorder: the
// recorded argv must equal the Command's. Windows PowerShell 5.1 may
// instead refuse a guarded line, but must never run docker with other
// arguments. RUNLIKE_SHELL_N sets the number of commands (default 200),
// RUNLIKE_SHELL_SEED the seed.
func TestWindowsShellsFuzz(t *testing.T) {
	n, _ := strconv.Atoi(os.Getenv("RUNLIKE_SHELL_N"))
	if n == 0 {
		n = 200
	}
	seed, _ := strconv.ParseUint(os.Getenv("RUNLIKE_SHELL_SEED"), 10, 64)
	if seed == 0 {
		seed = rand.Uint64()
	}
	t.Logf("RUNLIKE_SHELL_SEED=%d RUNLIKE_SHELL_N=%d", seed, n)
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, sh := range []Shell{PowerShell, Cmd} {
		t.Run(string(sh), func(t *testing.T) {
			r := rand.New(rand.NewPCG(seed, 1))
			var cmds []Command
			var lines []string
			// Each piece alone first, as an option value and as a whole argument.
			for _, p := range hostile {
				c := Command{Options: []string{"--env=A=" + p}, Image: "img" + strconv.Itoa(len(cmds)), Args: []string{p, p + p}}
				if line, err := Format(c, FormatOptions{Shell: sh}); err == nil {
					cmds, lines = append(cmds, c), append(lines, line)
				}
			}
			for i := 0; len(cmds) < n; i++ {
				tame := r.IntN(2) == 0
				v := func() string { return randValue(r, tame) }
				c := Command{Options: []string{"--env=A=" + v(), "--label=" + v(), "-" + v()},
					Image: "img" + strconv.Itoa(len(cmds)), Args: []string{v(), v()}}
				line, err := Format(c, FormatOptions{Shell: sh, Pretty: r.IntN(3) == 0})
				if err != nil {
					continue // cmd refuses control characters, by design
				}
				cmds, lines = append(cmds, c), append(lines, line)
			}
			for _, s := range winShells(t, sh) {
				t.Run(s.name, func(t *testing.T) {
					dir := t.TempDir()
					if err := copyFile(self, filepath.Join(dir, "docker.exe")); err != nil {
						t.Fatal(err)
					}
					out := filepath.Join(dir, "argv.jsonl")
					env := append(os.Environ(), "RUNLIKE_ARGV_OUT="+out, "PATH="+dir+";"+os.Getenv("PATH"))
					var run []string
					for _, l := range lines {
						if s.skip == nil || !s.skip(l) {
							run = append(run, l)
						}
					}
					s.run(t, dir, env, run)
					got := map[string][]string{}
					f, err := os.Open(out)
					if os.IsNotExist(err) {
						f, err = os.Open(os.DevNull) // no docker call at all
					}
					if err != nil {
						t.Fatal(err)
					}
					defer f.Close()
					sc := bufio.NewScanner(f)
					sc.Buffer(nil, 1<<20)
					for sc.Scan() {
						var argv []string
						if err := json.Unmarshal(sc.Bytes(), &argv); err != nil {
							t.Fatal(err)
						}
						i := slices.IndexFunc(argv, func(a string) bool { return strings.HasPrefix(a, "img") })
						key := ""
						if i >= 0 {
							key = argv[i]
						}
						if _, dup := got[key]; dup || i < 0 {
							t.Errorf("unexpected docker call %q", argv)
						}
						got[key] = argv
					}
					refused := 0
					for i, c := range cmds {
						if s.skip != nil && s.skip(lines[i]) {
							continue
						}
						g, ok := got[c.Image]
						guarded := strings.HasPrefix(lines[i], psGuard)
						switch {
						case !ok && s.name == "powershell.exe" && guarded:
							refused++
						case !ok:
							t.Errorf("line not run: %q", lines[i])
						case !slices.Equal(g, c.Argv()):
							t.Errorf("line %q\n got %q\nwant %q", lines[i], g, c.Argv())
						case s.name == "powershell.exe" && guarded:
							t.Errorf("5.1 ran a guarded line: %q", lines[i])
						}
					}
					t.Logf("%d lines run, %d refused by the 5.1 guard", len(run), refused)
				})
			}
		})
	}
}

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o755)
}

// TestWindowsShellsDocker is a live round trip in each shell: create
// container A with hostile values, run runlike.exe --shell X on it (and on
// its inspect JSON with --stdin), run the printed line in shell X with
// "docker run" made "docker create", compare A's and B's settings. Needs
// RUNLIKE_EXE (path to runlike.exe) and a local Windows Docker daemon.
func TestWindowsShellsDocker(t *testing.T) {
	exe := os.Getenv("RUNLIKE_EXE")
	if exe == "" {
		t.Skip("set RUNLIKE_EXE to runlike.exe for live shell round trips")
	}
	const img = "mcr.microsoft.com/windows/nanoserver:ltsc2022"
	hostileVals := []string{`it's "quoted" $HOME ${x} $(calc) ` + "`n" + ` %PATH% ^ & | < > ( ) ! ; , @ # {}`,
		"line1\nline2", `C:\trailing\`, `\\server\share\"x"\\`, "ち é 😀 \u2019\u201c\u2013", "", "--%", "-a.b:c"}
	tame := []string{`plain $x %PATH% ^ & | < > ( ) ! ; @ # {} 'single'`, `C:\Program Files\x y`, "--%"}
	type tc struct {
		sh    Shell
		shell string
		vals  []string
		stdin bool
	}
	var cases []tc
	for _, stdin := range []bool{false, true} {
		cases = append(cases,
			tc{PowerShell, "pwsh.exe", hostileVals, stdin},
			tc{PowerShell, "powershell.exe", tame, stdin},
			tc{Cmd, "cmd /c", slices.DeleteFunc(slices.Clone(hostileVals), func(s string) bool { return strings.Contains(s, "\n") }), stdin},
			tc{Cmd, "cmd prompt", tame, stdin})
	}
	run := func(t *testing.T, args ...string) string {
		t.Helper()
		out, err := exec.Command(args[0], args[1:]...).Output()
		if err != nil {
			t.Fatalf("%q: %v", args, err)
		}
		return strings.TrimSpace(string(out))
	}
	for i, c := range cases {
		t.Run(fmt.Sprintf("%s/stdin=%v", c.shell, c.stdin), func(t *testing.T) {
			a := fmt.Sprintf("win-shell-%d-a", i)
			b := fmt.Sprintf("win-shell-%d-b", i)
			exec.Command("docker", "rm", "-f", a, b).Run()
			t.Cleanup(func() { exec.Command("docker", "rm", "-f", a, b).Run() })
			args := []string{"create", "--name=" + a, "--workdir=C:\\Program Files\\w x"}
			for j, v := range c.vals {
				args = append(args, fmt.Sprintf("--env=E%d=%s", j, v), fmt.Sprintf("--label=l%d=%s", j, v))
			}
			args = append(append(args, img, "cmd", "/c"), c.vals...)
			run(t, append([]string{"docker"}, args...)...)
			var line string
			if c.stdin {
				js := run(t, "docker", "container", "inspect", a)
				cmd := exec.Command(exe, "--stdin", "--shell", string(c.sh))
				cmd.Stdin = strings.NewReader(js)
				out, err := cmd.Output()
				if err != nil {
					t.Fatal(err)
				}
				line = strings.TrimSpace(string(out))
			} else {
				line = run(t, exe, "--shell", string(c.sh), a)
			}
			t.Logf("runlike: %s", line)
			if !strings.Contains(line, "docker run --name="+a+" ") {
				t.Fatalf("unexpected line %q", line)
			}
			line = strings.Replace(line, "docker run --name="+a+" ", "docker create --name="+b+" ", 1)
			dir := t.TempDir()
			for _, s := range winShells(t, c.sh) {
				if s.name == c.shell {
					s.run(t, dir, os.Environ(), []string{line})
				}
			}
			field := func(name, f string) string {
				out, err := exec.Command("docker", "container", "inspect", "-f", f, name).Output()
				if err != nil {
					t.Fatalf("container %s: %v (line %q)", name, err, line)
				}
				if strings.HasPrefix(f, "{{range") {
					lines := strings.Split(string(out), "\n")
					slices.Sort(lines)
					return strings.Join(lines, "\n")
				}
				return string(out)
			}
			// The daemon returns Env in random order for the same argv.
			for _, f := range []string{"{{range .Config.Env}}{{println .}}{{end}}", "{{json .Config.Labels}}", "{{json .Config.Cmd}}", "{{json .Config.WorkingDir}}", "{{json .Config.Entrypoint}}"} {
				if fa, fb := field(a, f), field(b, f); fa != fb {
					t.Errorf("%s differs:\n A %s\n B %s", f, fa, fb)
				}
			}
		})
	}
}
