package analysis

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMLAnalyzeFile(t *testing.T) {
	// Path to the test file from fixtures
	testFile := filepath.Join("fixtures", "Spoon_-_05_-_Revenge.mp3")

	// Skip if test file doesn't exist
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skip("Test audio file not found, skipping ML analyzer test")
	}

	analyzer, err := NewMLAnalyzer()
	if err != nil {
		t.Fatalf("NewMLAnalyzer failed: %v", err)
	}

	result, err := analyzer.AnalyzeFile(testFile)
	if err != nil {
		t.Fatalf("AnalyzeFile failed: %v", err)
	}

	// Basic sanity checks
	if result.BPM < 60 || result.BPM > 200 {
		t.Errorf("BPM out of reasonable range: %.2f", result.BPM)
	}

	if result.NumBeats == 0 {
		t.Error("No beats detected")
	}

	if result.Duration <= 0 {
		t.Error("Invalid duration")
	}

	t.Logf("ML Analysis results:")
	t.Logf("  BPM: %.2f", result.BPM)
	t.Logf("  Bars: %.1f", result.Bars)
	t.Logf("  Total beats: %d", result.NumBeats)
	t.Logf("  Duration: %.2f seconds", result.Duration)
	t.Logf("  Sample rate: %d Hz", result.SampleRate)

	// Log first few beats for debugging
	if len(result.Beats) > 5 {
		t.Logf("  First 5 beats (seconds): %.3f, %.3f, %.3f, %.3f, %.3f",
			result.Beats[0], result.Beats[1], result.Beats[2], result.Beats[3], result.Beats[4])
	}
}
