// Package u1000 reports what staticcheck's unused analyzer finds, so that nogo can refuse a build with dead code
// (spec 011 FR-007, T005). The analyzer computes which objects of a package are unreachable from anything exported
// or executed; staticcheck's own driver turns that result into reports, and nogo has no such driver, so this
// package is that step: one diagnostic per unused object, at its position.
package u1000

import (
	"go/token"

	"golang.org/x/tools/go/analysis"
	"honnef.co/go/tools/unused"
)

// Analyzer is U1000 as nogo can run it.
var Analyzer = &analysis.Analyzer{
	Name:     "U1000",
	Doc:      "unused code, from honnef.co/go/tools/unused",
	Requires: []*analysis.Analyzer{unused.Analyzer.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	result, ok := pass.ResultOf[unused.Analyzer.Analyzer].(unused.Result)
	if !ok {
		return nil, nil
	}
	for _, object := range result.Unused {
		pass.Report(analysis.Diagnostic{
			Pos:     position(pass, object.Position),
			Message: object.Kind + " " + object.Name + " is unused",
		})
	}
	return nil, nil
}

// position finds the token position of a reported object in the files of this pass.
func position(pass *analysis.Pass, at token.Position) token.Pos {
	for _, file := range pass.Files {
		f := pass.Fset.File(file.Pos())
		if f != nil && f.Name() == at.Filename && at.Offset >= 0 && at.Offset < f.Size() {
			return f.Pos(at.Offset)
		}
	}
	return token.NoPos
}
