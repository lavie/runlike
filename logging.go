package runlike

// loggingArgs: log driver and its options. json-file is Docker's built-in
// default; any other driver is printed, even when it is the daemon's own
// configured default (Docker Desktop's local), so the line works on any
// daemon. json-file too is printed on a daemon whose default is another
// driver (isDefault). The options are printed in full: the daemon copies its
// configured log-opts into the container, and they apply to this driver.
func loggingArgs(x *ctx) []string {
	var a []string
	lc := x.hc.LogConfig
	if lc.Type != "" && !isDefault(lc.Type, builtinLogDriver, x.d.Defaults.LogDriver) {
		a = append(a, "--log-driver="+lc.Type)
	}
	x.eqKeys("--log-opt", lc.Config)
	for _, k := range sortedKeys(lc.Config) {
		a = append(a, "--log-opt="+k+"="+lc.Config[k])
	}
	return a
}
