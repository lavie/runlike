package runlike

import "strings"

// metadataArgs: labels, annotations, and the client's --cidfile.
func metadataArgs(x *ctx) []string {
	var a []string
	if !x.opts.NoLabels {
		// --label-file lands here too: the client reads the file into Labels.
		// Labels the image sets with the same value need no flag.
		printed := map[string]string{}
		for k, v := range x.cfg.Labels {
			if iv, ok := x.imageCfg().Labels[k]; (!ok || iv != v) && !(x.d.Desktop && strings.HasPrefix(k, desktopLabels)) {
				printed[k] = v
			}
		}
		x.eqKeys("--label", printed)
		for _, k := range sortedKeys(printed) {
			a = append(a, "--label="+k+"="+printed[k])
		}
	}
	x.eqKeys("--annotation", x.hc.Annotations)
	for _, k := range sortedKeys(x.hc.Annotations) {
		a = append(a, "--annotation="+k+"="+x.hc.Annotations[k])
	}
	// Recreating fails while the old file exists; printed anyway (ruling).
	if f := x.hc.ContainerIDFile; f != "" {
		if x.d.Platform == "" { // --stdin: the JSON may come from anyone
			x.warn("--cidfile=%s: running the command creates that file on this machine (from --stdin data, check the path)", f)
		}
		a = append(a, "--cidfile="+f)
	}
	return a
}
