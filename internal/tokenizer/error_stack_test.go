package tiktoken

import (
	"crypto/sha1"
	stderrors "errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	stackerrors "github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// TestTokenizerFileErrorsCarryStacks checks that external failures retain their messages, types, and origin stacks.
func TestTokenizerFileErrorsCarryStacks(t *testing.T) {
	type stackTracer interface{ StackTrace() stackerrors.StackTrace }
	tests := []struct {
		name   string
		invoke func(t *testing.T) (error, string, error)
		frame  string
	}{
		{"open", func(t *testing.T) (error, string, error) {
			path := filepath.Join(t.TempDir(), "missing")
			_, original := os.Open(path)
			_, err := readFile(path)
			return err, original.Error(), os.ErrNotExist
		}, "readFile"},
		{"http", func(t *testing.T) (error, string, error) {
			_, err := readFile("http://[::1")
			return err, `parse "http://[::1": missing ']' in host`, nil
		}, "readFile"},
		{"truncated response", func(t *testing.T) (error, string, error) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Length", "5")
				if _, err := io.WriteString(w, "abc"); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			data, err := readFile(server.URL)
			require.Equal(t, "abc", string(data))
			return err, io.ErrUnexpectedEOF.Error(), io.ErrUnexpectedEOF
		}, "readFile"},
		{"cached read", func(t *testing.T) (error, string, error) {
			dir := t.TempDir()
			t.Setenv("TIKTOKEN_CACHE_DIR", dir)
			path := "some-path"
			cache := filepath.Join(dir, fmt.Sprintf("%x", sha1.Sum([]byte(path))))
			require.NoError(t, os.Mkdir(cache, 0755))
			_, original := os.ReadFile(cache)
			_, err := readFileCached(path)
			return err, original.Error(), nil
		}, "readFileCached"},
		{"mkdir", func(t *testing.T) (error, string, error) {
			dir := t.TempDir()
			input := filepath.Join(dir, "input")
			cache := filepath.Join(dir, "file")
			require.NoError(t, os.WriteFile(input, []byte("YQ== 0"), 0644))
			require.NoError(t, os.WriteFile(cache, nil, 0644))
			t.Setenv("TIKTOKEN_CACHE_DIR", cache)
			original := os.MkdirAll(cache, os.ModePerm)
			_, err := readFileCached(input)
			return err, "failed to create cache directory: " + original.Error(), syscall.ENOTDIR
		}, "readFileCached"},
		{"base64", func(t *testing.T) (error, string, error) {
			dir := t.TempDir()
			t.Setenv("TIKTOKEN_CACHE_DIR", filepath.Join(dir, "cache"))
			path := filepath.Join(dir, "input")
			require.NoError(t, os.WriteFile(path, []byte("%%% 0"), 0644))
			_, err := loadTiktokenBpe(path)
			return err, "illegal base64 data at input byte 0", nil
		}, "loadTiktokenBpe"},
		{"rank", func(t *testing.T) (error, string, error) {
			dir := t.TempDir()
			t.Setenv("TIKTOKEN_CACHE_DIR", filepath.Join(dir, "cache"))
			path := filepath.Join(dir, "input")
			require.NoError(t, os.WriteFile(path, []byte("YQ== nope"), 0644))
			_, err := loadTiktokenBpe(path)
			return err, `strconv.Atoi: parsing "nope": invalid syntax`, nil
		}, "loadTiktokenBpe"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err, message, cause := tc.invoke(t)
			require.EqualError(t, err, message)
			if cause != nil {
				require.ErrorIs(t, err, cause)
			}
			var trace stackTracer
			require.True(t, stderrors.As(err, &trace))
			require.Contains(t, fmt.Sprintf("%+v", err), tc.frame)
		})
	}
}

type cyclicLoaderError struct{}

// Error returns the cyclic loader failure message.
func (e *cyclicLoaderError) Error() string { return "cyclic loader failure" }

// Unwrap returns the same error to model an invalid cyclic chain.
func (e *cyclicLoaderError) Unwrap() error { return e }

type failingBpeLoader struct{ err error }

// LoadTiktokenBpe returns the configured loader failure for any path.
func (l failingBpeLoader) LoadTiktokenBpe(string) (map[string]int, error) { return nil, l.err }

// TestTokenizerLoaderErrorsCarryStacks checks failures supplied by external loaders retain identity and acquire an origin stack.
func TestTokenizerLoaderErrorsCarryStacks(t *testing.T) {
	original := bpeLoader
	defer func() { bpeLoader = original }()
	cause := stderrors.New("external loader failure")
	bpeLoader = failingBpeLoader{err: cause}
	tests := []struct {
		name   string
		invoke func() (*Encoding, error)
	}{
		{"o200k_base", o200k_base}, {"cl100k_base", cl100k_base}, {"p50k_edit", p50k_edit}, {"p50k_base", p50k_base}, {"r50k_base", r50k_base},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.invoke()
			require.EqualError(t, err, cause.Error())
			require.ErrorIs(t, err, cause)
			require.Contains(t, fmt.Sprintf("%+v", err), tc.name)
		})
	}
	t.Run("wrapped stack", func(t *testing.T) {
		stacked := stackerrors.WithStack(cause)
		bpeLoader = failingBpeLoader{err: fmt.Errorf("loader: %w", stacked)}
		_, err := o200k_base()
		require.EqualError(t, err, "loader: external loader failure")
		require.ErrorIs(t, err, cause)
		require.Contains(t, fmt.Sprintf("%+v", err), "o200k_base")
	})
	t.Run("cyclic error", func(t *testing.T) {
		bpeLoader = failingBpeLoader{err: &cyclicLoaderError{}}
		_, err := o200k_base()
		require.EqualError(t, err, "cyclic loader failure")
		require.Contains(t, fmt.Sprintf("%+v", err), "o200k_base")
	})
	t.Run("preserve existing stack", func(t *testing.T) {
		stacked := stackerrors.WithStack(cause)
		bpeLoader = failingBpeLoader{err: stacked}
		_, err := o200k_base()
		require.Same(t, stacked, err)
	})
}
