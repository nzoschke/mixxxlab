// Package analysis provides beat detection and audio analysis.
// This file provides music structure analysis using SongFormer via Python subprocess.
package analysis

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
)

// SongFormerOutput contains the analysis results from SongFormer.
type SongFormerOutput struct {
	Phrases []Phrase `json:"phrases"`
}

// SongFormerAnalyzer performs music structure analysis using SongFormer.
type SongFormerAnalyzer struct {
	uvPath     string
	scriptPath string
	useFallback bool
}

// NewSongFormerAnalyzer creates a new SongFormer-based structure analyzer.
// It uses uv to run the songformer_analyzer.py script with PEP 723 inline dependencies.
func NewSongFormerAnalyzer() (*SongFormerAnalyzer, error) {
	// Get the directory containing this source file
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return nil, fmt.Errorf("failed to get current file path")
	}
	// Go up from pkg/analysis to project root
	baseDir := filepath.Dir(filepath.Dir(filepath.Dir(currentFile)))

	// Construct paths
	scriptPath := filepath.Join(baseDir, "songformer_analyzer.py")

	// Verify uv is available
	uvPath, err := exec.LookPath("uv")
	if err != nil {
		return nil, fmt.Errorf("uv not found - install with: curl -LsSf https://astral.sh/uv/install.sh | sh")
	}

	return &SongFormerAnalyzer{
		uvPath:     uvPath,
		scriptPath: scriptPath,
		useFallback: false,
	}, nil
}

// NewSongFormerAnalyzerFallback creates a SongFormer analyzer that uses the fallback
// energy-based method instead of the ML model.
func NewSongFormerAnalyzerFallback() (*SongFormerAnalyzer, error) {
	analyzer, err := NewSongFormerAnalyzer()
	if err != nil {
		return nil, err
	}
	analyzer.useFallback = true
	return analyzer, nil
}

// AnalyzeFile analyzes an audio file for music structure (phrases/sections).
func (s *SongFormerAnalyzer) AnalyzeFile(audioPath string) (*SongFormerOutput, error) {
	// Convert to absolute path if relative
	if !filepath.IsAbs(audioPath) {
		absPath, err := filepath.Abs(audioPath)
		if err == nil {
			audioPath = absPath
		}
	}

	// Build command using uv run with PEP 723 script
	args := []string{
		"run",
		s.scriptPath,
		"--json",
	}

	if s.useFallback {
		args = append(args, "--fallback")
	}

	args = append(args, audioPath)

	cmd := exec.Command(s.uvPath, args...)

	// Run and capture output
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := string(exitErr.Stderr)
			if stderr == "" {
				stderr = "unknown error"
			}
			return nil, fmt.Errorf("structure analysis failed: %s", stderr)
		}
		return nil, fmt.Errorf("structure analysis failed: %w", err)
	}

	// Parse JSON output
	var result SongFormerOutput
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse structure analysis output: %w", err)
	}

	return &result, nil
}
