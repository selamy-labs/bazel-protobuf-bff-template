package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFixture(t *testing.T, dir, rel, content string) string {
	t.Helper()
	full := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return full
}

func makeStubBins(t *testing.T, status string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	goBin := writeFixture(t, dir, "bin/go", "#!/bin/sh\nexit 0\n")
	gremlinsScript := `#!/bin/sh
out=""
while [ $# -gt 0 ]; do
  if [ "$1" = "--output" ]; then out="$2"; shift 2; continue; fi
  shift
done
printf '{"go_module":"m","files":[{"file_name":"a.go","mutations":[{"type":"ARITHMETIC_BASE","status":"` + status + `","line":1,"column":1}]}]}' > "$out"
`
	gremlinsBin := writeFixture(t, dir, "bin/gremlins", gremlinsScript)
	if err := os.Chmod(goBin, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(gremlinsBin, 0o700); err != nil {
		t.Fatal(err)
	}
	return goBin, gremlinsBin
}

func makeRepoFixture(t *testing.T, floor string) string {
	t.Helper()
	root := t.TempDir()
	writeFixture(t, root, "pkg/a/a.go", "package a\n")
	writeFixture(t, root, "quality/mutation_floors.json", `{
		"measured_at": "rev1",
		"equivalents": "quality/mutation_equivalents.json",
		"timeout_coefficient": 100,
		"packages": [{"name":"pkg/a","module":"example.com/m","go_version":"1.27.4","package":"./pkg/a","sources":["pkg/a"],"floor":`+floor+`}]
	}`)
	writeFixture(t, root, "quality/mutation_equivalents.json", `{"equivalents":[]}`)
	return root
}

func TestCLICheckPassesOnKilledMutant(t *testing.T) {
	// Arrange
	root := makeRepoFixture(t, "100")
	goBin, gremlinsBin := makeStubBins(t, "KILLED")
	var stdout, stderr bytes.Buffer

	// Act
	code := run([]string{"check", "--root=" + root, "--gremlins=" + gremlinsBin, "--go=" + goBin}, &stdout, &stderr)

	// Assert
	if code != 0 || !strings.Contains(stdout.String(), "measurement: mutation 1 packages, 1/1 killed (100.0%)") {
		t.Fatalf("check pass: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestCLICheckFailsOnLivedMutant(t *testing.T) {
	// Arrange
	root := makeRepoFixture(t, "100")
	goBin, gremlinsBin := makeStubBins(t, "LIVED")
	var stdout, stderr bytes.Buffer

	// Act
	code := run([]string{"check", "--root=" + root, "--gremlins=" + gremlinsBin, "--go=" + goBin}, &stdout, &stderr)

	// Assert
	if code != 1 || !strings.Contains(stdout.String(), "mutation pkg/a: LIVED") {
		t.Fatalf("check fail: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestCLICheckRefusesMissingFlags(t *testing.T) {
	// Arrange
	var stdout, stderr bytes.Buffer

	// Act
	code := run([]string{"check"}, &stdout, &stderr)

	// Assert
	if code != 2 {
		t.Fatalf("check missing flags: code=%d; want 2", code)
	}
}

func TestCLIComparePassesOnStricterFloor(t *testing.T) {
	// Arrange
	base := makeRepoFixture(t, "80")
	proposal := makeRepoFixture(t, "100")
	var stdout, stderr bytes.Buffer

	// Act
	code := run([]string{"compare", "--base-root=" + base, "--root=" + proposal}, &stdout, &stderr)

	// Assert
	if code != 0 || !strings.Contains(stdout.String(), "at least as strict") {
		t.Fatalf("compare pass: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestCLICompareFailsOnLoweredFloor(t *testing.T) {
	// Arrange
	base := makeRepoFixture(t, "100")
	proposal := makeRepoFixture(t, "80")
	var stdout, stderr bytes.Buffer

	// Act
	code := run([]string{"compare", "--base-root=" + base, "--root=" + proposal}, &stdout, &stderr)

	// Assert
	if code != 1 || !strings.Contains(stdout.String(), "floor lowered") {
		t.Fatalf("compare lowered: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestCLICompareRefusesMissingFlags(t *testing.T) {
	// Arrange
	var stdout, stderr bytes.Buffer

	// Act
	code := run([]string{"compare"}, &stdout, &stderr)

	// Assert
	if code != 2 {
		t.Fatalf("compare missing flags: code=%d; want 2", code)
	}
}
