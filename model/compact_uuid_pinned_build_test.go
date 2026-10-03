package model

// Self-provisioning pinned-artifact builds and the require-aware skip gate (AUTO-013/014).
//
// The regression suite owns its own qualification machinery, deliberately: bugs are prevented
// from regressing by TEST CASES, and CI's only job is to run `go test` with database engines
// available. Two mechanisms make that real rather than aspirational:
//
//   - compactLiveSkipf turns every "environment not configured" skip into a FAILURE when
//     ONEAPI_REQUIRE_DB_BACKENDS=1 (the same contract the external-UUID suites already honor),
//     so a CI job that sets the flag cannot go green by silently skipping a live suite.
//   - resolvePinnedCompactBinary builds the pinned pre-migration artifacts from their recorded
//     commits ON DEMAND, inside the test process, with a per-ref disk cache. No CI build step,
//     no artifact registry: checking out the repo and running the tests is sufficient.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

const (
	// compactOldBinaryPinnedRef is the oldest supported rollback build: the last commit
	// containing the v3 external-UUID writer contract and no compact code. Recorded with its
	// checksum in docs/manuals/compact_uuid_acceptance_status.md.
	compactOldBinaryPinnedRef = "4dfec29ac18dd922ccfb1e91dffaba71d25fb48b"
	// compactPrevBinaryPinnedRef is the immediately preceding production build, which still
	// declares the ordinary owned-uuid model index (see the manifest role-baseline finding).
	compactPrevBinaryPinnedRef = "ed15a1443cb463f3fd01cf0dcc39da5b8f5d2def"
)

// compactLiveSkipf skips a live-gated test, or fails it when the environment claims to be a
// qualification run.
//
// ONEAPI_REQUIRE_DB_BACKENDS=1 is the no-skip guard, relocated from CI shell into the tests
// themselves: with the flag set, a missing DSN or unbuildable artifact is a red test, never a
// silent skip that lets the run go green without the evidence it exists to produce.
// Parameters:
//   - t: test handle.
//   - format: printf-style reason.
//   - args: reason arguments.
//
// Return values: none.
func compactLiveSkipf(t *testing.T, format string, args ...any) {
	t.Helper()
	if os.Getenv(requireDBBackendsEnv) == "1" {
		t.Fatalf("required live suite cannot run: "+format, args...)
	}
	t.Skipf(format, args...)
}

var (
	// compactPinnedBuildMu serializes artifact builds within one test process.
	compactPinnedBuildMu sync.Mutex
	// compactPinnedBuilt caches resolved artifact paths per ref within one test process.
	compactPinnedBuilt = map[string]string{}
)

// resolvePinnedCompactBinary returns a runnable pinned pre-migration artifact, building it
// from the recorded commit when no override is supplied.
//
// Resolution order: the override environment variable (an operator- or CI-supplied artifact,
// kept for reproducing against exotic builds), then a per-ref disk cache under the system
// temp directory (one build per machine, not per test), then a fresh build in a detached git
// worktree. The build stubs web/build/default/index.html because the pinned main.go embeds
// web/build/* and a bare checkout has no compiled frontend; the stub is enough for go:embed
// and irrelevant to the database contract under test. A shallow clone is handled by fetching
// the pinned commit explicitly before the worktree is added.
// Parameters:
//   - t: test handle.
//   - ref: pinned commit hash to build.
//   - overrideEnv: environment variable that may carry a prebuilt artifact path.
//
// Return values:
//   - string: absolute path to the runnable artifact.
func resolvePinnedCompactBinary(t *testing.T, ref string, overrideEnv string) string {
	t.Helper()
	if override := strings.TrimSpace(os.Getenv(overrideEnv)); override != "" {
		return override
	}

	compactPinnedBuildMu.Lock()
	defer compactPinnedBuildMu.Unlock()
	if path, done := compactPinnedBuilt[ref]; done {
		return path
	}

	cache := filepath.Join(os.TempDir(), "one-api-pinned-"+ref[:12])
	if info, err := os.Stat(cache); err == nil && info.Mode().IsRegular() && info.Size() > 0 {
		compactPinnedBuilt[ref] = cache
		return cache
	}

	root, err := compactGitOutput(t, "", "rev-parse", "--show-toplevel")
	if err != nil {
		compactLiveSkipf(t, "pinned artifact %s needs a git checkout: %v", ref[:12], err)
	}

	if _, err := compactGitOutput(t, root, "cat-file", "-e", ref+"^{commit}"); err != nil {
		// A shallow or partial clone does not carry the pinned commit; fetch exactly it.
		if _, err := compactGitOutput(t, root, "fetch", "--depth=1", "origin", ref); err != nil {
			compactLiveSkipf(t, "pinned commit %s is unavailable and cannot be fetched: %v", ref[:12], err)
		}
	}

	worktree := filepath.Join(t.TempDir(), "pinned-"+ref[:12])
	if _, err := compactGitOutput(t, root, "worktree", "add", "--detach", worktree, ref); err != nil {
		compactLiveSkipf(t, "add pinned worktree for %s: %v", ref[:12], err)
	}
	defer func() {
		_, _ = compactGitOutput(t, root, "worktree", "remove", "--force", worktree)
		_, _ = compactGitOutput(t, root, "worktree", "prune")
	}()

	stub := filepath.Join(worktree, "web", "build", "default")
	require.NoError(t, os.MkdirAll(stub, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(stub, "index.html"),
		[]byte("<!doctype html><title>pinned qualification stub</title>"), 0o644))

	build := exec.Command("go", "build", "-o", cache, ".")
	build.Dir = worktree
	if output, err := build.CombinedOutput(); err != nil {
		compactLiveSkipf(t, "build pinned artifact %s: %v\n%s", ref[:12], err, string(output))
	}
	compactPinnedBuilt[ref] = cache
	return cache
}

// compactGitOutput runs one git command and returns its trimmed output.
// Parameters:
//   - t: test handle.
//   - dir: working directory; empty means the test's current directory.
//   - args: git arguments.
//
// Return values:
//   - string: trimmed combined output.
//   - error: the command error with output attached for diagnosis.
func compactGitOutput(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	command := exec.Command("git", args...)
	if dir != "" {
		command.Dir = dir
	}
	output, err := command.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if err != nil {
		return text, errors.Wrapf(err, "git %s: %s", strings.Join(args, " "), text)
	}
	return text, nil
}
