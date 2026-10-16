package master

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
)

type embedFileSystem struct {
	http.FileSystem
}

func (e embedFileSystem) Exists(prefix string, path string) bool {
	_, err := e.Open(path)
	return err == nil
}
func EmbedFolder(fsEmbed embed.FS, targetPath string) static.ServeFileSystem {
	fsys, err := fs.Sub(fsEmbed, targetPath)
	if err != nil {
		panic(err)
	}
	return embedFileSystem{
		FileSystem: http.FS(fsys),
	}
}

func HandleStaticFile(f embed.FS, router *gin.Engine) {
	root := EmbedFolder(f, "out")
	staticServer := static.Serve("/", root)

	// Keep the exported HTML shell fresh while allowing hashed Next.js assets
	// to be cached. This prevents a browser from combining an old document with
	// a new build's asset manifest after an upgrade.
	serve := func(c *gin.Context, requestPath string) bool {
		if !root.Exists("/", requestPath) {
			return false
		}

		if strings.HasPrefix(requestPath, "/_next/static/") {
			c.Header("Cache-Control", "public, max-age=31536000, immutable")
		} else if strings.HasSuffix(requestPath, ".html") {
			c.Header("Cache-Control", "no-cache, must-revalidate")
		}

		c.Request.URL.Path = requestPath
		staticServer(c)
		return true
	}

	router.Use(func(c *gin.Context) {
		if c.Request.Method == http.MethodGet && serve(c, c.Request.URL.Path) {
			return
		}
		c.Next()
	})

	router.NoRoute(func(c *gin.Context) {
		if c.Request.Method != http.MethodGet ||
			strings.ContainsRune(c.Request.URL.Path, '.') ||
			strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.Status(http.StatusNotFound)
			return
		}

		// Next.js static export writes `/clients.html`, while browsers and
		// reverse proxies commonly request `/clients/` on a refresh. Normalize
		// both forms to the same exported document.
		path := strings.TrimSuffix(c.Request.URL.Path, "/")
		if path == "" {
			path = "/index.html"
		} else {
			path += ".html"
		}
		if !serve(c, path) {
			c.Status(http.StatusNotFound)
		}
	})
}
