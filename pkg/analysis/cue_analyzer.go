// Package analysis provides cue point detection using ML features.
package analysis

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
)

// CuePoint represents a detected cue point in a track.
type CuePoint struct {
	Time       float64 `json:"time"`       // Time in seconds
	Type       string  `json:"type"`       // Type: intro, drop, breakdown, buildup, outro, section
	Confidence float64 `json:"confidence"` // Confidence score 0-1
	Name       string  `json:"name"`       // Display name
}

// CueAnalyzeOut contains the cue detection results.
type CueAnalyzeOut struct {
	CuePoints []CuePoint `json:"cue_points"`
}

// CueAnalyzer performs cue point detection using SampleCNN features.
type CueAnalyzer struct {
	uvPath     string
	scriptPath string
}

// NewCueAnalyzer creates a new cue point analyzer.
func NewCueAnalyzer() (*CueAnalyzer, error) {
	// Get the directory containing this source file
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return nil, fmt.Errorf("failed to get current file path")
	}
	// Go up from pkg/analysis to project root
	baseDir := filepath.Dir(filepath.Dir(filepath.Dir(currentFile)))
	scriptPath := filepath.Join(baseDir, "cue_detector.py")

	// Verify uv is available
	uvPath, err := exec.LookPath("uv")
	if err != nil {
		return nil, fmt.Errorf("uv not found - install with: curl -LsSf https://astral.sh/uv/install.sh | sh")
	}

	return &CueAnalyzer{
		uvPath:     uvPath,
		scriptPath: scriptPath,
	}, nil
}

// AnalyzeFile detects cue points in an audio file.
func (a *CueAnalyzer) AnalyzeFile(audioPath string, maxCues int, minDistance float64) (*CueAnalyzeOut, error) {
	// Convert to absolute path if relative
	if !filepath.IsAbs(audioPath) {
		absPath, err := filepath.Abs(audioPath)
		if err == nil {
			audioPath = absPath
		}
	}

	// Build command
	cmd := exec.Command(
		a.uvPath,
		"run",
		a.scriptPath,
		"--json",
		"--max-cues", fmt.Sprintf("%d", maxCues),
		"--min-distance", fmt.Sprintf("%.1f", minDistance),
		audioPath,
	)

	// Run and capture output
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := string(exitErr.Stderr)
			if stderr == "" {
				stderr = "unknown error"
			}
			return nil, fmt.Errorf("cue detection failed: %s", stderr)
		}
		return nil, fmt.Errorf("cue detection failed: %w", err)
	}

	// Parse JSON output
	var result CueAnalyzeOut
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse cue detection output: %w", err)
	}

	return &result, nil
}
