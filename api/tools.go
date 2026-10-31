//go:build tools

// Package api records what generated code imports. Generated files are not in the source tree, so
// `go mod tidy` cannot see these requirements any other way.
package api

import (
	_ "buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	_ "connectrpc.com/connect/v2"
)
