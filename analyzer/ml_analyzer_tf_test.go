package analyzer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTFAnalyzer_LoadModel(t *testing.T) {
	analyzer, err := NewTFAnalyzer()
	if err != nil {
		t.Skip("TF Analyzer not available:", err)
	}
	defer analyzer.Close()

	t.Log("TensorFlow model loaded successfully")
}

func TestTFAnalyzer_AnalyzeFile(t *testing.T) {
	analyzer, err := NewTFAnalyzer()
	if err != nil {
		t.Skip("TF Analyzer not available:", err)
	}
	defer analyzer.Close()

	// Check if test audio exists
	audioPath := filepath.Join("fixtures", "Spoon_-_05_-_Revenge.mp3")
	if _, err := os.Stat(audioPath); os.IsNotExist(err) {
		t.Skip("Test audio not found")
	}

	result, err := analyzer.AnalyzeFile(audioPath)
	require.NoError(t, err)

	t.Logf("TF Analysis results:")
	t.Logf("  BPM: %.2f", result.BPM)
	t.Logf("  Bars: %.1f", result.Bars)
	t.Logf("  Total beats: %d", result.NumBeats)
	t.Logf("  Duration: %.2f seconds", result.Duration)
	t.Logf("  Sample rate: %d Hz", result.SampleRate)

	// Log first few beats
	if len(result.Beats) > 5 {
		t.Logf("  First 5 beats: %.3f, %.3f, %.3f, %.3f, %.3f",
			result.Beats[0], result.Beats[1], result.Beats[2], result.Beats[3], result.Beats[4])
	} else if len(result.Beats) > 0 {
		t.Logf("  Beats: %v", result.Beats)
	}
}

func TestLoadAudioMono(t *testing.T) {
	// Check if test audio exists
	audioPath := filepath.Join("fixtures", "Spoon_-_05_-_Revenge.mp3")
	if _, err := os.Stat(audioPath); os.IsNotExist(err) {
		t.Skip("Test audio not found")
	}

	samples, sampleRate, err := LoadAudioMono(audioPath)
	require.NoError(t, err)

	t.Logf("Audio loaded: %d samples at %d Hz (%.2f seconds)",
		len(samples), sampleRate, float64(len(samples))/float64(sampleRate))

	// Basic sanity checks
	require.Equal(t, 44100, sampleRate)
	require.Greater(t, len(samples), 44100*100) // At least 100 seconds
}
