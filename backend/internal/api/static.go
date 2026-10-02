package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

// RegisterStatic serves a built frontend (dist directory) as a single-page
// application. API and internal routes are never shadowed: unknown /api or
// /internal paths return JSON 404, everything else falls back to index.html so
// client-side routing works on refresh.
//
// Returns false when dir does not exist, so the caller can log accordingly.
func RegisterStatic(r *gin.Engine, dir string) bool {
	if dir == "" {
		return false
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false
	}
	indexPath := filepath.Join(dir, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		return false
	}

	// Hashed build assets and other top-level files.
	r.Static("/assets", filepath.Join(dir, "assets"))
	r.StaticFile("/favicon.ico", filepath.Join(dir, "favicon.ico"))
	r.StaticFile("/vite.svg", filepath.Join(dir, "vite.svg"))

	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/internal/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		// Serve a real file if it exists (e.g. robots.txt, manifest.json).
		cleaned := filepath.Clean("/" + path)
		candidate := filepath.Join(dir, cleaned)
		if rel, err := filepath.Rel(dir, candidate); err == nil && !strings.HasPrefix(rel, "..") {
			if fi, err := os.Stat(candidate); err == nil && !fi.IsDir() {
				c.File(candidate)
				return
			}
		}
		c.File(indexPath)
	})
	return true
}
