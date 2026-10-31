// Command baseline writes the declared compatibility baseline to a directory.
//
//	baseline -repo <root> -marker <file> -schema <dir> -out <dir>
//
// It prints `baseline <hash> files=<n>` or `bootstrap`, and exits 4 when no comparison is possible.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/selamy-labs/bazel-protobuf-bff-template/quality/proto"
)

func main() {
	reader := baseline.Reader{}
	out := flag.String("out", "", "directory to write the baseline schema to")
	flag.StringVar(&reader.Repo, "repo", ".", "repository root")
	flag.StringVar(&reader.Marker, "marker", "", "marker file, relative to the root")
	flag.StringVar(&reader.Schema, "schema", "", "schema directory, relative to the root")
	flag.Parse()
	if *out == "" || reader.Marker == "" || reader.Schema == "" || flag.NArg() != 0 {
		flag.Usage()
		os.Exit(2)
	}
	result, err := reader.Materialize(*out)
	switch {
	case errors.Is(err, baseline.ErrUnavailable):
		fmt.Fprintf(os.Stderr, "proto-breaking: %v\n", err)
		os.Exit(4)
	case err != nil:
		fmt.Fprintf(os.Stderr, "proto-breaking: %v\n", err)
		os.Exit(1)
	case result.Bootstrap:
		fmt.Println(baseline.BootstrapMarker)
	default:
		fmt.Printf("baseline %s files=%d\n", result.Revision, result.Files)
	}
}
