// Command noentityresponse runs the noentityresponse analyzer as a standalone
// checker. Invoke it over the repo (e.g. `noentityresponse ./...`) or as a
// `go vet -vettool` binary. It exits non-zero when a raw management-API entity
// reaches a JSON boundary.
package main

import (
	"golang.org/x/tools/go/analysis/singlechecker"

	"github.com/Laisky/one-api/tools/analyzers/noentityresponse"
)

func main() {
	singlechecker.Main(noentityresponse.Analyzer)
}
