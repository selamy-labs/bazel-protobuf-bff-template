package mutationwitness

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runfile(t *testing.T, envKey string) string {
	t.Helper()
	rel := os.Getenv(envKey)
	if rel == "" {
		t.Fatalf("%s is not set", envKey)
	}
	path := filepath.Join(os.Getenv("TEST_SRCDIR"), rel)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("runfile %s (%s): %v", envKey, path, err)
	}
	return path
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func setupFixtureRepo(t *testing.T, goCode, testCode, floorsJSON, eqJSON string) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, root, "pkg/mathx/mathx.go", goCode)
	writeFile(t, root, "pkg/mathx/mathx_test.go", testCode)
	writeFile(t, root, "quality/mutation_floors.json", floorsJSON)
	writeFile(t, root, "quality/mutation_equivalents.json", eqJSON)
	return root
}

const defaultFloorsJSON = `{
  "measured_at": "witness",
  "equivalents": "quality/mutation_equivalents.json",
  "timeout_coefficient": 100,
  "packages": [
    {
      "name": "pkg/mathx",
      "module": "example.com/witness",
      "go_version": "1.27.4",
      "package": "./pkg/mathx",
      "sources": ["pkg/mathx"],
      "floor": 100
    }
  ]
}`

const cleanGoSource = `package mathx

func IsPositive(n int) bool {
	return n > 0
}
`

const strongTestSource = `package mathx

import "testing"

func TestIsPositive(t *testing.T) {
	if !IsPositive(1) || IsPositive(0) || IsPositive(-1) {
		t.Fatal("unexpected result")
	}
}
`

const weakTestSource = `package mathx

import "testing"

func TestIsPositive(t *testing.T) {
	if !IsPositive(1) || IsPositive(-1) {
		t.Fatal("unexpected result")
	}
}
`

const failingTestSource = `package mathx

import "testing"

func TestIsPositive(t *testing.T) {
	t.Fatal("pristine failure")
}
`

func runMutationCLI(t *testing.T, args ...string) (int, string) {
	t.Helper()
	cmd := exec.Command(runfile(t, "MUTATION_BIN"), args...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(out)
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode(), string(out)
	}
	t.Fatalf("exec mutation_bin: %v\n%s", err, string(out))
	return -1, ""
}

func TestRealGremlinsPassesCleanControl(t *testing.T) {
	// Arrange
	root := setupFixtureRepo(t, cleanGoSource, strongTestSource, defaultFloorsJSON, `{"equivalents":[]}`)

	// Act
	code, out := runMutationCLI(t, "check", "--root="+root, "--gremlins="+runfile(t, "GREMLINS_BIN"), "--go="+runfile(t, "GO_BIN"))

	// Assert
	if code != 0 || !strings.Contains(out, "measurement: mutation 1 packages, 2/2 killed (100.0%)") {
		t.Fatalf("clean control: code=%d out=%s", code, out)
	}
}

func TestRealGremlinsRejectsSurvivingMutant(t *testing.T) {
	// Arrange
	root := setupFixtureRepo(t, cleanGoSource, weakTestSource, defaultFloorsJSON, `{"equivalents":[]}`)

	// Act
	code, out := runMutationCLI(t, "check", "--root="+root, "--gremlins="+runfile(t, "GREMLINS_BIN"), "--go="+runfile(t, "GO_BIN"))

	// Assert
	if code != 1 || !strings.Contains(out, "LIVED CONDITIONALS_BOUNDARY at mathx.go:4:11") {
		t.Fatalf("planted survivor: code=%d out=%s", code, out)
	}
}

func TestRealGremlinsRefusesPristineTestFailure(t *testing.T) {
	// Arrange
	root := setupFixtureRepo(t, cleanGoSource, failingTestSource, defaultFloorsJSON, `{"equivalents":[]}`)

	// Act
	code, out := runMutationCLI(t, "check", "--root="+root, "--gremlins="+runfile(t, "GREMLINS_BIN"), "--go="+runfile(t, "GO_BIN"))

	// Assert
	if code != 2 || !strings.Contains(out, "pristine go test failed") {
		t.Fatalf("pristine failure: code=%d out=%s", code, out)
	}
}

func TestRealGremlinsAcceptsReviewedEquivalent(t *testing.T) {
	// Arrange
	eqJSON := `{
		"equivalents": [{
			"id": "EQ-WITNESS-1",
			"package": "pkg/mathx",
			"location": "mathx.go:4:11",
			"mutator": "CONDITIONALS_BOUNDARY",
			"proof": "witness equivalence fixture"
		}]
	}`
	root := setupFixtureRepo(t, cleanGoSource, weakTestSource, defaultFloorsJSON, eqJSON)

	// Act
	code, out := runMutationCLI(t, "check", "--root="+root, "--gremlins="+runfile(t, "GREMLINS_BIN"), "--go="+runfile(t, "GO_BIN"))

	// Assert
	if code != 0 || !strings.Contains(out, "1/1 killed (100.0%), 1 equivalent") {
		t.Fatalf("reviewed equivalent: code=%d out=%s", code, out)
	}
}

func TestRealGremlinsRejectsStaleEquivalent(t *testing.T) {
	// Arrange
	eqJSON := `{
		"equivalents": [{
			"id": "EQ-STALE-1",
			"package": "pkg/mathx",
			"location": "mathx.go:4:11",
			"mutator": "CONDITIONALS_BOUNDARY",
			"proof": "now killed by strong test"
		}]
	}`
	root := setupFixtureRepo(t, cleanGoSource, strongTestSource, defaultFloorsJSON, eqJSON)

	// Act
	code, out := runMutationCLI(t, "check", "--root="+root, "--gremlins="+runfile(t, "GREMLINS_BIN"), "--go="+runfile(t, "GO_BIN"))

	// Assert
	if code != 1 || !strings.Contains(out, "stale equivalent EQ-STALE-1") {
		t.Fatalf("stale equivalent: code=%d out=%s", code, out)
	}
}

func TestRealCompareRefusesLoweredFloor(t *testing.T) {
	// Arrange
	base := setupFixtureRepo(t, cleanGoSource, strongTestSource, defaultFloorsJSON, `{"equivalents":[]}`)
	lowered := strings.Replace(defaultFloorsJSON, `"floor": 100`, `"floor": 90`, 1)
	proposal := setupFixtureRepo(t, cleanGoSource, strongTestSource, lowered, `{"equivalents":[]}`)

	// Act
	code, out := runMutationCLI(t, "compare", "--base-root="+base, "--root="+proposal)

	// Assert
	if code != 1 || !strings.Contains(out, "floor lowered from 100.0% to 90.0%") {
		t.Fatalf("lowered floor: code=%d out=%s", code, out)
	}
}

func TestRealCompareRefusesSourceSelectionChangeAtSameFloor(t *testing.T) {
	// Arrange
	base := setupFixtureRepo(t, cleanGoSource, strongTestSource, defaultFloorsJSON, `{"equivalents":[]}`)
	swapped := strings.Replace(defaultFloorsJSON, `"sources": ["pkg/mathx"]`, `"sources": ["pkg/other"]`, 1)
	proposal := setupFixtureRepo(t, cleanGoSource, strongTestSource, swapped, `{"equivalents":[]}`)

	// Act
	code, out := runMutationCLI(t, "compare", "--base-root="+base, "--root="+proposal)

	// Assert
	if code != 1 || !strings.Contains(out, "source mapping changed") {
		t.Fatalf("source selection change: code=%d out=%s", code, out)
	}
}

func TestRealCompareRefusesDestinationMappingChangeAtSameFloor(t *testing.T) {
	// Arrange
	base := setupFixtureRepo(t, cleanGoSource, strongTestSource, defaultFloorsJSON, `{"equivalents":[]}`)
	redirected := strings.Replace(defaultFloorsJSON, `"sources": ["pkg/mathx"]`, `"strip_prefix": "pkg/", "sources": ["pkg/mathx"]`, 1)
	proposal := setupFixtureRepo(t, cleanGoSource, strongTestSource, redirected, `{"equivalents":[]}`)

	// Act
	code, out := runMutationCLI(t, "compare", "--base-root="+base, "--root="+proposal)

	// Assert
	if code != 1 || !strings.Contains(out, "source mapping changed") {
		t.Fatalf("destination mapping change: code=%d out=%s", code, out)
	}
}

func TestRealGremlinsRefusesSymlinkedSourceDirectory(t *testing.T) {
	// Arrange
	root := t.TempDir()
	outside := t.TempDir()
	writeFile(t, outside, "mathx.go", cleanGoSource)
	writeFile(t, outside, "mathx_test.go", strongTestSource)
	writeFile(t, root, "quality/mutation_floors.json", defaultFloorsJSON)
	writeFile(t, root, "quality/mutation_equivalents.json", `{"equivalents":[]}`)
	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "pkg", "mathx")); err != nil {
		t.Fatal(err)
	}

	// Act
	code, out := runMutationCLI(t, "check", "--root="+root, "--gremlins="+runfile(t, "GREMLINS_BIN"), "--go="+runfile(t, "GO_BIN"))

	// Assert
	if code != 2 || !strings.Contains(out, "must not be a symlink") {
		t.Fatalf("symlinked source dir: code=%d out=%s", code, out)
	}
}

func TestRealGremlinsRefusesSymlinkedGoFile(t *testing.T) {
	// Arrange
	root := t.TempDir()
	outside := t.TempDir()
	writeFile(t, outside, "mathx.go", cleanGoSource)
	writeFile(t, root, "pkg/mathx/mathx_test.go", strongTestSource)
	writeFile(t, root, "quality/mutation_floors.json", defaultFloorsJSON)
	writeFile(t, root, "quality/mutation_equivalents.json", `{"equivalents":[]}`)
	if err := os.Symlink(filepath.Join(outside, "mathx.go"), filepath.Join(root, "pkg", "mathx", "mathx.go")); err != nil {
		t.Fatal(err)
	}

	// Act
	code, out := runMutationCLI(t, "check", "--root="+root, "--gremlins="+runfile(t, "GREMLINS_BIN"), "--go="+runfile(t, "GO_BIN"))

	// Assert
	if code != 2 || !strings.Contains(out, "must not be a symlink") {
		t.Fatalf("symlinked Go file: code=%d out=%s", code, out)
	}
}
