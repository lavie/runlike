// Command runlike prints the docker run command that recreates an
// existing container. Its flags match runlike 1.x (Python/click).
package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode"

	"github.com/lavie/runlike/v2"
	"github.com/lavie/runlike/v2/internal/daemon"
	"github.com/spf13/pflag"
)

const usage = "Usage: runlike [OPTIONS] [CONTAINER]"

const helpText = usage + `

  Shows command line necessary to run copy of existing Docker container.

  Run the copy with: eval "$(runlike CONTAINER)". Warnings go to stderr.
  With --stdin there is no image data: values the image sets are printed
  too, and an entrypoint cleared with --entrypoint= cannot be seen.

Options:
  --no-name            Do not include container name in output
  --use-volume-id      Keep the automatically assigned volume id
  -p, --pretty         One option per line
  -s, --stdin          Read docker inspect JSON from stdin
  -l, --no-labels      Do not include labels in output
  --shell SHELL        Quote for posix (default), powershell or cmd
  --version            Show the version and exit.
  --help               Show this message and exit.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(argv []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := pflag.NewFlagSet("runlike", pflag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.SortFlags = false
	var opts runlike.Options
	fs.BoolVar(&opts.NoName, "no-name", false, "Do not include container name in output")
	fs.BoolVar(&opts.UseVolumeID, "use-volume-id", false, "Keep the automatically assigned volume id")
	pretty := fs.BoolP("pretty", "p", false, "One option per line")
	useStdin := fs.BoolP("stdin", "s", false, "Read docker inspect JSON from stdin")
	fs.BoolVarP(&opts.NoLabels, "no-labels", "l", false, "Do not include labels in output")
	shell := fs.String("shell", "posix", "Quote for this shell: posix, powershell or cmd")
	help := fs.Bool("help", false, "Show this message and exit.") // defined so -h stays unknown, as in click
	showVersion := addVersionFlag(fs)

	usageErr := func(msg string) int {
		fmt.Fprintf(stderr, "%s\nTry 'runlike --help' for help.\n\nError: %s\n", usage, msg)
		return 2
	}
	if msg := boolValue(fs, argv); msg != "" {
		return usageErr(msg)
	}
	if err := fs.Parse(argv); err != nil {
		return usageErr(err.Error())
	}
	if *help {
		fmt.Fprint(stdout, helpText)
		return 0
	}
	if *showVersion {
		fmt.Fprintln(stdout, "runlike", version)
		return 0
	}
	if fs.NArg() > 1 {
		return usageErr(fmt.Sprintf("Got unexpected extra argument (%s)", strings.Join(fs.Args()[1:], " ")))
	}
	container := fs.Arg(0)
	if container == "" && !*useStdin {
		return usageErr("usage error")
	}

	var in runlike.Inspect
	var dm runlike.Daemon
	var warnings []string
	var err error
	if container != "" {
		in, dm, warnings, err = daemon.CLI{}.Inspect(container)
	} else {
		in.Container, err = io.ReadAll(stdin)
	}
	if err != nil {
		fmt.Fprintln(stderr, sanitize(err.Error()))
		return 1
	}
	cmd, err := runlike.Args(in, dm, opts)
	if err == nil {
		var line string
		if line, err = runlike.Format(cmd, runlike.FormatOptions{Shell: runlike.Shell(*shell), Pretty: *pretty}); err == nil {
			fmt.Fprintln(stdout, line)
			for _, w := range append(warnings, cmd.Warnings...) {
				fmt.Fprintln(stderr, "runlike: warning:", sanitize(w))
			}
			return 0
		}
	}
	fmt.Fprintln(stderr, "runlike:", sanitize(err.Error()))
	return 1
}

// sanitize escapes what a terminal would act on: every rune that is not
// printable, tab or newline (ESC, C1 controls, bidi format characters),
// as \x1b or \u202e. Warnings and errors quote container data.
func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsPrint(r) || r == '\t' || r == '\n' {
			b.WriteRune(r)
		} else {
			q := strconv.QuoteRuneToASCII(r)
			b.WriteString(q[1 : len(q)-1])
		}
	}
	return b.String()
}

// boolValue returns click's error for a value given to a flag that takes
// none ("--pretty=true", "-p=1"), which pflag would accept.
func boolValue(fs *pflag.FlagSet, argv []string) string {
	isBool := func(f *pflag.Flag) bool { return f != nil && f.Value.Type() == "bool" }
	for i := 0; i < len(argv); i++ {
		a := argv[i]
		switch {
		case a == "--":
			return ""
		case strings.HasPrefix(a, "--"):
			name, _, hasValue := strings.Cut(a[2:], "=")
			f := fs.Lookup(name)
			if hasValue && isBool(f) {
				return fmt.Sprintf("Option '--%s' does not take a value.", name)
			}
			if f != nil && !isBool(f) && !hasValue {
				i++ // its value
			}
		case strings.HasPrefix(a, "-") && len(a) > 1:
			for _, c := range a[1:] {
				if !isBool(fs.ShorthandLookup(string(c))) {
					return fmt.Sprintf("No such option: -%c", c)
				}
			}
		}
	}
	return ""
}
