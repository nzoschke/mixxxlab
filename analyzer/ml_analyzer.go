// Package analyzer provides Go bindings for beat detection.
// This file provides ML-based beat detection using a Python subprocess.
package analyzer

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// MLAnalyzeOut contains the analysis results from ML beat detection.
type MLAnalyzeOut struct {
	BPM         float64   `json:"bpm"`
	Beats       []float64 `json:"beats"`
	SampleRate  int       `json:"sample_rate"`
	Duration    float64   `json:"duration"`
	TotalFrames int64     `json:"total_frames"`
	NumBeats    int       `json:"num_beats"`
	Bars        float64   `json:"bars"`
}

// MLAnalyzer performs beat detection using TensorFlow models via Python.
type MLAnalyzer struct {
	pythonPath string
	scriptPath string
	modelPath  string
}

// rekordboxModelsPath is the path to rekordbox's bundled ML models.
const rekordboxModelsPathML = "/Applications/rekordbox 7/rekordbox.app/Contents/Resources/models"

// NewMLAnalyzer creates a new ML-based beat analyzer.
// It uses uv to run the beat_detector.py script with PEP 723 inline dependencies.
func NewMLAnalyzer() (*MLAnalyzer, error) {
	// Get the directory containing this source file
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return nil, fmt.Errorf("failed to get current file path")
	}
	baseDir := filepath.Dir(filepath.Dir(currentFile))

	// Construct paths
	scriptPath := filepath.Join(baseDir, "beat_detector.py")
	modelPath := filepath.Join(rekordboxModelsPathML, "detect_beat", "model8")

	// Verify model exists
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("model not found at %s - rekordbox 7 must be installed", modelPath)
	}

	// Verify uv is available
	uvPath, err := exec.LookPath("uv")
	if err != nil {
		return nil, fmt.Errorf("uv not found - install with: curl -LsSf https://astral.sh/uv/install.sh | sh")
	}

	return &MLAnalyzer{
		pythonPath: uvPath,
		scriptPath: scriptPath,
		modelPath:  modelPath,
	}, nil
}

// NewMLAnalyzerWithModel creates a new ML-based beat analyzer with a specific model.
func NewMLAnalyzerWithModel(modelPath string) (*MLAnalyzer, error) {
	analyzer, err := NewMLAnalyzer()
	if err != nil {
		return nil, err
	}
	analyzer.modelPath = modelPath
	return analyzer, nil
}

// AnalyzeFile analyzes an audio file using ML-based beat detection.
func (a *MLAnalyzer) AnalyzeFile(audioPath string) (*MLAnalyzeOut, error) {
	// Convert to absolute path if relative
	if !filepath.IsAbs(audioPath) {
		absPath, err := filepath.Abs(audioPath)
		if err == nil {
			audioPath = absPath
		}
	}

	// Build command using uv run with PEP 723 script
	cmd := exec.Command(
		a.pythonPath, // uv
		"run",
		a.scriptPath,
		"--model", a.modelPath,
		"--json",
		audioPath,
	)

	// Run and capture output
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := string(exitErr.Stderr)
			// Filter out common warnings from stderr
			if stderr == "" {
				stderr = "unknown error"
			}
			return nil, fmt.Errorf("beat detection failed: %s", stderr)
		}
		return nil, fmt.Errorf("beat detection failed: %w", err)
	}

	// Parse JSON output
	var result MLAnalyzeOut
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse beat detection output: %w", err)
	}

	return &result, nil
}
