package roundtrip

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/lavie/runlike/v2"
)

// TestCidfile: --cidfile is printed back (ruling). docker run refuses an
// existing cid file, so the generic harness cannot recreate it: this test
// removes the file between A and the copy, then compares as usual.
func TestCidfile(t *testing.T) {
	needLinux(t)
	p := "rt-client-cidfile-" + randHex()
	cid := filepath.Join(t.TempDir(), "my cid's file")
	var ids []string
	t.Cleanup(func() { docker(append([]string{"rm", "-f"}, ids...)...) })
	out, err := docker("create", "--name="+p+"-a", "--label="+sweepLabel, "--cidfile="+cid, alpine, "true")
	if err != nil {
		t.Fatal(err)
	}
	ids = append(ids, strings.TrimSpace(out))
	jsA, _ := docker("container", "inspect", ids[0])
	jsImg, _ := imageJSON(alpine)
	rc, err := runlike.Args(runlike.Inspect{Container: []byte(jsA), Image: []byte(jsImg)}, runlike.Daemon{}, runlike.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(rc.Options, "--cidfile="+cid) {
		t.Fatalf("no --cidfile in %q", rc.Options)
	}
	if err := os.Remove(cid); err != nil {
		t.Fatal(err)
	}
	b := forB(rc, p+"-b", false).Argv()
	b[1] = "create"
	out, err = docker(b[1:]...)
	if err != nil {
		t.Fatal(err)
	}
	ids = append(ids, strings.TrimSpace(out))
	jsB, _ := docker("container", "inspect", ids[1])
	if d := diff(normalize(t, jsA, Case{}), normalize(t, jsB, Case{})); d != "" {
		t.Errorf("copy differs:\n%s", d)
	}
}
