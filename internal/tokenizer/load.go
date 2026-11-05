package tiktoken

import (
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/google/uuid"
)

type BpeLoader interface {
	LoadTiktokenBpe(tiktokenBpeFile string) (map[string]int, error)
}

func readFile(blobpath string) ([]byte, error) {
	if !strings.HasPrefix(blobpath, "http://") && !strings.HasPrefix(blobpath, "https://") {
		file, err := os.Open(blobpath)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		defer file.Close()
		contents, err := io.ReadAll(file)
		return contents, errors.WithStack(err)
	}
	// avoiding blobfile for public files helps avoid auth issues, like MFA prompts
	resp, err := http.Get(blobpath)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer resp.Body.Close()
	contents, err := io.ReadAll(resp.Body)
	return contents, errors.WithStack(err)
}

func readFileCached(blobpath string) ([]byte, error) {
	if blobpath == "" {
		return nil, errors.New("blobpath cannot be empty")
	}

	cacheDir := strings.TrimSpace(os.Getenv("TIKTOKEN_CACHE_DIR"))
	if cacheDir == "" {
		cacheDir = strings.TrimSpace(os.Getenv("DATA_GYM_CACHE_DIR"))
	}

	if cacheDir == "" {
		cacheDir = filepath.Join(os.TempDir(), "data-gym-cache")
	}

	cacheKey := fmt.Sprintf("%x", sha1.Sum([]byte(blobpath)))
	cachePath := filepath.Join(cacheDir, cacheKey)
	if _, err := os.Stat(cachePath); err == nil {
		contents, err := os.ReadFile(cachePath)
		return contents, errors.WithStack(err)
	}

	contents, err := readFile(blobpath)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(cacheDir, os.ModePerm); err != nil {
		return nil, errors.Wrap(err, "failed to create cache directory")
	}

	tmpFilename := fmt.Sprintf("%s.%s.tmp", cachePath, uuid.NewString())
	if err := os.WriteFile(tmpFilename, contents, 0644); err != nil {
		return nil, errors.Wrap(err, "failed to write temporary file")
	}

	if err := os.Rename(tmpFilename, cachePath); err != nil {
		os.Remove(tmpFilename) // Clean up on failure
		return nil, errors.Wrap(err, "failed to rename temporary file")
	}

	return contents, nil
}

func loadTiktokenBpe(tiktokenBpeFile string) (map[string]int, error) {
	contents, err := readFileCached(tiktokenBpeFile)
	if err != nil {
		return nil, err
	}

	bpeRanks := make(map[string]int)
	for _, line := range strings.Split(string(contents), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, " ")
		token, err := base64.StdEncoding.DecodeString(parts[0])
		if err != nil {
			return nil, errors.WithStack(err)
		}
		rank, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, errors.WithStack(err)
		}
		bpeRanks[string(token)] = rank
	}
	return bpeRanks, nil
}

type defaultBpeLoader struct{}

func (l *defaultBpeLoader) LoadTiktokenBpe(tiktokenBpeFile string) (map[string]int, error) {
	return loadTiktokenBpe(tiktokenBpeFile)
}

func NewDefaultBpeLoader() BpeLoader {
	return &defaultBpeLoader{}
}
