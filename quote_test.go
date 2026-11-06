package runlike

import (
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

func cmdOf(argv ...string) Command {
	i := slices.IndexFunc(argv, func(s string) bool { return !strings.HasPrefix(s, "-") })
	return Command{Options: argv[:i], Image: argv[i], Args: argv[i+1:]}
}

func TestFormat(t *testing.T) {
	const pf = `"$(printf '`
	for _, tc := range []struct {
		sh     Shell
		pretty bool
		argv   []string // options..., image, args...
		want   string
	}{
		{Posix, false, []string{"img"}, "docker run img"},
		{"", false, []string{"--env=A=b c", "img", "sh", "-c", "it's"}, `docker run '--env=A=b c' img sh -c 'it'\''s'`},
		{Posix, true, []string{"--name=x", "--detach", "img", "a", ""}, "docker run \\\n\t--name=x \\\n\t--detach \\\n\timg \\\n\ta ''"},
		{Posix, true, []string{"img"}, "docker run \\\n\timg"},
		{Posix, false, []string{"img", "=ls", "~", "*"}, "docker run img '=ls' '~' '*'"},
		// zsh magic_equal_subst
		{Posix, false, []string{"img", "a==ls", "a:=ls", "a=b:c"}, "docker run img 'a==ls' 'a:=ls' a=b:c"},
		// EUC lead bytes never sit before a quote: all non-ASCII goes through printf.
		{Posix, false, []string{"--label=a=ち", "img"}, `(LC_ALL=C; docker run '--label=a='` + pf + `\343\201\241')" img)`},
		{Posix, false, []string{"img", "\xa1'", "\xa1'\\''\xa1"}, `(LC_ALL=C; docker run img ` + pf + `\241')"''\''' ` + pf + `\241')"''\''\'\'''\'''` + pf + `\241')")`},
		// controls: ESC, BEL, CR, DEL, C1, bidi override; newline stays literal.
		{Posix, false, []string{"img", "\x1b]52;c;aGk=\x07\rX\x7f\u0085\u202e\n"}, `(LC_ALL=C; docker run img ` + pf + `\033')"']52;c;aGk='` + pf + `\007\015')"'X'` + pf + `\177\302\205\342\200\256')"'` + "\n')"},
		{Posix, false, []string{"img", "\x1b"}, `docker run img ` + pf + `\033')"`},

		{PowerShell, false, []string{"--name=web", "--env=A=b c", "img", "it's", "--", "-a.b", "x,y"}, `docker run --name=web '--env=A=b c' img 'it''s' '--' '-a.b' 'x,y'`},
		{PowerShell, false, []string{"img", "$(id)`;\u2019x"}, psGuard + "docker run img \"`$(id)``;`u{2019}x\""},
		{PowerShell, false, []string{"img", "\x1b\"", ""}, psGuard + "docker run img \"`u{1b}`\"\" ''"},
		{PowerShell, false, []string{"img", `C:\x\`}, psGuard + `docker run img 'C:\x\'`},
		{PowerShell, true, []string{"--detach", "img", "a"}, "docker run `\n\t--detach `\n\timg `\n\ta"},
		// --% in any quoting would stop PowerShell's parsing (checked on Windows).
		{PowerShell, false, []string{"img", "--%", "%PATH%"}, "docker run img @('--%') '%PATH%'"},

		{Cmd, false, []string{"--env=A=b c", "img", `a"b\`, `%PATH%`, `x & del *`, "\\\\\"", "ち"}, `docker run ^"--env=A=b c^" img ^"a\^"b\\^" ^"^%PATH^%^" ^"x ^& del *^" ^"\\\\\^"^" ^"ち^"`},
		{Cmd, true, []string{"--detach", "img"}, "docker run ^\n  --detach ^\n  img"},
	} {
		got, err := Format(cmdOf(tc.argv...), FormatOptions{Shell: tc.sh, Pretty: tc.pretty})
		if err != nil || got != tc.want {
			t.Errorf("Format(%s, %q, %v)\n got %q, %v\nwant %q", tc.sh, tc.argv, tc.pretty, got, err, tc.want)
		}
	}
	for _, sh := range []Shell{Posix, PowerShell, Cmd} {
		if _, err := Format(cmdOf("img", "a\x00b"), FormatOptions{Shell: sh}); err == nil {
			t.Errorf("%s: NUL byte: want error", sh)
		}
	}
	// Posix and PowerShell write every non-ASCII character by its code.
	for _, r := range "\u200e\u200f\u061c\u2028\u2029\ufeff\u200b\u202e\u2066" {
		for _, sh := range []Shell{Posix, PowerShell} {
			got, err := Format(cmdOf("img", "a"+string(r)), FormatOptions{Shell: sh})
			if err != nil || !printable(got) {
				t.Errorf("%s %U: got %q, %v; want printable ASCII", sh, r, got, err)
			}
		}
	}
	for _, bad := range []struct {
		sh  Shell
		arg string
	}{{"fish", "a"}, {PowerShell, "\xff"}, {Cmd, "\xff"}, {Cmd, "a\nb"}, {Cmd, "\u202e"}, {Cmd, "\u0085"}, {Cmd, "\x1b"},
		{Cmd, "a\u200eb"}, {Cmd, "\u200f"}, {Cmd, "\u061c"}, {Cmd, "\u2028"}, {Cmd, "\u2029"}, {Cmd, "\ufeff"}, {Cmd, "\u200b"}} {
		if got, err := Format(cmdOf("img", bad.arg), FormatOptions{Shell: bad.sh}); err == nil {
			t.Errorf("%s %q: want error, got %q", bad.sh, bad.arg, got)
		}
	}
}

// locales the Posix fuzz runs every shell under. The EUC-type ones treat
// bytes 0xA1-0xFE as multibyte lead bytes in bash.
var locales = []string{"en_US.UTF-8", "ko_KR.eucKR", "ko_KR.CP949", "zh_CN.eucCN", "zh_CN.GB2312", "ja_JP.eucJP"}

// shells that must turn Format's output back into the same arguments.
func shells(t testing.TB) [][]string {
	var out [][]string
	for _, s := range [][]string{{"/bin/sh"}, {"/bin/bash"}, {"/opt/homebrew/bin/bash"}, {"/usr/local/bin/bash"}, {"/usr/bin/bash"},
		{"zsh"}, {"zsh", "-o", "magicequalsubst"}, {"dash"}} {
		if p, err := exec.LookPath(s[0]); err == nil && !slices.ContainsFunc(out, func(o []string) bool { return slices.Equal(o, append([]string{p}, s[1:]...)) }) {
			out = append(out, append([]string{p}, s[1:]...))
		}
	}
	if len(out) == 0 {
		t.Skip("no shell found")
	}
	return out
}

// FuzzFormatPosix evaluates Format's output the way eval "$(runlike x)"
// does, in every shell and locale, with docker replaced by a program
// that prints its arguments NUL-separated. It also checks the line is
// printable ASCII (plus newline and the pretty tab).
func FuzzFormatPosix(f *testing.F) {
	for _, s := range []string{"", "a b", "it's", "$(id)", "`id`", "\n", "=ls", "a==ls", "a:=ls", "~root", "*", "\\", "\"'\"", "-", "--x=$HOME", "!!",
		"\xff\xfe", "é", "ち", "\xa1'", "\xa1'\\''\xa1", "\x1b]52;c;aGk=\x07", "\r", "\u202e", "\u0085", "\n\n", "\x01\n"} {
		f.Add(s, s, false)
		f.Add(s, s, true)
	}
	sh := shells(f)
	// docker is an external program, as in real use: a function's own
	// printf "$@" would run through the shell's expansion bugs a second time.
	bin := f.TempDir()
	if err := os.WriteFile(bin+"/docker", []byte("#!/bin/dash\nprintf '%s\\0' docker \"$@\"\n"), 0o755); err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, opt, word string, pretty bool) {
		if strings.ContainsRune(opt+word, 0) {
			t.Skip("NUL cannot be an argument")
		}
		c := Command{Options: []string{"--env=" + opt, "-" + opt}, Image: "img", Args: []string{word, opt}}
		line, err := Format(c, FormatOptions{Pretty: pretty})
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < len(line); i++ {
			if b := line[i]; (b < 0x20 || b > 0x7e) && b != '\n' && b != '\t' {
				t.Fatalf("byte %#x in output %q", b, line)
			}
		}
		for _, s := range sh {
			for _, loc := range locales {
				cmd := exec.Command(s[0], append(s[1:], "-c", `eval "$RUNLIKE_LINE"`)...)
				cmd.Env = []string{"LC_ALL=" + loc, "PATH=" + bin + ":/usr/bin:/bin", "HOME=/nonexistent", "RUNLIKE_LINE=" + line}
				out, err := cmd.Output()
				if err != nil {
					t.Fatalf("%s %s: %v on %q", s, loc, err, line)
				}
				got := strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
				want := c.Argv()
				if !slices.Equal(got, want) {
					t.Fatalf("%s %s: round trip failed\nline: %q\n got: %q\nwant: %q", s, loc, line, got, c.Argv())
				}
			}
		}
	})
}

// TestLocalesExist keeps the fuzz honest: an unknown LC_ALL silently
// falls back to C, which would hide the EUC bug, and a missing shell is
// silently not tried. Without them it skips, except on CI (CI set), where
// .github/workflows/ci.yml installs them.
func TestLocalesExist(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no POSIX shells: the Posix fuzz runs on Linux and macOS")
	}
	var missing []string
	for _, l := range locales {
		cmd := exec.Command("locale", "charmap")
		cmd.Env = []string{"LC_ALL=" + l}
		out, err := cmd.CombinedOutput()
		if cm := strings.TrimSpace(string(out)); err != nil || cm == "US-ASCII" || cm == "ANSI_X3.4-1968" || strings.Contains(cm, "\n") {
			missing = append(missing, "locale "+l)
		}
	}
	for _, sh := range []string{"bash", "zsh", "dash"} {
		if _, err := exec.LookPath(sh); err != nil {
			missing = append(missing, "shell "+sh)
		}
	}
	if len(missing) == 0 {
		return
	}
	if os.Getenv("CI") != "" {
		t.Fatalf("missing on CI: %s", strings.Join(missing, ", "))
	}
	t.Skipf("FuzzFormatPosix runs without: %s", strings.Join(missing, ", "))
}

// FuzzFormatCmd checks the cmd dialect against a model of cmd.exe (caret
// removal; no unescaped metacharacter may remain) and of the Windows
// argv parser docker.exe uses.
func FuzzFormatCmd(f *testing.F) {
	for _, s := range []string{"", "a b", `a"b`, `a\`, `a\\"b\`, "%PATH%", "^", "!x!", "&|<>()", "ち", `\\server\share\`} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		c := Command{Options: []string{"--label=" + s}, Image: "img", Args: []string{s, "x"}}
		line, err := Format(c, FormatOptions{Shell: Cmd})
		invisible := func(r rune) bool { // controls, bidi and format characters, separators
			return r < 0x20 || r >= 0x7f && r <= 0x9f || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp)
		}
		if err != nil {
			if !utf8.ValidString(s) || strings.ContainsFunc(s, invisible) {
				t.Skip()
			}
			t.Fatal(err)
		}
		if strings.ContainsFunc(line, invisible) {
			t.Fatalf("invisible character in %q", line)
		}
		var raw strings.Builder // what cmd passes to CreateProcess
		for i := 0; i < len(line); i++ {
			if line[i] == '^' {
				i++
			} else if strings.IndexByte(`&|<>()%!"`, line[i]) >= 0 {
				t.Fatalf("unescaped %q in %q", line[i], line)
			}
			raw.WriteByte(line[i])
		}
		if got := winArgv(raw.String()); !slices.Equal(got, c.Argv()) {
			t.Fatalf("line %q\n got %q\nwant %q", line, got, c.Argv())
		}
	})
}

// winArgv splits a command line like the MSVCRT / Go Windows parser.
func winArgv(s string) []string {
	var args []string
	var cur strings.Builder
	in, have := false, false
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '\\':
			n := 0
			for i < len(s) && s[i] == '\\' {
				n++
				i++
			}
			if i < len(s) && s[i] == '"' {
				cur.WriteString(strings.Repeat(`\`, n/2))
				if n%2 == 1 {
					cur.WriteByte('"')
				} else {
					in = !in
				}
			} else {
				cur.WriteString(strings.Repeat(`\`, n))
				i--
			}
			have = true
		case c == '"':
			in, have = !in, true
		case (c == ' ' || c == '\t') && !in:
			if have {
				args = append(args, cur.String())
			}
			cur.Reset()
			have = false
		default:
			cur.WriteByte(c)
			have = true
		}
	}
	if have {
		args = append(args, cur.String())
	}
	return args
}
