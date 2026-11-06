package runlike

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Shell names a quoting dialect for Format.
type Shell string

// Dialects. What each guarantees, for any argument without a NUL byte:
//
// Posix (sh, bash, zsh, dash, also via eval "$(runlike x)"): the line
// evaluates to exactly the argument bytes, in any locale, including EUC
// and GB/CP949 locales whose multibyte decoding would eat a quote after a
// byte >= 0x80, and with zsh's magic_equal_subst. The line itself is
// printable ASCII plus newline: every other byte (C0 controls except
// newline, DEL, and every byte >= 0x80, so also C1 controls and bidi
// overrides) is written as "$(printf '\NNN')" in octal. A newline stays
// literal inside single quotes, because $(...) would strip it. When any
// argument holds a byte >= 0x80, the line runs as (LC_ALL=C; docker ...):
// bash 3.2 and 5.x in those locales otherwise corrupt bytes next to it
// during quote removal (a stray \x01, a lost quoted space). The parse is
// done by then and the line is ASCII, so only expansion sees LC_ALL=C;
// the subshell keeps the caller's locale.
//
// PowerShell (pwsh 7.3+, pasted or via -Command / -File): docker gets
// exactly the arguments. The line is printable ASCII: controls and all
// non-ASCII characters are written as `u{X} inside double quotes. Windows
// PowerShell 5.1 and $PSNativeCommandArgumentPassing=Legacy mangle
// arguments that are empty, hold a double quote or end in a backslash
// (and know no `u{}), which can shift argument boundaries; a line with
// any such argument starts with a guard that throws instead of running
// docker there.
//
// Cmd (cmd.exe prompt or cmd /c, delayed expansion off, the default):
// each argument is quoted for the Windows command-line parser docker.exe
// uses, then every cmd metacharacter (^ & | < > ( ) % ! ") is escaped
// with ^, so cmd sees no quoted text, operators or %VAR% references
// (only a variable whose name ends in ^ could expand). cmd has no way to
// write a character by its code, so control characters, C1 controls,
// format characters (Unicode Cf: bidi marks and overrides, zero-width
// joiners, BOM) and line/paragraph separators are an error; other
// non-ASCII text is printed as is.
// Not for batch files, where % must be doubled instead.
//
// PowerShell and Cmd need valid UTF-8: Windows command lines are UTF-16.
const (
	Posix      Shell = "posix"
	PowerShell Shell = "powershell"
	Cmd        Shell = "cmd"
)

// FormatOptions select the shell dialect and layout.
type FormatOptions struct {
	Shell  Shell // "" means Posix
	Pretty bool  // one option per line, with line continuations
}

// psGuard stops Windows PowerShell 5.1 and Legacy argument passing; see
// the PowerShell dialect above.
const psGuard = "if ($PSNativeCommandArgumentPassing -notin 'Standard','Windows') { throw 'runlike: this command needs PowerShell 7.3 or later' }; "

// Format quotes cmd.Argv() for one shell and joins it into one command
// line. With Pretty, "docker run", each option, the image and the
// command go on their own lines.
func Format(cmd Command, f FormatOptions) (string, error) {
	var quote func(string) (string, error)
	cont := " \\\n\t"
	switch f.Shell {
	case Posix, "":
		quote = quotePosix
	case PowerShell:
		quote, cont = quotePowerShell, " `\n\t"
	case Cmd:
		quote, cont = quoteCmd, " ^\n  "
	default:
		return "", fmt.Errorf("unknown shell %q (want posix, powershell or cmd)", f.Shell)
	}
	q := func(words []string) ([]string, error) {
		out := make([]string, len(words))
		for i, w := range words {
			if strings.IndexByte(w, 0) >= 0 {
				return nil, errors.New("argument contains a NUL byte, which no shell can pass")
			}
			var err error
			if out[i], err = quote(w); err != nil {
				return nil, err
			}
		}
		return out, nil
	}
	opts, err := q(cmd.Options)
	if err != nil {
		return "", err
	}
	tail, err := q(append([]string{cmd.Image}, cmd.Args...))
	if err != nil {
		return "", err
	}
	var lines []string
	if f.Pretty {
		lines = append(append([]string{"docker run"}, opts...), tail[0])
		if len(tail) > 1 {
			lines = append(lines, strings.Join(tail[1:], " "))
		}
	} else {
		cont = " "
		lines = append(append([]string{"docker", "run"}, opts...), tail...)
	}
	line := strings.Join(lines, cont)
	switch {
	case f.Shell == PowerShell && needsPSGuard(cmd.Argv()):
		line = psGuard + line
	case (f.Shell == Posix || f.Shell == "") && !ascii(strings.Join(cmd.Argv(), "")):
		line = "(LC_ALL=C; " + line + ")"
	}
	return line, nil
}

// quotePosix leaves s bare when it holds only characters no POSIX shell
// or zsh treats specially. Otherwise it single-quotes runs of printable
// ASCII and newline (a ' becomes: close quote, \', reopen) and all other bytes in
// octal through printf. A leading "=" is quoted because zsh expands =cmd,
// "==" and ":=" because zsh's magic_equal_subst expands =cmd after them.
func quotePosix(s string) (string, error) {
	if s != "" && s[0] != '=' && strings.Trim(s, safe) == "" &&
		!strings.Contains(s, "==") && !strings.Contains(s, ":=") {
		return s, nil
	}
	plain := func(c byte) bool { return c == '\n' || c >= 0x20 && c < 0x7f }
	var b strings.Builder
	for i := 0; i < len(s); {
		j := i
		if plain(s[i]) {
			for j < len(s) && plain(s[j]) {
				j++
			}
			b.WriteString("'" + strings.ReplaceAll(s[i:j], "'", `'\''`) + "'")
		} else {
			b.WriteString(`"$(printf '`)
			for ; j < len(s) && !plain(s[j]); j++ {
				fmt.Fprintf(&b, `\%03o`, s[j])
			}
			b.WriteString(`')"`)
		}
		i = j
	}
	if s == "" {
		return "''", nil
	}
	return b.String(), nil
}

const safe = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_@%+=:,./-"

// winSafe characters need no quoting in PowerShell or cmd arguments.
const winSafe = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_+=:,./-"

func quotePowerShell(s string) (string, error) {
	if !utf8.ValidString(s) {
		return "", errors.New("--shell powershell: argument is not valid UTF-8")
	}
	// PowerShell treats an argument whose value is --% as the stop-parsing
	// token however it is quoted: it drops it and passes the rest of the
	// line raw, %VAR% expanded (pwsh 7.6 and 5.1). An array element is
	// passed as is.
	if s == "--%" {
		return "@('--%')", nil
	}
	// Bare words, but not "--" (end of parameters), "," (array), and not
	// a "-word" holding "." or ":", which PowerShell may split.
	if s != "" && s != "--" && strings.Trim(s, winSafe) == "" && !strings.Contains(s, ",") &&
		(s[0] != '-' || !strings.ContainsAny(s, ".:")) {
		return s, nil
	}
	if printable(s) { // single quotes: only ' is special (and its look-alikes, which are not ASCII)
		return "'" + strings.ReplaceAll(s, "'", "''") + "'", nil
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '`' || r == '"' || r == '$':
			b.WriteByte('`')
			b.WriteRune(r)
		case r >= 0x20 && r < 0x7f:
			b.WriteRune(r)
		default:
			fmt.Fprintf(&b, "`u{%x}", r)
		}
	}
	b.WriteByte('"')
	return b.String(), nil
}

// needsPSGuard reports whether Legacy argument passing would mangle args.
func needsPSGuard(args []string) bool {
	for _, a := range args {
		if a == "" || strings.ContainsRune(a, '"') || strings.HasSuffix(a, `\`) {
			return true
		}
		if !printable(a) {
			return true // written with `u{}
		}
	}
	return false
}

// printable reports whether s is all printable ASCII.
func printable(s string) bool {
	return !strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 || r >= 0x7f })
}

// ascii reports whether s has no byte >= 0x80.
func ascii(s string) bool {
	return !strings.ContainsFunc(s, func(r rune) bool { return r >= 0x80 })
}

func quoteCmd(s string) (string, error) {
	if !utf8.ValidString(s) {
		return "", errors.New("--shell cmd: argument is not valid UTF-8")
	}
	for _, r := range s {
		// Cf holds bidi marks and overrides, zero-width and other format
		// characters; Zl, Zp the Unicode line and paragraph separators.
		if r < 0x20 || r >= 0x7f && r <= 0x9f || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp) {
			return "", fmt.Errorf("--shell cmd cannot write control character %U; use --shell powershell", r)
		}
	}
	if s != "" && strings.Trim(s, winSafe) == "" && !strings.Contains(s, ",") {
		return s, nil
	}
	// Quote for the Windows argv parser: backslashes are literal except
	// before a quote, where 2n backslashes + \" mean n backslashes + ".
	var w strings.Builder
	w.WriteByte('"')
	n := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			n++
			continue
		case '"':
			w.WriteString(strings.Repeat(`\`, 2*n+1))
		default:
			w.WriteString(strings.Repeat(`\`, n))
		}
		n = 0
		w.WriteByte(s[i])
	}
	w.WriteString(strings.Repeat(`\`, 2*n) + `"`)
	var b strings.Builder
	for _, c := range []byte(w.String()) {
		if strings.IndexByte(`^&|<>()%!"`, c) >= 0 {
			b.WriteByte('^')
		}
		b.WriteByte(c)
	}
	return b.String(), nil
}
