package analysis

import (
	"os"
	"path/filepath"
	"testing"
)

func TestQMAnalyzer(t *testing.T) {
	// Find a test audio file
	musicDir := filepath.Join("..", "..", "music")
	files, err := filepath.Glob(filepath.Join(musicDir, "*", "*.mp3"))
	if err != nil || len(files) == 0 {
		t.Skip("No test audio files found")
	}

	testFile := files[0]
	t.Logf("Testing with: %s", testFile)

	// Test with default config
	result, err := AnalyzeFileQM(testFile)
	if err != nil {
		t.Fatalf("AnalyzeFileQM failed: %v", err)
	}

	// Verify basic results
	if result.BPM <= 0 {
		t.Errorf("Expected positive BPM, got %f", result.BPM)
	}
	if len(result.Beats) == 0 {
		t.Error("Expected beats to be detected")
	}
	if result.Duration <= 0 {
		t.Errorf("Expected positive duration, got %f", result.Duration)
	}

	// Verify two-stage results
	if len(result.DetectionFunction) == 0 {
		t.Error("Expected detection function values")
	}
	if result.StepSizeFrames <= 0 {
		t.Errorf("Expected positive step size, got %d", result.StepSizeFrames)
	}
	if result.WindowSize <= 0 {
		t.Errorf("Expected positive window size, got %d", result.WindowSize)
	}
	if len(result.BeatPeriods) == 0 {
		t.Error("Expected beat periods")
	}

	t.Logf("BPM: %.2f", result.BPM)
	t.Logf("Beats: %d", len(result.Beats))
	t.Logf("Duration: %.2fs", result.Duration)
	t.Logf("Detection function values: %d", len(result.DetectionFunction))
	t.Logf("Beat periods: %d", len(result.BeatPeriods))
	t.Logf("Step size: %d frames", result.StepSizeFrames)
	t.Logf("Window size: %d frames", result.WindowSize)

	// Test BeatPeriodToBPM conversion
	if len(result.BeatPeriods) > 0 {
		midIdx := len(result.BeatPeriods) / 2
		periodBPM := result.BeatPeriodToBPM(result.BeatPeriods[midIdx])
		t.Logf("Beat period[%d] = %d -> %.2f BPM", midIdx, result.BeatPeriods[midIdx], periodBPM)
	}
}

func TestQMAnalyzerStreaming(t *testing.T) {
	// Find a test audio file
	musicDir := filepath.Join("..", "..", "music")
	files, err := filepath.Glob(filepath.Join(musicDir, "*", "*.mp3"))
	if err != nil || len(files) == 0 {
		t.Skip("No test audio files found")
	}

	testFile := files[0]
	t.Logf("Testing streaming with: %s", testFile)

	// Load audio
	samples, sampleRate, err := LoadAudioMono(testFile)
	if err != nil {
		t.Fatalf("Failed to load audio: %v", err)
	}

	// Create streaming analyzer
	analyzer, err := NewQMAnalyzer(sampleRate, 1, nil)
	if err != nil {
		t.Fatalf("Failed to create analyzer: %v", err)
	}
	defer analyzer.Close()

	// Process in chunks
	chunkSize := 4096
	for i := 0; i < len(samples); i += chunkSize {
		end := i + chunkSize
		if end > len(samples) {
			end = len(samples)
		}
		if err := analyzer.Process(samples[i:end]); err != nil {
			t.Fatalf("Failed to process chunk: %v", err)
		}
	}

	// Finalize
	result, err := analyzer.Finalize()
	if err != nil {
		t.Fatalf("Finalize failed: %v", err)
	}

	t.Logf("Streaming result - BPM: %.2f, Beats: %d, DF values: %d",
		result.BPM, len(result.Beats), len(result.DetectionFunction))
}

func TestQMAnalyzerConfig(t *testing.T) {
	// Find a test audio file
	musicDir := filepath.Join("..", "..", "music")
	files, err := filepath.Glob(filepath.Join(musicDir, "*", "*.mp3"))
	if err != nil || len(files) == 0 {
		t.Skip("No test audio files found")
	}

	testFile := files[0]

	// Test with custom config - constrain tempo around 120 BPM
	cfg := DefaultQMConfig()
	cfg.InputTempo = 120.0
	cfg.ConstrainTempo = true

	result, err := AnalyzeFileQMConfig(testFile, &cfg)
	if err != nil {
		t.Fatalf("AnalyzeFileQMConfig failed: %v", err)
	}

	t.Logf("Constrained BPM: %.2f (target: 120)", result.BPM)

	// Test with different detection function types
	for _, dfType := range []DetectionFunctionType{DFTypeHFC, DFTypeSpecDiff, DFTypeComplexSD} {
		cfg := DefaultQMConfig()
		cfg.DFType = dfType

		result, err := AnalyzeFileQMConfig(testFile, &cfg)
		if err != nil {
			t.Errorf("DFType %d failed: %v", dfType, err)
			continue
		}
		t.Logf("DFType %d: BPM=%.2f, Beats=%d", dfType, result.BPM, len(result.Beats))
	}
}

func TestQMVersion(t *testing.T) {
	version := QMVersion()
	if version == "" {
		t.Error("Expected non-empty version string")
	}
	t.Logf("QM-DSP Analyzer version: %s", version)
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
