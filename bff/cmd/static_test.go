// Package cmd_test holds the standing witness for spec 006 FR-020: automatic toolchain discovery
// must not change how Go links. On a host with a compiler, detection once made these binaries
// dynamic, which a compiler-free host could not reproduce.
package cmd_test

import (
	"debug/elf"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// interpreter reports whether the file a runfiles-rooted variable names is dynamically linked. A Go
// test starts in its package, so the path is resolved from the runfiles root.
func interpreter(t *testing.T, name string) bool {
	t.Helper()
	path := filepath.Join(os.Getenv("TEST_SRCDIR"), os.Getenv("TEST_WORKSPACE"), os.Getenv(name))
	file, err := elf.Open(path)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	defer func() { _ = file.Close() }()
	for _, program := range file.Progs {
		if program.Type == elf.PT_INTERP {
			return true
		}
	}
	return false
}

func TestServerBinariesAreStatic(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("ELF only")
	}
	if !interpreter(t, "DYNAMIC_CONTROL") {
		t.Fatal("the control has an interpreter, so this check cannot see one")
	}
	for _, name := range []string{"BFF", "BFF_DEV"} {
		if interpreter(t, name) {
			t.Errorf("%s links dynamically: a host tool changed the build", name)
		}
	}
}
