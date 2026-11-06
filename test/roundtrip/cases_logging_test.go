package roundtrip

// Logging area: --log-driver, --log-opt. The test daemons use json-file
// (the built-in default); the goldens in /testdata show a daemon whose
// default is local (Docker Desktop) printing --log-driver=local.
var _ = add("logging",
	Case{Name: "default", Args: []string{alpine, "true"}},
	Case{Name: "json-file-explicit", Args: []string{"--log-driver=json-file", alpine, "true"}},
	Case{Name: "json-file-opts", Args: []string{"--log-opt", "max-size=10m", "--log-opt", "max-file=3", "--log-opt", "labels=a,b", alpine, "true"}},
	Case{Name: "none", Args: []string{"--log-driver=none", alpine, "true"}},
	Case{Name: "local-opts", Args: []string{"--log-driver=local", "--log-opt", "max-size=1m", "--log-opt", "compress=false", alpine, "true"}},
	Case{Name: "tag-template", Args: []string{"--log-opt", "tag={{.Name}} {{.ImageName}}", "--log-opt", "env=A,B", alpine, "true"}},
	Case{Name: "syslog", Args: []string{"--log-driver=syslog", "--log-opt", "syslog-address=udp://127.0.0.1:514", "--log-opt", "tag=x y", alpine, "true"}},
)
