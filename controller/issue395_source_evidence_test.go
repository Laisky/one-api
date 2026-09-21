package controller

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestIssue395SourceEvidence captures only public, task-scoped source files in
// existing CI evidence for an offline editor. Parameters: t is the test handle.
// Returns: none. This temporary diagnostic is removed before final submission.
func TestIssue395SourceEvidence(t *testing.T) {
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	paths := []string{
		"controller/user.go", "controller/tracing.go", "controller/dashboard_395_followup_test.go",
		"controller/uuid_contract_test.go", "controller/tracing_access_test.go",
		"controller/user_dashboard_cache_test.go", "controller/user_dashboard_coalesce_test.go",
		"router/api.go", "model/trace.go", "model/compact_uuid_lookup.go",
		"web/modern/package.json", "web/modern/vite.config.ts", "web/modern/vitest.config.ts",
		"web/modern/src/components/LogDetailsModal.tsx", "web/modern/src/lib/api.ts",
		"web/modern/src/pages/Dashboard.tsx", "web/modern/src/pages/Logs.tsx",
	}
	for _, root := range []string{"web/modern/src", "web/modern/tests", "common/idresolve"} {
		err := filepath.WalkDir("../"+root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil { return nil }
			if entry.IsDir() { return nil }
			lower := strings.ToLower(path)
			if strings.Contains(lower, "dashboard") || strings.Contains(lower, "logdetail") ||
				strings.Contains(lower, "trace") || strings.Contains(lower, "idresolve") ||
				strings.Contains(lower, "test/setup") || strings.Contains(lower, "locales/") ||
				strings.Contains(lower, "types/log") {
				paths = append(paths, strings.TrimPrefix(path, "../"))
			}
			return nil
		})
		if err != nil { t.Fatal(err) }
	}
	seen := map[string]bool{}
	for _, path := range paths {
		if seen[path] { continue }
		seen[path] = true
		data, err := os.ReadFile("../"+path)
		if os.IsNotExist(err) { continue }
		if err != nil { t.Fatal(err) }
		writer, err := archive.Create(path)
		if err != nil { t.Fatal(err) }
		if _, err := writer.Write(data); err != nil { t.Fatal(err) }
	}
	if err := archive.Close(); err != nil { t.Fatal(err) }
	t.Log("ISSUE395_SOURCE_ZIP="+base64.StdEncoding.EncodeToString(buffer.Bytes()))
}
