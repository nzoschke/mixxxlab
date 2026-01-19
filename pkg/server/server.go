// Package server provides the Echo web server for the beat grid visualizer.
package server

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// Track represents a track in the music library.
type Track struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	HasJSON  bool   `json:"has_json"`
	JSONPath string `json:"json_path,omitempty"`
}

// Run starts the web server on :8080.
func Run() error {
	e := echo.New()
	e.HideBanner = true

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	// Routes
	e.GET("/", serveIndex)
	e.Static("/src", "src")
	e.GET("/api/music", listMusic)
	e.GET("/api/music/*", serveMusic)

	return e.Start(":8080")
}

// serveIndex serves the main index.html page.
func serveIndex(c echo.Context) error {
	return c.File("src/index.html")
}

// listMusic returns a list of all tracks in the music directory.
func listMusic(c echo.Context) error {
	var tracks []Track

	err := filepath.WalkDir("music", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if !isAudioFile(ext) {
			return nil
		}

		// Convert path to URL path (relative to music/)
		relPath := strings.TrimPrefix(path, "music/")
		jsonPath := strings.TrimSuffix(path, ext) + ".json"

		track := Track{
			Name: strings.TrimSuffix(filepath.Base(path), ext),
			Path: relPath,
		}

		// Check if JSON sidecar exists
		if _, err := os.Stat(jsonPath); err == nil {
			track.HasJSON = true
			track.JSONPath = strings.TrimPrefix(jsonPath, "music/")
		}

		tracks = append(tracks, track)
		return nil
	})

	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, tracks)
}

// serveMusic serves audio files and JSON analysis files from the music directory.
func serveMusic(c echo.Context) error {
	// Get the path after /api/music/ and URL-decode it
	path := c.Param("*")
	decodedPath, err := url.PathUnescape(path)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid path encoding")
	}
	fullPath := filepath.Join("music", decodedPath)

	// Security: prevent directory traversal
	if strings.Contains(decodedPath, "..") {
		return echo.NewHTTPError(http.StatusForbidden, "invalid path")
	}

	// Check file exists
	info, err := os.Stat(fullPath)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "file not found")
	}
	if info.IsDir() {
		return echo.NewHTTPError(http.StatusForbidden, "cannot serve directory")
	}

	// Only serve allowed file types
	ext := strings.ToLower(filepath.Ext(decodedPath))
	if isAudioFile(ext) {
		return c.File(fullPath)
	}
	if ext == ".json" {
		// Read and parse JSON to validate it
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		var analysis map[string]any
		if err := json.Unmarshal(data, &analysis); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "invalid JSON")
		}
		return c.JSON(http.StatusOK, analysis)
	}
	return echo.NewHTTPError(http.StatusForbidden, "file type not allowed")
}

// isAudioFile returns true if the extension is a supported audio format.
func isAudioFile(ext string) bool {
	switch ext {
	case ".mp3", ".m4a", ".aac", ".wav", ".flac", ".ogg", ".aiff":
		return true
	default:
		return false
	}
}
