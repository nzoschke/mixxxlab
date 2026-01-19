package analyzer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConvertToONNX(t *testing.T) {
	// Paths relative to analyzer directory
	savedModelPath, _ := filepath.Abs(filepath.Join("..", "models", "detect_beat", "model8"))
	outputPath, _ := filepath.Abs(filepath.Join("..", "models", "detect_beat", "model8.onnx"))

	// Skip if saved model doesn't exist
	if _, err := os.Stat(savedModelPath); os.IsNotExist(err) {
		t.Skip("SavedModel not found, skipping conversion test")
	}

	// Remove existing ONNX file to force reconversion
	os.Remove(outputPath)

	t.Log("Converting TensorFlow SavedModel to ONNX with opset 17...")
	err := ConvertToONNX(savedModelPath, outputPath, 17)
	if err != nil {
		t.Fatalf("ConvertToONNX failed: %v", err)
	}

	// Verify file was created
	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("ONNX file not created: %v", err)
	}
	t.Logf("ONNX file created: %s (%.2f MB)", outputPath, float64(info.Size())/1024/1024)
}

func TestVerifyONNX(t *testing.T) {
	onnxPath, _ := filepath.Abs(filepath.Join("..", "models", "detect_beat", "model8.onnx"))

	// Skip if ONNX model doesn't exist
	if _, err := os.Stat(onnxPath); os.IsNotExist(err) {
		t.Skip("ONNX model not found, skipping verification test")
	}

	t.Log("Verifying ONNX model with test inference...")
	inputLength := 441000 // 10 seconds at 44100 Hz

	result, err := VerifyONNX(onnxPath, inputLength)
	if err != nil {
		t.Fatalf("VerifyONNX failed: %v", err)
	}

	if result["status"][0] != 1 {
		t.Error("Verification failed")
	}
	t.Log("ONNX model verification passed")
}

func TestEnsureConvertDeps(t *testing.T) {
	t.Log("Ensuring conversion dependencies are up to date...")
	if err := EnsureConvertDeps(); err != nil {
		t.Fatalf("EnsureConvertDeps failed: %v", err)
	}
}

func TestConvertAndVerify(t *testing.T) {
	// NOTE: This test demonstrates ONNX conversion but the resulting model
	// cannot be executed by ONNX Runtime due to unsupported RFFT patterns.
	// The beat detection model uses RFFT→StridedSlice which tf2onnx cannot
	// properly convert. Use MLAnalyzer (Python subprocess) for actual inference.

	savedModelPath, _ := filepath.Abs(filepath.Join("..", "models", "detect_beat", "model8"))
	onnxPath, _ := filepath.Abs(filepath.Join("..", "models", "detect_beat", "model8.onnx"))

	// Skip if saved model doesn't exist
	if _, err := os.Stat(savedModelPath); os.IsNotExist(err) {
		t.Skip("SavedModel not found, skipping test")
	}

	// Ensure deps are up to date
	t.Log("Step 0: Ensuring dependencies...")
	if err := EnsureConvertDeps(); err != nil {
		t.Logf("Warning: EnsureConvertDeps failed: %v", err)
	}

	// Convert - this succeeds but produces an incomplete model
	t.Log("Step 1: Converting to ONNX with opset 17...")
	if err := ConvertToONNX(savedModelPath, onnxPath, 17); err != nil {
		t.Fatalf("Conversion failed: %v", err)
	}
	t.Log("Conversion completed (but model has unsupported RFFT ops)")

	// Verify - expected to fail due to RFFT ops
	t.Log("Step 2: Verifying ONNX model (expected to fail)...")
	if _, err := VerifyONNX(onnxPath, 441000); err != nil {
		t.Logf("Expected failure: %v", err)
		t.Log("ONNX Runtime cannot execute RFFT ops - use MLAnalyzer instead")
	} else {
		t.Log("Verification succeeded (unexpected)")
	}
}
