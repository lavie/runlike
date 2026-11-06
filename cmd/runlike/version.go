package main

import "github.com/spf13/pflag"

// version is set at build time: -ldflags "-X main.version=$(git describe)".
var version = "dev"

// addVersionFlag registers --version (new in 2.0; runlike 1.x had none).
func addVersionFlag(fs *pflag.FlagSet) *bool {
	return fs.Bool("version", false, "Show the version and exit.")
}
