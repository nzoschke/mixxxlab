// Package analysis provides beat detection and audio analysis.
package analysis

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// TrackAnalysis represents the JSON output for a track with multiple analyzer results.
type TrackAnalysis struct {
	File       string               `json:"file"`
	Duration   float64              `json:"duration"`
	SampleRate int                  `json:"sample_rate"`
	Analyzers  map[string]*Analysis `json:"analyzers"`
	CuePoints  []CuePoint           `json:"cue_points,omitempty"`
	Waveform   *Waveform            `json:"waveform,omitempty"`
}

// Analysis represents beat detection results from a single analyzer.
type Analysis struct {
	BPM   float64   `json:"bpm"`
	Beats []float64 `json:"beats"`
	Error string    `json:"error,omitempty"`

	// Extended data from QM-DSP two-stage process (optional)
	DetectionFunction []float64 `json:"detection_function,omitempty"` // Stage 1: onset strength
	BeatPeriods       []int     `json:"beat_periods,omitempty"`       // Stage 2: tempo per window
	StepSizeFrames    int       `json:"step_size_frames,omitempty"`   // DF frame step in samples
	WindowSize        int       `json:"window_size,omitempty"`        // FFT window size
}

// Waveform contains downsampled waveform data for visualization.
type Waveform struct {
	PixelsPerSec int       `json:"pixels_per_sec"`
	Peaks        []float64 `json:"peaks"`
	Troughs      []float64 `json:"troughs"`
}

// AnalyzerType represents the type of analyzer to use.
type AnalyzerType string

const (
	AnalyzerQMDSP      AnalyzerType = "qm-dsp"          // CGO qm-dsp library (basic)
	AnalyzerQMDSPEx    AnalyzerType = "qm-dsp-extended" // CGO qm-dsp with two-stage process data
	AnalyzerMLPython   AnalyzerType = "ml-python"       // Python ML subprocess
	AnalyzerTensorFlow AnalyzerType = "tensorflow"      // TensorFlow Go bindings
)

// Analyzer wraps multiple beat analyzers for comparison.
type Analyzer struct {
	mlPython *MLAnalyzer
	tfGo     *TFAnalyzer
	cue      *CueAnalyzer
}

// New creates a new Analyzer with all available implementations.
func New() (*Analyzer, error) {
	a := &Analyzer{}

	// Try to initialize ML Python analyzer
	if ml, err := NewMLAnalyzer(); err == nil {
		a.mlPython = ml
	}

	// Try to initialize TensorFlow Go analyzer
	if tf, err := NewTFAnalyzer(); err == nil {
		a.tfGo = tf
	}

	// Try to initialize Cue analyzer
	if cue, err := NewCueAnalyzer(); err == nil {
		a.cue = cue
	}

	return a, nil
}

// Close releases resources.
func (a *Analyzer) Close() error {
	if a.tfGo != nil {
		return a.tfGo.Close()
	}
	return nil
}

// AnalyzeFileWithPath analyzes a single audio file with all available analyzers.
func (a *Analyzer) AnalyzeFileWithPath(audioPath string) (*TrackAnalysis, error) {
	result := &TrackAnalysis{
		File:      filepath.Base(audioPath),
		Analyzers: make(map[string]*Analysis),
	}

	// Run qm-dsp analyzer (CGO) - basic output
	if qmResult, err := AnalyzeFile(audioPath); err != nil {
		result.Analyzers[string(AnalyzerQMDSP)] = &Analysis{Error: err.Error()}
	} else {
		result.Duration = qmResult.Duration
		result.SampleRate = qmResult.SampleRate
		result.Analyzers[string(AnalyzerQMDSP)] = &Analysis{
			BPM:   qmResult.BPM,
			Beats: qmResult.Beats,
		}
	}

	// Run qm-dsp-extended analyzer (CGO) - full two-stage Mixxx process
	if qmExResult, err := AnalyzeFileQM(audioPath); err != nil {
		result.Analyzers[string(AnalyzerQMDSPEx)] = &Analysis{Error: err.Error()}
	} else {
		if result.Duration == 0 {
			result.Duration = qmExResult.Duration
			result.SampleRate = qmExResult.SampleRate
		}
		result.Analyzers[string(AnalyzerQMDSPEx)] = &Analysis{
			BPM:               qmExResult.BPM,
			Beats:             qmExResult.Beats,
			DetectionFunction: qmExResult.DetectionFunction,
			BeatPeriods:       qmExResult.BeatPeriods,
			StepSizeFrames:    qmExResult.StepSizeFrames,
			WindowSize:        qmExResult.WindowSize,
		}
	}

	// Run ML Python analyzer
	if a.mlPython != nil {
		if mlResult, err := a.mlPython.AnalyzeFile(audioPath); err != nil {
			result.Analyzers[string(AnalyzerMLPython)] = &Analysis{Error: err.Error()}
		} else {
			if result.Duration == 0 {
				result.Duration = mlResult.Duration
				result.SampleRate = mlResult.SampleRate
			}
			result.Analyzers[string(AnalyzerMLPython)] = &Analysis{
				BPM:   mlResult.BPM,
				Beats: mlResult.Beats,
			}
		}
	}

	// Run TensorFlow Go analyzer
	if a.tfGo != nil {
		if tfResult, err := a.tfGo.AnalyzeFile(audioPath); err != nil {
			result.Analyzers[string(AnalyzerTensorFlow)] = &Analysis{Error: err.Error()}
		} else {
			if result.Duration == 0 {
				result.Duration = tfResult.Duration
				result.SampleRate = tfResult.SampleRate
			}
			result.Analyzers[string(AnalyzerTensorFlow)] = &Analysis{
				BPM:   tfResult.BPM,
				Beats: tfResult.Beats,
			}
		}
	}

	if len(result.Analyzers) == 0 {
		return nil, fmt.Errorf("no analyzers available")
	}

	// Generate waveform data
	waveform, err := GenerateWaveform(audioPath, 100) // 100 pixels per second
	if err != nil {
		fmt.Printf("  Warning: could not generate waveform: %v\n", err)
	} else {
		result.Waveform = waveform
	}

	// Detect cue points
	if a.cue != nil {
		if cueResult, err := a.cue.AnalyzeFile(audioPath, 8, 8.0); err != nil {
			fmt.Printf("  Warning: could not detect cue points: %v\n", err)
		} else {
			result.CuePoints = cueResult.CuePoints
		}
	}

	return result, nil
}

// GenerateWaveform creates downsampled waveform data for visualization.
// pixelsPerSec controls the resolution (e.g., 100 = 100 data points per second).
func GenerateWaveform(audioPath string, pixelsPerSec int) (*Waveform, error) {
	// Load audio samples
	samples, sampleRate, err := LoadAudioMono(audioPath)
	if err != nil {
		return nil, fmt.Errorf("load audio: %w", err)
	}

	// Calculate samples per pixel
	samplesPerPixel := sampleRate / pixelsPerSec
	if samplesPerPixel < 1 {
		samplesPerPixel = 1
	}

	numPixels := len(samples) / samplesPerPixel
	if numPixels == 0 {
		return nil, fmt.Errorf("audio too short")
	}

	peaks := make([]float64, numPixels)
	troughs := make([]float64, numPixels)

	for i := 0; i < numPixels; i++ {
		start := i * samplesPerPixel
		end := start + samplesPerPixel
		if end > len(samples) {
			end = len(samples)
		}

		maxVal := float32(-1.0)
		minVal := float32(1.0)
		for j := start; j < end; j++ {
			if samples[j] > maxVal {
				maxVal = samples[j]
			}
			if samples[j] < minVal {
				minVal = samples[j]
			}
		}

		peaks[i] = float64(maxVal)
		troughs[i] = float64(minVal)
	}

	return &Waveform{
		PixelsPerSec: pixelsPerSec,
		Peaks:        peaks,
		Troughs:      troughs,
	}, nil
}

// AnalyzeDir recursively analyzes all audio files in a directory.
// For each audio file, it creates a corresponding .json sidecar file.
// If force is true, existing JSON files are overwritten.
func (a *Analyzer) AnalyzeDir(dir string, force bool) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		// Process supported audio files
		ext := strings.ToLower(filepath.Ext(path))
		if !isSupportedAudio(ext) {
			return nil
		}

		// Check if JSON already exists
		jsonPath := strings.TrimSuffix(path, ext) + ".json"
		if !force {
			if _, err := os.Stat(jsonPath); err == nil {
				fmt.Printf("Skipping %s (already analyzed)\n", filepath.Base(path))
				return nil
			}
		}

		fmt.Printf("Analyzing %s...\n", filepath.Base(path))

		analysis, err := a.AnalyzeFileWithPath(path)
		if err != nil {
			fmt.Printf("  Error: %v\n", err)
			return nil // Continue with other files
		}

		// Write JSON sidecar
		data, err := json.MarshalIndent(analysis, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal JSON: %w", err)
		}

		if err := os.WriteFile(jsonPath, data, 0644); err != nil {
			return fmt.Errorf("write JSON: %w", err)
		}

		// Print summary for each analyzer
		fmt.Printf("  Duration: %.1fs\n", analysis.Duration)
		for name, a := range analysis.Analyzers {
			if a.Error != "" {
				fmt.Printf("  %s: error - %s\n", name, a.Error)
			} else {
				fmt.Printf("  %s: BPM=%.1f, Beats=%d\n", name, a.BPM, len(a.Beats))
			}
		}
		if analysis.Waveform != nil {
			fmt.Printf("  Waveform: %d samples at %d px/sec\n",
				len(analysis.Waveform.Peaks), analysis.Waveform.PixelsPerSec)
		}
		if len(analysis.CuePoints) > 0 {
			fmt.Printf("  Cue points: %d detected\n", len(analysis.CuePoints))
		}

		return nil
	})
}

// isSupportedAudio returns true if the file extension is a supported audio format.
func isSupportedAudio(ext string) bool {
	switch ext {
	case ".mp3", ".m4a", ".aac", ".wav", ".flac", ".ogg", ".aiff":
		return true
	default:
		return false
	}
}

// WriteJSON writes the analysis to a JSON file.
func (ta *TrackAnalysis) WriteJSON(path string) error {
	data, err := json.MarshalIndent(ta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
