//go:build tools

// Package analysis records the analyzer modules, so go.mod keeps them.
package analysis

import (
	_ "github.com/mibk/dupl/syntax"
	_ "golang.org/x/tools/go/analysis"
	_ "honnef.co/go/tools/staticcheck/sa1000"
)
