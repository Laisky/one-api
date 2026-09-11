package common

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/gin-contrib/static"
)

// Credit: https://github.com/gin-contrib/static/issues/19

type embedFileSystem struct {
	http.FileSystem
}

// Exists reports whether the given path can be opened within the embedded filesystem.
func (e embedFileSystem) Exists(prefix string, path string) bool {
	_, err := e.Open(path)
	return err == nil
}

// EmbedFolder exposes a subset of the embedded filesystem as a gin static file system rooted at targetPath.
// It panics when the requested directory does not exist in the supplied embed.FS.
func EmbedFolder(fsEmbed embed.FS, targetPath string) static.ServeFileSystem {
	efs, err := fs.Sub(fsEmbed, targetPath)
	if err != nil {
		panic(err)
	}
	return embedFileSystem{
		FileSystem: http.FS(efs),
	}
}
