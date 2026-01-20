// Package analysis provides beat detection and audio analysis.
// This file provides beat detection using CPJKU/beat_this via ONNX Runtime.
package analysis

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

// BeatThisAnalyzer performs beat and downbeat detection using CPJKU/beat_this.
// It uses two ONNX models: one for mel spectrogram preprocessing and one for
// the beat tracker itself.
type BeatThisAnalyzer struct {
	melSession   *ort.DynamicAdvancedSession
	modelSession *ort.DynamicAdvancedSession
	modelSize    string // "small" or "full"
	hopLength    int    // 441 samples at 22050 Hz
	sampleRate   int    // 22050 Hz
}

// beat_this model parameters
const (
	beatThisSampleRate = 22050
	beatThisHopLength  = 441
	beatThisNumMels    = 128
	beatThisChunkSize  = 1500 // frames (~30 seconds)
	beatThisOverlap    = 150  // frames (~3 seconds overlap)
)

// ortInitOnce ensures ONNX Runtime is initialized only once
var ortInitOnce sync.Once
var ortInitErr error

// NewBeatThisAnalyzer creates a new beat_this analyzer with the small model.
func NewBeatThisAnalyzer() (*BeatThisAnalyzer, error) {
	return NewBeatThisAnalyzerWithSize("small")
}

// NewBeatThisAnalyzerFull creates a new beat_this analyzer with the full model.
func NewBeatThisAnalyzerFull() (*BeatThisAnalyzer, error) {
	return NewBeatThisAnalyzerWithSize("full")
}

// NewBeatThisAnalyzerWithSize creates a new beat_this analyzer with the specified model size.
func NewBeatThisAnalyzerWithSize(modelSize string) (*BeatThisAnalyzer, error) {
	// Find models directory
	modelsDir, err := findBeatThisModels()
	if err != nil {
		return nil, err
	}

	melPath := filepath.Join(modelsDir, "mel.onnx")
	modelPath := filepath.Join(modelsDir, fmt.Sprintf("model_%s.onnx", modelSize))

	// Verify files exist
	if _, err := os.Stat(melPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("mel spectrogram model not found at %s - run: uv run export_beat_this.py", melPath)
	}
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("beat_this model not found at %s - run: uv run export_beat_this.py", modelPath)
	}

	// Initialize ONNX Runtime (once per process)
	ortInitOnce.Do(func() {
		ort.SetSharedLibraryPath(getONNXLibPath())
		ortInitErr = ort.InitializeEnvironment()
	})
	if ortInitErr != nil {
		return nil, fmt.Errorf("failed to initialize ONNX Runtime: %w", ortInitErr)
	}

	// Get model input/output names from ONNX file
	_, modelOutputInfo, err := ort.GetInputOutputInfo(modelPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get model info: %w", err)
	}
	if len(modelOutputInfo) < 2 {
		return nil, fmt.Errorf("model should have 2 outputs, got %d", len(modelOutputInfo))
	}

	// Extract output names (first is beat logits, second is downbeat logits)
	modelOutputNames := make([]string, len(modelOutputInfo))
	for i, info := range modelOutputInfo {
		modelOutputNames[i] = info.Name
	}

	// Create mel spectrogram session
	melSession, err := ort.NewDynamicAdvancedSession(
		melPath,
		[]string{"audio"},           // input names
		[]string{"mel_spectrogram"}, // output names
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create mel session: %w", err)
	}

	// Create beat tracker model session
	// Model has two outputs: beat logits and downbeat logits (names vary by model)
	modelSession, err := ort.NewDynamicAdvancedSession(
		modelPath,
		[]string{"mel_spectrogram"}, // input names
		modelOutputNames,            // output names (discovered from model)
		nil,
	)
	if err != nil {
		melSession.Destroy()
		return nil, fmt.Errorf("failed to create model session: %w", err)
	}

	return &BeatThisAnalyzer{
		melSession:   melSession,
		modelSession: modelSession,
		modelSize:    modelSize,
		hopLength:    beatThisHopLength,
		sampleRate:   beatThisSampleRate,
	}, nil
}

// findBeatThisModels locates the beat_this ONNX models directory.
func findBeatThisModels() (string, error) {
	// Check common locations
	candidates := []string{
		"models/beat_this",
		"../models/beat_this",
		"../../models/beat_this",
	}

	// Also check relative to executable
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(exeDir, "models/beat_this"),
			filepath.Join(exeDir, "../models/beat_this"),
		)
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("beat_this models not found - run: uv run export_beat_this.py")
}

// getONNXLibPath returns the path to the ONNX Runtime shared library.
func getONNXLibPath() string {
	// Check environment variable first
	if path := os.Getenv("ONNXRUNTIME_LIB_PATH"); path != "" {
		return path
	}

	// Platform-specific defaults
	// macOS: brew install onnxruntime
	// Linux: apt install libonnxruntime
	candidates := []string{
		"/opt/homebrew/lib/libonnxruntime.dylib",          // macOS ARM (Homebrew)
		"/usr/local/lib/libonnxruntime.dylib",             // macOS Intel (Homebrew)
		"/usr/lib/libonnxruntime.so",                      // Linux
		"/usr/local/lib/libonnxruntime.so",                // Linux (manual install)
		"C:\\Program Files\\onnxruntime\\onnxruntime.dll", // Windows
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// Fallback - let the library try to find it
	return "onnxruntime"
}

// Close releases ONNX Runtime resources.
func (a *BeatThisAnalyzer) Close() error {
	if a.melSession != nil {
		a.melSession.Destroy()
	}
	if a.modelSession != nil {
		a.modelSession.Destroy()
	}
	return nil
}

// BeatThisResult contains the output from beat_this analysis.
type BeatThisResult struct {
	BPM        float64
	Beats      []float64 // Beat timestamps in seconds
	Downbeats  []int     // Indices into Beats that are downbeats
	Duration   float64
	SampleRate int
}

// AnalyzeFile analyzes an audio file using beat_this.
func (a *BeatThisAnalyzer) AnalyzeFile(audioPath string) (*BeatThisResult, error) {
	// Load audio
	samples, sampleRate, err := LoadAudioMono(audioPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load audio: %w", err)
	}

	return a.AnalyzeSamples(samples, sampleRate)
}

// AnalyzeSamples analyzes audio samples using beat_this.
func (a *BeatThisAnalyzer) AnalyzeSamples(samples []float32, sampleRate int) (*BeatThisResult, error) {
	duration := float64(len(samples)) / float64(sampleRate)

	// Resample to 22050 Hz if needed
	if sampleRate != a.sampleRate {
		samples = resampleAudioBeatThis(samples, sampleRate, a.sampleRate)
		sampleRate = a.sampleRate
	}

	// Compute mel spectrogram
	mel, err := a.computeMelSpectrogram(samples)
	if err != nil {
		return nil, fmt.Errorf("mel spectrogram failed: %w", err)
	}

	// Run beat tracker with chunking for long audio
	beatLogits, downbeatLogits, err := a.runBeatTracker(mel)
	if err != nil {
		return nil, fmt.Errorf("beat tracking failed: %w", err)
	}

	// Extract beats and downbeats using peak detection
	beats, downbeatIndices := a.extractBeatsAndDownbeats(beatLogits, downbeatLogits)

	// Calculate BPM from beat intervals
	bpm := calculateBPMFromBeatsBeatThis(beats)

	return &BeatThisResult{
		BPM:        bpm,
		Beats:      beats,
		Downbeats:  downbeatIndices,
		Duration:   duration,
		SampleRate: sampleRate,
	}, nil
}

// computeMelSpectrogram runs the mel spectrogram ONNX model.
func (a *BeatThisAnalyzer) computeMelSpectrogram(audio []float32) ([][]float32, error) {
	// Create input tensor with shape (1, samples)
	inputShape := ort.NewShape(1, int64(len(audio)))
	inputTensor, err := ort.NewTensor(inputShape, audio)
	if err != nil {
		return nil, fmt.Errorf("failed to create input tensor: %w", err)
	}
	defer inputTensor.Destroy()

	// Create output slice with nil for auto-allocation
	outputs := []ort.Value{nil}

	// Run inference
	err = a.melSession.Run(
		[]ort.Value{inputTensor},
		outputs,
	)
	if err != nil {
		return nil, fmt.Errorf("mel inference failed: %w", err)
	}

	// Clean up auto-allocated output
	if outputs[0] != nil {
		defer outputs[0].Destroy()
	} else {
		return nil, fmt.Errorf("mel output was nil")
	}

	// Extract output - shape is (1, time, 128)
	outputShape := outputs[0].GetShape()

	if len(outputShape) != 3 {
		return nil, fmt.Errorf("unexpected mel output shape: %v", outputShape)
	}

	timeFrames := int(outputShape[1])
	numMels := int(outputShape[2])

	// Get the data from the output tensor
	outputTensor, ok := outputs[0].(*ort.Tensor[float32])
	if !ok {
		return nil, fmt.Errorf("unexpected output tensor type")
	}
	outputData := outputTensor.GetData()

	// Convert flat array to 2D (time, mels)
	mel := make([][]float32, timeFrames)
	for t := 0; t < timeFrames; t++ {
		mel[t] = make([]float32, numMels)
		for m := 0; m < numMels; m++ {
			mel[t][m] = outputData[t*numMels+m]
		}
	}

	return mel, nil
}

// runBeatTracker runs the beat tracker model with chunking for long audio.
func (a *BeatThisAnalyzer) runBeatTracker(mel [][]float32) ([]float32, []float32, error) {
	numFrames := len(mel)

	// For short audio, process in one go
	if numFrames <= beatThisChunkSize {
		return a.runBeatTrackerChunk(mel)
	}

	// For long audio, process in overlapping chunks and stitch
	var allBeatLogits []float32
	var allDownbeatLogits []float32

	chunkStart := 0
	for chunkStart < numFrames {
		chunkEnd := chunkStart + beatThisChunkSize
		if chunkEnd > numFrames {
			chunkEnd = numFrames
		}

		chunk := mel[chunkStart:chunkEnd]
		beatLogits, downbeatLogits, err := a.runBeatTrackerChunk(chunk)
		if err != nil {
			return nil, nil, err
		}

		if chunkStart == 0 {
			// First chunk - take all
			allBeatLogits = append(allBeatLogits, beatLogits...)
			allDownbeatLogits = append(allDownbeatLogits, downbeatLogits...)
		} else {
			// Subsequent chunks - skip overlap region
			skipFrames := beatThisOverlap / 2
			if skipFrames < len(beatLogits) {
				allBeatLogits = append(allBeatLogits, beatLogits[skipFrames:]...)
				allDownbeatLogits = append(allDownbeatLogits, downbeatLogits[skipFrames:]...)
			}
		}

		chunkStart += beatThisChunkSize - beatThisOverlap
	}

	return allBeatLogits, allDownbeatLogits, nil
}

// runBeatTrackerChunk runs the beat tracker on a single chunk.
func (a *BeatThisAnalyzer) runBeatTrackerChunk(mel [][]float32) ([]float32, []float32, error) {
	numFrames := len(mel)
	if numFrames == 0 {
		return nil, nil, fmt.Errorf("empty mel spectrogram")
	}
	numMels := len(mel[0])

	// Flatten mel to 1D for tensor creation - shape (1, time, 128)
	flatMel := make([]float32, numFrames*numMels)
	for t := 0; t < numFrames; t++ {
		for m := 0; m < numMels; m++ {
			flatMel[t*numMels+m] = mel[t][m]
		}
	}

	// Create input tensor with shape (1, time, 128)
	inputShape := ort.NewShape(1, int64(numFrames), int64(numMels))
	inputTensor, err := ort.NewTensor(inputShape, flatMel)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create input tensor: %w", err)
	}
	defer inputTensor.Destroy()

	// Create output slices with nil for auto-allocation (two outputs: beat and downbeat)
	outputs := []ort.Value{nil, nil}

	// Run inference
	err = a.modelSession.Run(
		[]ort.Value{inputTensor},
		outputs,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("model inference failed: %w", err)
	}

	// Clean up auto-allocated outputs
	for i, out := range outputs {
		if out != nil {
			defer out.Destroy()
		} else {
			return nil, nil, fmt.Errorf("model output %d was nil", i)
		}
	}

	// Extract beat logits - shape is (1, time)
	beatShape := outputs[0].GetShape()
	if len(beatShape) != 2 {
		return nil, nil, fmt.Errorf("unexpected beat output shape: %v", beatShape)
	}

	beatTensor, ok := outputs[0].(*ort.Tensor[float32])
	if !ok {
		return nil, nil, fmt.Errorf("unexpected beat output tensor type")
	}
	beatLogits := beatTensor.GetData()

	// Extract downbeat logits - shape is (1, time)
	downbeatShape := outputs[1].GetShape()
	if len(downbeatShape) != 2 {
		return nil, nil, fmt.Errorf("unexpected downbeat output shape: %v", downbeatShape)
	}

	downbeatTensor, ok := outputs[1].(*ort.Tensor[float32])
	if !ok {
		return nil, nil, fmt.Errorf("unexpected downbeat output tensor type")
	}
	downbeatLogits := downbeatTensor.GetData()

	// Copy the data since we'll destroy the tensors
	beatLogitsCopy := make([]float32, len(beatLogits))
	downbeatLogitsCopy := make([]float32, len(downbeatLogits))
	copy(beatLogitsCopy, beatLogits)
	copy(downbeatLogitsCopy, downbeatLogits)

	return beatLogitsCopy, downbeatLogitsCopy, nil
}

// extractBeatsAndDownbeats converts model logits to beat timestamps.
func (a *BeatThisAnalyzer) extractBeatsAndDownbeats(beatLogits, downbeatLogits []float32) ([]float64, []int) {
	// Apply sigmoid to convert logits to probabilities
	beatProbs := make([]float32, len(beatLogits))
	downbeatProbs := make([]float32, len(downbeatLogits))

	for i := range beatLogits {
		beatProbs[i] = sigmoid(beatLogits[i])
		downbeatProbs[i] = sigmoid(downbeatLogits[i])
	}

	// Find beat peaks
	hopSizeSeconds := float64(a.hopLength) / float64(a.sampleRate)
	beatThreshold := float32(0.5)
	minDistanceFrames := 20 // ~400ms at 50fps, allows up to 150 BPM

	beats := findPeaksBeatThis(beatProbs, beatThreshold, minDistanceFrames, hopSizeSeconds)

	// Find downbeat peaks
	downbeatThreshold := float32(0.5)
	minDownbeatDistanceFrames := 80 // ~1.6s at 50fps

	downbeatTimes := findPeaksBeatThis(downbeatProbs, downbeatThreshold, minDownbeatDistanceFrames, hopSizeSeconds)

	// Match downbeats to nearest beats
	downbeatIndices := matchDownbeatsToBeats(beats, downbeatTimes)

	return beats, downbeatIndices
}

// sigmoid applies the sigmoid function.
func sigmoid(x float32) float32 {
	return float32(1.0 / (1.0 + math.Exp(-float64(x))))
}

// findPeaksBeatThis finds peaks in probability array above threshold.
func findPeaksBeatThis(probs []float32, threshold float32, minDistance int, hopSizeSeconds float64) []float64 {
	var peaks []float64

	for i := 1; i < len(probs)-1; i++ {
		// Check if local maximum
		if probs[i] <= probs[i-1] || probs[i] <= probs[i+1] {
			continue
		}

		// Check threshold
		if probs[i] < threshold {
			continue
		}

		// Check minimum distance from previous peak
		if len(peaks) > 0 {
			lastPeakFrame := int(peaks[len(peaks)-1] / hopSizeSeconds)
			if i-lastPeakFrame < minDistance {
				// Keep the higher peak
				if probs[i] > probs[lastPeakFrame] {
					peaks[len(peaks)-1] = float64(i) * hopSizeSeconds
				}
				continue
			}
		}

		peaks = append(peaks, float64(i)*hopSizeSeconds)
	}

	return peaks
}

// matchDownbeatsToBeats finds which beats are downbeats.
func matchDownbeatsToBeats(beats []float64, downbeatTimes []float64) []int {
	var indices []int

	tolerance := 0.05 // 50ms tolerance

	for _, dt := range downbeatTimes {
		// Find closest beat
		bestIdx := -1
		bestDist := math.MaxFloat64

		for i, bt := range beats {
			dist := math.Abs(bt - dt)
			if dist < bestDist {
				bestDist = dist
				bestIdx = i
			}
		}

		if bestIdx >= 0 && bestDist < tolerance {
			// Check if already marked
			found := false
			for _, idx := range indices {
				if idx == bestIdx {
					found = true
					break
				}
			}
			if !found {
				indices = append(indices, bestIdx)
			}
		}
	}

	// Sort indices
	sort.Ints(indices)

	return indices
}

// resampleAudioBeatThis resamples audio from srcRate to dstRate using linear interpolation.
// This is a copy to avoid build tag issues with the TF version.
func resampleAudioBeatThis(samples []float32, srcRate, dstRate int) []float32 {
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

// calculateBPMFromBeatsBeatThis estimates BPM from beat timestamps.
// This is a local copy to avoid build tag issues with the TF version.
func calculateBPMFromBeatsBeatThis(beats []float64) float64 {
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
	median := medianFloat64BeatThis(intervals)

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

// medianFloat64BeatThis returns the median of a slice of float64 values.
func medianFloat64BeatThis(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	// Make a copy to avoid modifying the original
	sorted := make([]float64, len(values))
	copy(sorted, values)

	// Simple sort
	sort.Float64s(sorted)

	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return (sorted[mid-1] + sorted[mid]) / 2
	}
	return sorted[mid]
}
