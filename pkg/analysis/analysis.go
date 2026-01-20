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

// TrackAnalysis represents the JSON output for a track with separate grid and marker results.
type TrackAnalysis struct {
	File       string                  `json:"file"`
	Duration   float64                 `json:"duration"`
	SampleRate int                     `json:"sample_rate"`
	Grids      map[string]*GridAnalysis   `json:"grids"`             // Beat grid strategies
	Markers    map[string]*MarkerAnalysis `json:"markers,omitempty"` // Cue/phrase marker strategies
	Waveform   *Waveform               `json:"waveform,omitempty"`
}

// GridAnalysis represents beat detection results from a single grid analyzer.
type GridAnalysis struct {
	BPM   float64   `json:"bpm"`
	Beats []float64 `json:"beats"`
	Error string    `json:"error,omitempty"`

	// Downbeat detection (indices into Beats that are downbeats)
	Downbeats []int `json:"downbeats,omitempty"`

	// Extended data from QM-DSP two-stage process (optional)
	DetectionFunction []float64 `json:"detection_function,omitempty"` // Stage 1: onset strength
	BeatPeriods       []int     `json:"beat_periods,omitempty"`       // Stage 2: tempo per window
	StepSizeFrames    int       `json:"step_size_frames,omitempty"`   // DF frame step in samples
	WindowSize        int       `json:"window_size,omitempty"`        // FFT window size
}

// MarkerAnalysis represents cue points and phrases from a single marker analyzer.
type MarkerAnalysis struct {
	CuePoints []CuePoint `json:"cue_points,omitempty"` // Detected cue points
	Phrases   []Phrase   `json:"phrases,omitempty"`    // Detected phrases/sections
	Error     string     `json:"error,omitempty"`
}

// Segment represents a structural segment of a track.
type Segment struct {
	Start float64 `json:"start"` // Start time in seconds
	End   float64 `json:"end"`   // End time in seconds
	Type  int     `json:"type"`  // Segment type (0 to num_clusters-1)
}

// Phrase represents a musical phrase/section detected by SongFormer.
type Phrase struct {
	Time     float64 `json:"time"`     // Start time in seconds
	Label    string  `json:"label"`    // Original label (intro, verse, chorus, etc.)
	Duration float64 `json:"duration"` // Duration in seconds (calculated)
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
	AnalyzerMixx         AnalyzerType = "mixx"          // CGO qm-dsp library (basic)
	AnalyzerMixxExtended AnalyzerType = "mixx-extended" // CGO qm-dsp with two-stage process data
	AnalyzerRekordboxPy  AnalyzerType = "rekordbox-py"  // Python ML subprocess (Rekordbox model)
	AnalyzerRekordboxGo  AnalyzerType = "rekordbox-go"  // TensorFlow Go bindings (Rekordbox model)
	AnalyzerBeatThis     AnalyzerType = "beatthis"      // CPJKU/beat_this via ONNX (small model)
	AnalyzerBeatThisFull AnalyzerType = "beatthis-full" // CPJKU/beat_this via ONNX (full model)
)

// Analyzer wraps multiple beat analyzers for comparison.
type Analyzer struct {
	mlPython      *MLAnalyzer
	tfGo          *TFAnalyzer
	cue           *CueAnalyzer
	beatThis      *BeatThisAnalyzer
	beatThisFull  *BeatThisAnalyzer
	songformer    *SongFormerAnalyzer
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

	// Try to initialize beat_this analyzer (small model)
	if bt, err := NewBeatThisAnalyzer(); err == nil {
		a.beatThis = bt
	}

	// Try to initialize beat_this analyzer (full model)
	if btFull, err := NewBeatThisAnalyzerFull(); err == nil {
		a.beatThisFull = btFull
	}

	// Try to initialize SongFormer analyzer (music structure)
	if sf, err := NewSongFormerAnalyzer(); err == nil {
		a.songformer = sf
	}

	return a, nil
}

// Close releases resources.
func (a *Analyzer) Close() error {
	var errs []error
	if a.tfGo != nil {
		if err := a.tfGo.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if a.beatThis != nil {
		if err := a.beatThis.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if a.beatThisFull != nil {
		if err := a.beatThisFull.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// AnalyzeFileWithPath analyzes a single audio file with all available analyzers.
func (a *Analyzer) AnalyzeFileWithPath(audioPath string) (*TrackAnalysis, error) {
	result := &TrackAnalysis{
		File:    filepath.Base(audioPath),
		Grids:   make(map[string]*GridAnalysis),
		Markers: make(map[string]*MarkerAnalysis),
	}

	// Run qm-dsp analyzer (CGO) - basic output
	if qmResult, err := AnalyzeFile(audioPath); err != nil {
		result.Grids[string(AnalyzerMixx)] = &GridAnalysis{Error: err.Error()}
	} else {
		result.Duration = qmResult.Duration
		result.SampleRate = qmResult.SampleRate
		result.Grids[string(AnalyzerMixx)] = &GridAnalysis{
			BPM:   qmResult.BPM,
			Beats: qmResult.Beats,
		}
	}

	// Run qm-dsp-extended analyzer (CGO) - full two-stage Mixxx process with segmentation
	segConfig := DefaultSegmenterConfig()
	if qmExResult, err := AnalyzeFileQMFull(audioPath, nil, &segConfig); err != nil {
		result.Grids[string(AnalyzerMixxExtended)] = &GridAnalysis{Error: err.Error()}
	} else {
		if result.Duration == 0 {
			result.Duration = qmExResult.Duration
			result.SampleRate = qmExResult.SampleRate
		}

		// Grid analysis
		result.Grids[string(AnalyzerMixxExtended)] = &GridAnalysis{
			BPM:               qmExResult.BPM,
			Beats:             qmExResult.Beats,
			DetectionFunction: qmExResult.DetectionFunction,
			BeatPeriods:       qmExResult.BeatPeriods,
			StepSizeFrames:    qmExResult.StepSizeFrames,
			WindowSize:        qmExResult.WindowSize,
			Downbeats:         qmExResult.Downbeats,
		}

		// Convert cues from QM beat analysis for markers
		var cues []CuePoint
		for _, cue := range qmExResult.Cues {
			cueType := "unknown"
			switch cue.Type {
			case CueTypeDownbeat:
				cueType = "downbeat"
			case CueTypePhrase:
				cueType = "phrase"
			case CueTypeSection:
				cueType = "section"
			case CueTypeEnergy:
				cueType = "energy"
			}
			cues = append(cues, CuePoint{
				Time:       cue.Time,
				Type:       cueType,
				Confidence: cue.Confidence,
				Name:       fmt.Sprintf("%s-%d", cueType, cue.TypeIndex),
			})
		}
		if len(cues) > 0 {
			result.Markers["beats"] = &MarkerAnalysis{CuePoints: cues}
		}
	}

	// Run ML Python analyzer
	if a.mlPython != nil {
		if mlResult, err := a.mlPython.AnalyzeFile(audioPath); err != nil {
			result.Grids[string(AnalyzerRekordboxPy)] = &GridAnalysis{Error: err.Error()}
		} else {
			if result.Duration == 0 {
				result.Duration = mlResult.Duration
				result.SampleRate = mlResult.SampleRate
			}
			result.Grids[string(AnalyzerRekordboxPy)] = &GridAnalysis{
				BPM:   mlResult.BPM,
				Beats: mlResult.Beats,
			}
		}
	}

	// Run TensorFlow Go analyzer
	if a.tfGo != nil {
		if tfResult, err := a.tfGo.AnalyzeFile(audioPath); err != nil {
			result.Grids[string(AnalyzerRekordboxGo)] = &GridAnalysis{Error: err.Error()}
		} else {
			if result.Duration == 0 {
				result.Duration = tfResult.Duration
				result.SampleRate = tfResult.SampleRate
			}
			result.Grids[string(AnalyzerRekordboxGo)] = &GridAnalysis{
				BPM:   tfResult.BPM,
				Beats: tfResult.Beats,
			}
		}
	}

	// Run beat_this analyzer (small model)
	if a.beatThis != nil {
		if btResult, err := a.beatThis.AnalyzeFile(audioPath); err != nil {
			result.Grids[string(AnalyzerBeatThis)] = &GridAnalysis{Error: err.Error()}
		} else {
			if result.Duration == 0 {
				result.Duration = btResult.Duration
				result.SampleRate = btResult.SampleRate
			}
			result.Grids[string(AnalyzerBeatThis)] = &GridAnalysis{
				BPM:       btResult.BPM,
				Beats:     btResult.Beats,
				Downbeats: btResult.Downbeats,
			}
		}
	}

	// Run beat_this analyzer (full model)
	if a.beatThisFull != nil {
		if btResult, err := a.beatThisFull.AnalyzeFile(audioPath); err != nil {
			result.Grids[string(AnalyzerBeatThisFull)] = &GridAnalysis{Error: err.Error()}
		} else {
			if result.Duration == 0 {
				result.Duration = btResult.Duration
				result.SampleRate = btResult.SampleRate
			}
			result.Grids[string(AnalyzerBeatThisFull)] = &GridAnalysis{
				BPM:       btResult.BPM,
				Beats:     btResult.Beats,
				Downbeats: btResult.Downbeats,
			}
		}
	}

	if len(result.Grids) == 0 {
		return nil, fmt.Errorf("no grid analyzers available")
	}

	// Generate waveform data
	waveform, err := GenerateWaveform(audioPath, 100) // 100 pixels per second
	if err != nil {
		fmt.Printf("  Warning: could not generate waveform: %v\n", err)
	} else {
		result.Waveform = waveform
	}

	// Detect cue points with Mixx analyzer (SampleCNN features)
	if a.cue != nil {
		if cueResult, err := a.cue.AnalyzeFile(audioPath, 8, 8.0); err != nil {
			fmt.Printf("  Warning: could not detect cue points: %v\n", err)
		} else {
			result.Markers["mixx"] = &MarkerAnalysis{CuePoints: cueResult.CuePoints}
		}
	}

	// Analyze music structure (phrases/sections) with SongFormer
	if a.songformer != nil {
		if sfResult, err := a.songformer.AnalyzeFile(audioPath); err != nil {
			fmt.Printf("  Warning: could not analyze music structure: %v\n", err)
		} else {
			result.Markers["songformer"] = &MarkerAnalysis{Phrases: sfResult.Phrases}
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

		// Print summary for each grid analyzer
		fmt.Printf("  Duration: %.1fs\n", analysis.Duration)
		fmt.Printf("  Grids:\n")
		for name, g := range analysis.Grids {
			if g.Error != "" {
				fmt.Printf("    %s: error - %s\n", name, g.Error)
			} else {
				fmt.Printf("    %s: BPM=%.1f, Beats=%d\n", name, g.BPM, len(g.Beats))
			}
		}
		if analysis.Waveform != nil {
			fmt.Printf("  Waveform: %d samples at %d px/sec\n",
				len(analysis.Waveform.Peaks), analysis.Waveform.PixelsPerSec)
		}
		if len(analysis.Markers) > 0 {
			fmt.Printf("  Markers:\n")
			for name, m := range analysis.Markers {
				if m.Error != "" {
					fmt.Printf("    %s: error - %s\n", name, m.Error)
				} else {
					parts := []string{}
					if len(m.CuePoints) > 0 {
						parts = append(parts, fmt.Sprintf("%d cues", len(m.CuePoints)))
					}
					if len(m.Phrases) > 0 {
						parts = append(parts, fmt.Sprintf("%d phrases", len(m.Phrases)))
					}
					fmt.Printf("    %s: %s\n", name, strings.Join(parts, ", "))
				}
			}
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
