package noentityresponse

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// TestAnalyzer exercises the analyzer against lightweight stub packages (T12).
// It covers direct entities, pointers and slices nested through gin.H composite
// literals, json.Marshal, and the AbortWithStatusJSON / IndentedJSON responders,
// plus negative cases (dto.*Response, list mappers, scalars, safe types) that
// must not be flagged.
func TestAnalyzer(t *testing.T) {
	// Point the analyzer at the stub packages under testdata/src.
	if err := Analyzer.Flags.Set("modelpkg", "example.com/model"); err != nil {
		t.Fatalf("set modelpkg: %v", err)
	}
	if err := Analyzer.Flags.Set("ginpkg", "example.com/gin"); err != nil {
		t.Fatalf("set ginpkg: %v", err)
	}
	t.Cleanup(func() {
		_ = Analyzer.Flags.Set("modelpkg", "github.com/Laisky/one-api/model")
		_ = Analyzer.Flags.Set("ginpkg", "github.com/gin-gonic/gin")
	})

	analysistest.Run(t, analysistest.TestData(), Analyzer, "example.com/handlers")
}
