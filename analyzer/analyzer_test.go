package analyzer

import (
	"math"
	"path/filepath"
	"testing"
)

func TestAnalyzeFile_NotGoingHome(t *testing.T) {
	// Path to the test file relative to the test directory
	testFile := filepath.Join("..", "01 - Not Going Home.flac")

	result, err := AnalyzeFile(testFile)
	if err != nil {
		t.Fatalf("AnalyzeFile failed: %v", err)
	}

	// Expected values from rekordbox
	expectedBPM := 125.0
	expectedBars := 110.4

	// Check BPM with a tolerance of 0.5 BPM
	bpmTolerance := 0.5
	if math.Abs(result.BPM-expectedBPM) > bpmTolerance {
		t.Errorf("BPM mismatch: got %.2f, expected %.2f (tolerance: %.2f)",
			result.BPM, expectedBPM, bpmTolerance)
	}

	// Check bars with a tolerance of 2 bars (since beat detection can vary slightly)
	barsTolerance := 2.0
	actualBars := result.Bars()
	if math.Abs(actualBars-expectedBars) > barsTolerance {
		t.Errorf("Bars mismatch: got %.1f, expected %.1f (tolerance: %.1f)",
			actualBars, expectedBars, barsTolerance)
	}

	t.Logf("Analysis results:")
	t.Logf("  BPM: %.2f (expected: %.2f)", result.BPM, expectedBPM)
	t.Logf("  Bars: %.1f (expected: %.1f)", actualBars, expectedBars)
	t.Logf("  Total beats: %d", len(result.Beats))
	t.Logf("  Duration: %.2f seconds", result.Duration)
	t.Logf("  Sample rate: %d Hz", result.SampleRate)

	// Log first few beats for debugging
	if len(result.Beats) > 5 {
		t.Logf("  First 5 beats (seconds): %.3f, %.3f, %.3f, %.3f, %.3f",
			result.Beats[0], result.Beats[1], result.Beats[2], result.Beats[3], result.Beats[4])
	}
}

func TestVersion(t *testing.T) {
	version := Version()
	if version == "" {
		t.Error("Version returned empty string")
	}
	t.Logf("Analyzer version: %s", version)
}

func TestAnalyzeFile_InvalidFile(t *testing.T) {
	_, err := AnalyzeFile("/nonexistent/file.flac")
	if err == nil {
		t.Error("Expected error for nonexistent file, got nil")
	}
	t.Logf("Got expected error: %v", err)
}
