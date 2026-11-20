package runner

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/selamy-labs/bazel-protobuf-bff-template/quality/proto/registrypb"
)

type artifactFixture struct {
	root, receipt, run, log string
	registry                *registrypb.Registry
	record                  Receipt
}

func newArtifactFixture(t *testing.T) *artifactFixture {
	t.Helper()
	h := newHarness(t)
	f := &artifactFixture{root: h.dir, receipt: filepath.Join(h.dir, "receipt.json")}
	f.registry = h.registry(bazelCheck("tests", "//a:t", "STAGE_FULL") + native("other", "true"))
	var err error
	f.run, err = newLogDirectory(f.receipt)
	if err != nil {
		t.Fatal(err)
	}
	f.log, err = RetainLog(f.run, "tests", "specific assertion got 7 want 11\n")
	if err != nil {
		t.Fatal(err)
	}
	f.record = Receipt{Result: "FAILED", Expected: 1, Checks: []Result{{Check: "tests", State: Failed, Log: f.log}}}
	f.write(t)
	return f
}

func (f *artifactFixture) write(t *testing.T) {
	t.Helper()
	if err := WriteReceipt(f.receipt, &f.record); err != nil {
		t.Fatal(err)
	}
}

func (f *artifactFixture) export() (string, error) {
	return ExportReceipt(context.Background(), ArtifactOptions{Registry: f.registry, Root: f.root, ReceiptPath: f.receipt})
}

func artifactNames(t *testing.T, root string) []string {
	t.Helper()
	var names []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			name, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			names = append(names, name)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(names)
	return names
}

func TestArtifactSelectsOnlyItsReferencedOwnedLogs(t *testing.T) {
	// Arrange
	f := newArtifactFixture(t)
	foreign := filepath.Join(f.root, "receipt.logs-foreign")
	if err := os.Mkdir(foreign, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel, ignored := filepath.Join(foreign, "keep"), filepath.Join(f.run, "unreferenced.log")
	for _, path := range []string{sentinel, ignored} {
		if err := os.WriteFile(path, []byte("unrelated"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	original, _ := os.ReadFile(f.receipt)
	// Act
	directory, err := f.export()
	if err != nil {
		t.Fatal(err)
	}
	copied, _ := os.ReadFile(filepath.Join(directory, "receipt.json"))
	kept, _ := os.ReadFile(sentinel)
	untouched, _ := os.ReadFile(ignored)
	relative, _ := filepath.Rel(f.root, f.log)
	want := []string{"receipt.json", relative}
	sort.Strings(want)
	// Assert
	if !reflect.DeepEqual(artifactNames(t, directory), want) || !bytes.Equal(original, copied) || string(kept) != "unrelated" || string(untouched) != "unrelated" {
		t.Fatalf("artifact changed receipt or selected unrelated content: %v", artifactNames(t, directory))
	}
}

func TestPassingArtifactContainsOnlyTheOriginalReceipt(t *testing.T) {
	// Arrange
	f := newArtifactFixture(t)
	f.record.Result, f.record.Checks[0].State, f.record.Checks[0].Log = "PASSED", Passed, ""
	f.write(t)
	original, _ := os.ReadFile(f.receipt)
	// Act
	directory, err := f.export()
	if err != nil {
		t.Fatal(err)
	}
	copied, _ := os.ReadFile(filepath.Join(directory, "receipt.json"))
	// Assert
	if !reflect.DeepEqual(artifactNames(t, directory), []string{"receipt.json"}) || !bytes.Equal(original, copied) {
		t.Fatal("passing export was not exactly its original receipt")
	}
}

func artifactMust(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestArtifactRefusesUnsafeReferences(t *testing.T) {
	cases := []struct {
		name, want string
		change     func(*testing.T, *artifactFixture)
	}{
		{"unmarked", "unmarked", func(t *testing.T, f *artifactFixture) { artifactMust(t, os.Remove(filepath.Join(f.run, logOwnerMark))) }},
		{"outside", "direct check filename", func(t *testing.T, f *artifactFixture) {
			f.record.Checks[0].Log = filepath.Join(t.TempDir(), "tests.log")
		}},
		{"unknown", "unknown or duplicate", func(t *testing.T, f *artifactFixture) { f.record.Checks[0].Check = "unknown" }},
		{"duplicate", "unknown or duplicate", func(t *testing.T, f *artifactFixture) { f.record.Checks = append(f.record.Checks, f.record.Checks[0]) }},
		{"count bound", "registry bound", func(t *testing.T, f *artifactFixture) {
			f.record.Checks = append(f.record.Checks, f.record.Checks[0], f.record.Checks[0])
		}},
		{"wrong filename", "direct check filename", func(t *testing.T, f *artifactFixture) { f.record.Checks[0].Log = filepath.Join(f.run, "other.log") }},
		{"nested", "direct check filename", func(t *testing.T, f *artifactFixture) {
			f.record.Checks[0].Log = filepath.Join(f.run, "nested", "tests.log")
		}},
		{"symlink file", "not regular", func(t *testing.T, f *artifactFixture) {
			artifactMust(t, os.Remove(f.log))
			artifactMust(t, os.Symlink(filepath.Join(t.TempDir(), "other"), f.log))
		}},
		{"symlink directory", "not a regular directory", func(t *testing.T, f *artifactFixture) {
			artifactMust(t, os.Rename(f.run, f.run+"-real"))
			artifactMust(t, os.Symlink(f.run+"-real", f.run))
		}},
		{"symlink marker", "unmarked", func(t *testing.T, f *artifactFixture) {
			mark := filepath.Join(f.run, logOwnerMark)
			artifactMust(t, os.Remove(mark))
			artifactMust(t, os.Symlink(filepath.Join(t.TempDir(), "mark"), mark))
		}},
		{"nonempty marker", "unmarked", func(t *testing.T, f *artifactFixture) {
			artifactMust(t, os.WriteFile(filepath.Join(f.run, logOwnerMark), []byte("foreign"), 0o600))
		}},
		{"oversized log", "exceeds its bound", func(t *testing.T, f *artifactFixture) {
			artifactMust(t, os.WriteFile(f.log, bytes.Repeat([]byte("x"), int(maxRetainedLog)+256), 0o600))
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			f := newArtifactFixture(t)
			test.change(t, f)
			f.write(t)
			// Act
			_, err := f.export()
			// Assert
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("unsafe reference accepted or wrong refusal: %v; want %q", err, test.want)
			}
		})
	}
}

func TestFailedBazelAssertionReachesPreparedArtifact(t *testing.T) {
	// Arrange
	h := committed(newHarness(t))
	fakeBazel(h, `printf '%s\n' '--- FAIL: TestSpecificAssertion (0.01s)' '    example_test.go:42: native assertion got 7 want 11' '//a:t FAILED in 0.1s'; exit 3`)
	options := h.options(bazelCheck("tests", "//a:t", "STAGE_FULL"), registrypb.Stage_STAGE_FULL)
	options.ReceiptPath = filepath.Join(h.dir, "receipt.json")
	// Act
	code := h.run(options)
	directory, err := ExportReceipt(context.Background(), ArtifactOptions{Registry: options.Registry, Root: h.dir, ReceiptPath: options.ReceiptPath})
	if err != nil {
		t.Fatal(err)
	}
	record := receiptOf(t, options.ReceiptPath)
	relative, _ := filepath.Rel(h.dir, record.Checks[0].Log)
	output, err := os.ReadFile(filepath.Join(directory, relative))
	// Assert
	if code != ExitFailed || record.Result != "FAILED" || err != nil || !bytes.Contains(output, []byte("native assertion got 7 want 11")) {
		t.Fatalf("failed runner-to-artifact assertion was lost or verdict changed: code=%d state=%s error=%v output=%q", code, record.Result, err, output)
	}
}

func TestArtifactPreservesBoundedTruncationAndRelativeLogReference(t *testing.T) {
	// Arrange
	f := newArtifactFixture(t)
	bounded, err := RetainLog(f.run, "tests", strings.Repeat("x", int(maxRetainedLog)+1))
	artifactMust(t, err)
	relative, err := filepath.Rel(f.root, bounded)
	artifactMust(t, err)
	f.record.Checks[0].Log = relative
	f.write(t)
	original, err := os.ReadFile(bounded)
	artifactMust(t, err)
	// Act
	directory, err := f.export()
	artifactMust(t, err)
	copied, err := os.ReadFile(filepath.Join(directory, relative))
	// Assert
	if err != nil || !bytes.Equal(original, copied) || !bytes.Contains(copied, []byte("[truncated at")) || len(copied) > int(maxRetainedLog)+128 {
		t.Fatalf("bounded log or explicit cut changed in export: error=%v bytes=%d", err, len(copied))
	}
}

func TestArtifactRefusesReceiptOutsideWorkspace(t *testing.T) {
	// Arrange
	f := newArtifactFixture(t)
	outside := filepath.Join(t.TempDir(), "receipt.json")
	artifactMust(t, WriteReceipt(outside, &f.record))
	// Act
	_, err := ExportReceipt(context.Background(), ArtifactOptions{Registry: f.registry, Root: f.root, ReceiptPath: outside})
	// Assert
	if err == nil {
		t.Fatal("outside receipt was accepted")
	}
}

func TestArtifactRefusesSymlinkReceipt(t *testing.T) {
	// Arrange
	f := newArtifactFixture(t)
	artifactMust(t, os.Rename(f.receipt, f.receipt+"-real"))
	artifactMust(t, os.Symlink(f.receipt+"-real", f.receipt))
	// Act
	_, err := f.export()
	// Assert
	if err == nil || !strings.Contains(err.Error(), "not regular") {
		t.Fatalf("symbolic receipt was not refused: %v", err)
	}
}

func TestArtifactRefusesSymlinkReceiptParent(t *testing.T) {
	// Arrange
	f := newArtifactFixture(t)
	alias := filepath.Join(f.root, "linked")
	artifactMust(t, os.Symlink(f.root, alias))
	// Act
	_, err := ExportReceipt(context.Background(), ArtifactOptions{Registry: f.registry, Root: f.root, ReceiptPath: filepath.Join(alias, "receipt.json")})
	// Assert
	if err == nil || !strings.Contains(err.Error(), "not a regular directory") {
		t.Fatalf("symbolic receipt parent was not refused: %v", err)
	}
}

func TestArtifactRefusesMixedOwnedRuns(t *testing.T) {
	// Arrange
	f := newArtifactFixture(t)
	otherRun, err := newLogDirectory(f.receipt)
	artifactMust(t, err)
	otherLog, err := RetainLog(otherRun, "other", "second invocation")
	artifactMust(t, err)
	f.record.Checks = append(f.record.Checks, Result{Check: "other", State: Failed, Log: otherLog})
	f.write(t)
	// Act
	_, err = f.export()
	// Assert
	if err == nil || !strings.Contains(err.Error(), "multiple log runs") {
		t.Fatalf("mixed invocation logs were not refused: %v", err)
	}
}
