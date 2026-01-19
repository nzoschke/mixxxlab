package analyzer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAnalyzeModel(t *testing.T) {
	savedModelPath, _ := filepath.Abs(filepath.Join("..", "models", "detect_beat", "model8"))

	if _, err := os.Stat(savedModelPath); os.IsNotExist(err) {
		t.Skip("SavedModel not found")
	}

	ms, err := AnalyzeModel(savedModelPath)
	if err != nil {
		t.Fatalf("AnalyzeModel failed: %v", err)
	}

	t.Logf("Model has %d STFT configs:", len(ms.STFTConfigs))
	for i, cfg := range ms.STFTConfigs {
		t.Logf("  STFT %d: frame_length=%d, frame_step=%d, num_bins=%d",
			i, cfg.FrameLength, cfg.FrameStep, cfg.NumBins)
	}
}

func TestExportPostSTFTModel(t *testing.T) {
	savedModelPath, _ := filepath.Abs(filepath.Join("..", "models", "detect_beat", "model8"))

	if _, err := os.Stat(savedModelPath); os.IsNotExist(err) {
		t.Skip("SavedModel not found")
	}

	t.Log("Analyzing model structure for post-STFT export...")
	err := ExportPostSTFTModel(savedModelPath, "")
	if err != nil {
		t.Fatalf("ExportPostSTFTModel failed: %v", err)
	}
}
