package roundtrip

import "github.com/lavie/runlike/v2"

// Metadata area: labels; name / --no-name (handled in runlike.go).
var _ = add("metadata",
	Case{Name: "labels", Args: []string{"-l", "a=1", "-l", "b=two words", "-l", "empty=", alpine, "true"}},
	Case{Name: "labels-special", Args: []string{"-l", "q=it's \"x\" $Y `z`\nnext", "-l", "u=ünï 🐳", "-l", "com.example.k=v=w", alpine, "true"}},
	Case{Name: "labels-image-same", Args: []string{fixture}},
	Case{Name: "labels-image-override", Args: []string{"-l", "img.label=changed", fixture}},
	Case{Name: "no-labels", Args: []string{"-l", "a=1", alpine, "true"}, Opts: runlike.Options{NoLabels: true},
		Ignore: []Ignore{{"Config.Labels", "--no-labels leaves labels out by design"}}},
	Case{Name: "name", Args: []string{alpine, "true"}},
	Case{Name: "no-name", Args: []string{alpine, "true"}, Opts: runlike.Options{NoName: true}},
)

// Saved-data example: a container saved from the live env-quotes setup.
var _ = addSaved(SavedCase{Name: "saved-env", Want: []string{"--env=Q=it's \"quoted\" `x` $Y", "--label=k=v w"}})

var _ = add("metadata",
	// --label-file folds into --label
	Case{Name: "label-file", Args: []string{"--label-file", "testdata/metadata.labels", "-l", "lf.a=flag", alpine, "true"}},
	Case{Name: "annotation", Args: []string{"--annotation", "a=1", "--annotation", "com.example/x=two words", "--annotation", "e=", alpine, "true"}},
	Case{Name: "annotation-special", Args: []string{"--annotation", "q=it's \"x\" $Y\nz", "--annotation", "eq=a=b", alpine, "true"}},
)

// Label and annotation keys holding "=" (API only): docker run splits at
// the first "=", so runlike warns.
var _ = addSteps(stepCase{name: "label-key-eq-api", args: []string{alpine, "true"}, noCopy: true,
	api:  `{"Labels":{"a=b":"c"},"HostConfig":{"Annotations":{"x=y":"z"}}}`,
	want: []string{"--label=a=b=c", "--annotation=x=y=z"}, warn: []string{`--label key "a=b"`, `--annotation key "x=y"`}})
