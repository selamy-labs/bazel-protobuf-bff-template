package u1000

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/selamy-labs/bazel-protobuf-bff-template/quality/analysis/u1000/witness"
)

// The planted control for U1000 (spec 011 FR-002, FR-007), run with the pinned Go SDK this host declares: an
// unused unexported function is reported at its position, a used one and an exported one are not.
func TestPlantedUnusedFunctionIsReported(t *testing.T) {
	goroot := filepath.Dir(filepath.Join(os.Getenv("TEST_SRCDIR"), os.Getenv("GOROOT_FILE")))
	planted := filepath.Join(os.Getenv("TEST_SRCDIR"), os.Getenv("PLANTED"))
	testdata := filepath.Dir(filepath.Dir(filepath.Dir(planted)))
	witness.Run(t, goroot, testdata, []string{"planted.go:8: func dead is unused"})
}
