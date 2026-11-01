// Package witness runs the U1000 planted control (spec 011 FR-002, FR-007) with a Go toolchain the caller names.
// The analysis test driver skips silently when the go command is not the one its own binary was built with,
// which under Bazel it never is, and a silent skip would be a setup failure counted as a pass; so the packages
// are loaded and the analyzer run directly, and every expectation is asserted here.
package witness

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
	"golang.org/x/tools/go/packages"

	"github.com/selamy-labs/bazel-protobuf-bff-template/quality/analysis/u1000"
)

// Run points the go command at goroot, refuses to continue without it, loads the planted package under testdata
// in GOPATH mode, runs the analyzer, and compares its diagnostics with want: "file:line: message" lines.
func Run(t *testing.T, goroot, testdata string, want []string) {
	t.Helper()
	t.Setenv("GOROOT", goroot)
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOCACHE", filepath.Join(t.TempDir(), "gocache"))
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GOPATH", testdata)
	t.Setenv("GO111MODULE", "off")
	t.Setenv("GOPROXY", "off")
	t.Setenv("GOFLAGS", "")
	t.Setenv("PATH", filepath.Join(goroot, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	if _, err := exec.LookPath("go"); err != nil {
		t.Fatalf("no go command under %s; the witness cannot run, and a skip is not a pass: %v", goroot, err)
	}
	config := &packages.Config{Mode: packages.LoadAllSyntax, Dir: testdata, Tests: false}
	pkgs, err := packages.Load(config, "planted")
	if err != nil {
		t.Fatalf("loading the planted package: %v", err)
	}
	if packages.PrintErrors(pkgs) > 0 || len(pkgs) != 1 {
		t.Fatalf("the planted package did not load cleanly: %d packages", len(pkgs))
	}
	graph, err := checker.Analyze([]*analysis.Analyzer{u1000.Analyzer}, pkgs, nil)
	if err != nil {
		t.Fatalf("analyzing: %v", err)
	}
	var got []string
	for action := range graph.All() {
		if action.Err != nil {
			t.Fatalf("%s: %v", action, action.Err)
		}
		if action.Analyzer != u1000.Analyzer {
			continue
		}
		for _, d := range action.Diagnostics {
			position := pkgs[0].Fset.Position(d.Pos)
			got = append(got, filepath.Base(position.Filename)+":"+itoa(position.Line)+": "+d.Message)
		}
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("diagnostics:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}
