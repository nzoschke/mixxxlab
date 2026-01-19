//go:build tensorflow

// Package analyzer provides Go bindings for beat detection.
// This file provides ML-based beat detection using TensorFlow Go bindings.
package analysis

import (
	"fmt"
	"math"
	"os"
	"path/filepath"

	tf "github.com/wamuir/graft/tensorflow"
)

// TFAnalyzer performs beat detection using TensorFlow SavedModel.
type TFAnalyzer struct {
	model      *tf.SavedModel
	inputOp    string
	outputOps  []string
	sampleRate int
}

// rekordboxModelsPath is the path to rekordbox's bundled ML models.
const rekordboxModelsPath = "/Applications/rekordbox 7/rekordbox.app/Contents/Resources/models"

// NewTFAnalyzer creates a new TensorFlow-based beat analyzer.
// It looks for models in rekordbox's application bundle.
func NewTFAnalyzer() (*TFAnalyzer, error) {
	modelPath := filepath.Join(rekordboxModelsPath, "detect_beat", "model8")

	// Verify model exists
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("model not found at %s - rekordbox 7 must be installed", modelPath)
	}

	return NewTFAnalyzerWithModel(modelPath)
}

// NewTFAnalyzerWithModel creates a new TensorFlow-based beat analyzer with a specific model.
func NewTFAnalyzerWithModel(modelPath string) (*TFAnalyzer, error) {
	// Load the SavedModel
	model, err := tf.LoadSavedModel(modelPath, []string{"serve"}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to load SavedModel: %w", err)
	}

	return &TFAnalyzer{
		model:      model,
		inputOp:    "serving_default_fltp",
		outputOps:  []string{"StatefulPartitionedCall"},
		sampleRate: 44100,
	}, nil
}

// Close releases the TensorFlow model resources.
func (a *TFAnalyzer) Close() error {
	if a.model != nil && a.model.Session != nil {
		return a.model.Session.Close()
	}
	return nil
}

// AnalyzeFile analyzes an audio file using TensorFlow beat detection.
func (a *TFAnalyzer) AnalyzeFile(audioPath string) (*MLAnalyzeOut, error) {
	// Load audio using the existing decoder
	samples, sampleRate, err := LoadAudioMono(audioPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load audio: %w", err)
	}

	return a.AnalyzeSamples(samples, sampleRate)
}

// AnalyzeSamples analyzes audio samples using TensorFlow beat detection.
func (a *TFAnalyzer) AnalyzeSamples(samples []float32, sampleRate int) (*MLAnalyzeOut, error) {
	// Resample to 44100 Hz if needed
	if sampleRate != a.sampleRate {
		samples = resampleAudio(samples, sampleRate, a.sampleRate)
		sampleRate = a.sampleRate
	}

	// Create input tensor [1, num_samples]
	inputData := make([][]float32, 1)
	inputData[0] = samples

	inputTensor, err := tf.NewTensor(inputData)
	if err != nil {
		return nil, fmt.Errorf("failed to create input tensor: %w", err)
	}

	// Get input and output operations
	inputOp := a.model.Graph.Operation(a.inputOp)
	if inputOp == nil {
		return nil, fmt.Errorf("input operation %q not found", a.inputOp)
	}

	outputOp := a.model.Graph.Operation(a.outputOps[0])
	if outputOp == nil {
		return nil, fmt.Errorf("output operation %q not found", a.outputOps[0])
	}

	// Run inference
	// Model outputs: output_1 (features), output_2 (beat activation)
	// We want output_2 which is index 1
	outputs, err := a.model.Session.Run(
		map[tf.Output]*tf.Tensor{
			inputOp.Output(0): inputTensor,
		},
		[]tf.Output{
			outputOp.Output(1), // beat activation [frames, 2]
		},
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("inference failed: %w", err)
	}

	// Extract beat activation from output
	// Shape is [batch, frames, 2] - column 0 is beat probability
	beatsOutput := outputs[0].Value()
	var beatsRaw []float32

	switch v := beatsOutput.(type) {
	case [][][]float32:
		// Shape [batch, frames, 2]
		frames := v[0]
		beatsRaw = make([]float32, len(frames))
		for i, frame := range frames {
			beatsRaw[i] = frame[0] // Column 0 is beat activation
		}
	case [][]float32:
		// Shape [frames, 2]
		beatsRaw = make([]float32, len(v))
		for i, frame := range v {
			beatsRaw[i] = frame[0]
		}
	default:
		return nil, fmt.Errorf("unexpected output type: %T", beatsOutput)
	}

	// Convert frame indices to seconds
	// Model outputs at 10ms resolution (hop size = 441 samples at 44100 Hz)
	hopSizeSeconds := float64(441) / float64(sampleRate)
	beats := extractBeats(beatsRaw, hopSizeSeconds, 0.1)

	// Calculate BPM from beat intervals
	bpm := calculateBPMFromBeats(beats)

	duration := float64(len(samples)) / float64(sampleRate)
	bars := float64(len(beats)) / 4.0

	return &MLAnalyzeOut{
		BPM:         bpm,
		Beats:       beats,
		SampleRate:  sampleRate,
		Duration:    duration,
		TotalFrames: int64(len(samples)),
		NumBeats:    len(beats),
		Bars:        bars,
	}, nil
}

// extractBeats converts model output probabilities to beat timestamps.
// Parameters match Python's scipy.signal.find_peaks behavior.
func extractBeats(probs []float32, hopSizeSeconds float64, threshold float32) []float64 {
	var beats []float64

	minDistance := 40 // 40 frames = 400ms at 10ms hop, allowing up to 150 BPM
	prominence := float32(0.05)

	// Find peaks above threshold with minimum distance and prominence
	for i := 1; i < len(probs)-1; i++ {
		// Check if this is a local maximum
		if probs[i] <= probs[i-1] || probs[i] <= probs[i+1] {
			continue
		}

		// Check threshold
		if probs[i] < threshold {
			continue
		}

		// Check prominence (simplified - compare to local minima)
		leftMin := probs[i]
		for j := i - 1; j >= 0 && j >= i-minDistance; j-- {
			if probs[j] < leftMin {
				leftMin = probs[j]
			}
		}
		rightMin := probs[i]
		for j := i + 1; j < len(probs) && j <= i+minDistance; j++ {
			if probs[j] < rightMin {
				rightMin = probs[j]
			}
		}
		localProminence := probs[i] - max(leftMin, rightMin)
		if localProminence < prominence {
			continue
		}

		// Check minimum distance from previous beat
		if len(beats) > 0 {
			lastBeatFrame := int(beats[len(beats)-1] / hopSizeSeconds)
			if i-lastBeatFrame < minDistance {
				continue
			}
		}

		beats = append(beats, float64(i)*hopSizeSeconds)
	}

	return beats
}

func max(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

// calculateBPMFromBeats estimates BPM from beat timestamps.
func calculateBPMFromBeats(beats []float64) float64 {
	if len(beats) < 2 {
		return 0
	}

	// Calculate intervals between beats
	var intervals []float64
	for i := 1; i < len(beats); i++ {
		interval := beats[i] - beats[i-1]
		if interval > 0.2 && interval < 2.0 { // Reasonable beat interval range
			intervals = append(intervals, interval)
		}
	}

	if len(intervals) == 0 {
		return 0
	}

	// Calculate median interval
	median := medianFloat64(intervals)

	// Convert to BPM
	bpm := 60.0 / median

	// Normalize to reasonable BPM range (60-180)
	for bpm < 60 {
		bpm *= 2
	}
	for bpm > 180 {
		bpm /= 2
	}

	return math.Round(bpm*100) / 100
}

// medianFloat64 returns the median of a slice of float64 values.
func medianFloat64(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	// Make a copy to avoid modifying the original
	sorted := make([]float64, len(values))
	copy(sorted, values)

	// Simple bubble sort for small arrays
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j] < sorted[i] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return (sorted[mid-1] + sorted[mid]) / 2
	}
	return sorted[mid]
}

// resampleAudio resamples audio from srcRate to dstRate using linear interpolation.
func resampleAudio(samples []float32, srcRate, dstRate int) []float32 {
	if srcRate == dstRate {
		return samples
	}

	ratio := float64(srcRate) / float64(dstRate)
	newLen := int(float64(len(samples)) / ratio)
	result := make([]float32, newLen)

	for i := 0; i < newLen; i++ {
		srcIdx := float64(i) * ratio
		srcIdxInt := int(srcIdx)
		frac := float32(srcIdx - float64(srcIdxInt))

		if srcIdxInt+1 < len(samples) {
			result[i] = samples[srcIdxInt]*(1-frac) + samples[srcIdxInt+1]*frac
		} else if srcIdxInt < len(samples) {
			result[i] = samples[srcIdxInt]
		}
	}

	return result
}
