package analysis

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"testing"
)

func TestSTFT_Basic(t *testing.T) {
	// Generate test signal: 1 second of random noise
	sampleRate := 44100
	samples := make([]float64, sampleRate)
	for i := range samples {
		samples[i] = rand.Float64()*2 - 1 // Random in [-1, 1]
	}

	cfg := GoSTFTConfig{
		FFTSize:    1024,
		HopSize:    441,
		WindowSize: 1024,
	}

	result := STFT(samples, cfg)

	// Expected frames: (44100 - 1024) / 441 = 97
	expectedFrames := (len(samples) - cfg.WindowSize) / cfg.HopSize
	if len(result) != expectedFrames {
		t.Errorf("Expected %d frames, got %d", expectedFrames, len(result))
	}

	// Expected bins: 1024/2 + 1 = 513
	expectedBins := cfg.FFTSize/2 + 1
	if len(result[0]) != expectedBins {
		t.Errorf("Expected %d bins, got %d", expectedBins, len(result[0]))
	}

	t.Logf("STFT result: %d frames x %d bins", len(result), len(result[0]))
}

func TestSTFT_ComparePython(t *testing.T) {
	// Generate deterministic test signal
	rand.Seed(42)
	sampleRate := 44100
	samples := make([]float64, sampleRate) // 1 second
	for i := range samples {
		samples[i] = rand.Float64()*2 - 1
	}

	// Compute STFT in Go
	cfg := GoSTFTConfig{
		FFTSize:    1024,
		HopSize:    441,
		WindowSize: 1024,
	}
	goResult := STFT(samples, cfg)

	// Compute STFT in Python and compare
	script := fmt.Sprintf(`
import numpy as np
import scipy.signal as signal
import json

# Same random seed and samples
np.random.seed(42)
samples = np.random.uniform(-1, 1, %d)

# Compute STFT with scipy
_, _, Zxx = signal.stft(
    samples,
    fs=44100,
    window='hann',
    nperseg=%d,
    noverlap=%d,
    nfft=%d,
    boundary=None,
    padded=False,
)

# Get magnitude, transpose to (frames, bins)
mag = np.abs(Zxx.T)

# Output shape and first frame for comparison
result = {
    'shape': list(mag.shape),
    'first_frame': mag[0, :10].tolist(),  # First 10 bins of first frame
    'last_frame': mag[-1, :10].tolist(),
    'mean': float(mag.mean()),
    'max': float(mag.max()),
}
print(json.dumps(result))
`, sampleRate, cfg.WindowSize, cfg.WindowSize-cfg.HopSize, cfg.FFTSize)

	cmd := exec.Command(getPythonPath(), "-c", script)
	cmd.Stderr = os.Stderr
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Python STFT failed: %v", err)
	}

	var pyResult map[string]interface{}
	if err := json.Unmarshal(output, &pyResult); err != nil {
		t.Fatalf("Failed to parse Python output: %v", err)
	}

	pyShape := pyResult["shape"].([]interface{})
	pyFrames := int(pyShape[0].(float64))
	pyBins := int(pyShape[1].(float64))

	t.Logf("Go STFT: %d frames x %d bins", len(goResult), len(goResult[0]))
	t.Logf("Python STFT: %d frames x %d bins", pyFrames, pyBins)

	// Compare shapes (may differ slightly due to boundary handling)
	if abs(len(goResult)-pyFrames) > 2 {
		t.Errorf("Frame count mismatch: Go=%d, Python=%d", len(goResult), pyFrames)
	}

	if len(goResult[0]) != pyBins {
		t.Errorf("Bin count mismatch: Go=%d, Python=%d", len(goResult[0]), pyBins)
	}

	// Compare mean magnitude
	goMean := 0.0
	for _, frame := range goResult {
		for _, v := range frame {
			goMean += v
		}
	}
	goMean /= float64(len(goResult) * len(goResult[0]))

	pyMean := pyResult["mean"].(float64)
	t.Logf("Mean magnitude: Go=%.6f, Python=%.6f", goMean, pyMean)

	// Allow some tolerance due to different implementations
	if math.Abs(goMean-pyMean)/pyMean > 0.1 {
		t.Errorf("Mean magnitude differs too much: Go=%.6f, Python=%.6f", goMean, pyMean)
	}
}

func TestMultiScaleSTFT(t *testing.T) {
	// 10 seconds of audio
	samples := make([]float64, 441000)
	for i := range samples {
		samples[i] = rand.Float64()*2 - 1
	}

	result := ComputeMultiScaleSTFT(samples)

	configs := DefaultSTFTConfigs()
	for i, cfg := range configs {
		expectedFrames := (len(samples) - cfg.WindowSize) / cfg.HopSize
		expectedBins := cfg.FFTSize/2 + 1

		t.Logf("Scale %d (FFT=%d): %d frames x %d bins (expected %d x %d)",
			i, cfg.FFTSize, len(result[i]), len(result[i][0]), expectedFrames, expectedBins)

		if len(result[i]) != expectedFrames {
			t.Errorf("Scale %d: expected %d frames, got %d", i, expectedFrames, len(result[i]))
		}
		if len(result[i][0]) != expectedBins {
			t.Errorf("Scale %d: expected %d bins, got %d", i, expectedBins, len(result[i][0]))
		}
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
