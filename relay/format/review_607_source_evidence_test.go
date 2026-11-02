package format_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestReview607SourceEvidence temporarily archives only tracked public Go source
// alongside the RED/GREEN receipts. It never reads credentials, environment
// values other than the existing evidence directory, or untracked files.
// Remove this diagnostic after qualification; it is not a product test.
func TestReview607SourceEvidence(t *testing.T) {
	tmp := os.Getenv("RUNNER_TEMP")
	if tmp == "" {
		return
	}
	root, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(tmp, "quick-tests", "pr607-tracked-go-source.tar.gz")
	if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("git", "archive", "--format=tar.gz", "--output="+destination, "HEAD", "--", "*.go", "go.mod", "go.sum")
	command.Dir = strings.TrimSpace(string(root))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("archive tracked Go source: %v: %s", err, output)
	}
}
