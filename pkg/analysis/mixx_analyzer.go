// Package analysis provides Go bindings for Mixxx beat detection.
// This file wraps the qm-dsp library (Queen Mary DSP) two-stage beat detection.
package analysis

/*
#cgo CFLAGS: -I${SRCDIR}/lib
#cgo pkg-config: sndfile
#cgo LDFLAGS: -L${SRCDIR}/lib/build -lmixxx_analyzer -lstdc++
#cgo darwin LDFLAGS: -Wl,-rpath,${SRCDIR}/lib/build

#include "analyzer.h"
#include <stdlib.h>
*/
import "C"
import (
	"errors"
	"fmt"
	"unsafe"
)

// DetectionFunctionType specifies the onset detection algorithm.
type DetectionFunctionType int

const (
	// DFTypeHFC uses High-Frequency Content detection.
	DFTypeHFC DetectionFunctionType = C.DF_TYPE_HFC
	// DFTypeSpecDiff uses Spectral Difference detection.
	DFTypeSpecDiff DetectionFunctionType = C.DF_TYPE_SPECDIFF
	// DFTypePhaseDev uses Phase Deviation detection.
	DFTypePhaseDev DetectionFunctionType = C.DF_TYPE_PHASEDEV
	// DFTypeComplexSD uses Complex Spectral Difference (default, best for beats).
	DFTypeComplexSD DetectionFunctionType = C.DF_TYPE_COMPLEXSD
	// DFTypeBroadband uses Broadband Energy Rise detection.
	DFTypeBroadband DetectionFunctionType = C.DF_TYPE_BROADBAND
)

// SegmentFeatureType specifies the feature extraction method for segmentation.
type SegmentFeatureType int

const (
	// SegFeatureConstQ uses Constant-Q transform features.
	SegFeatureConstQ SegmentFeatureType = C.SEG_FEATURE_CONSTQ
	// SegFeatureChroma uses Chroma features.
	SegFeatureChroma SegmentFeatureType = C.SEG_FEATURE_CHROMA
	// SegFeatureMFCC uses MFCC features.
	SegFeatureMFCC SegmentFeatureType = C.SEG_FEATURE_MFCC
)

// QMCueType specifies the type of cue point.
type QMCueType int

const (
	// CueTypeDownbeat marks the first beat of a bar.
	CueTypeDownbeat QMCueType = C.CUE_TYPE_DOWNBEAT
	// CueTypePhrase marks the start of a phrase (e.g., every 8 bars).
	CueTypePhrase QMCueType = C.CUE_TYPE_PHRASE
	// CueTypeSection marks a section boundary (intro, verse, chorus, etc.).
	CueTypeSection QMCueType = C.CUE_TYPE_SECTION
	// CueTypeEnergy marks an energy change (drop, breakdown).
	CueTypeEnergy QMCueType = C.CUE_TYPE_ENERGY
)

// QMConfig holds configuration for the QM-DSP beat analyzer.
type QMConfig struct {
	// DFType specifies the detection function type.
	// Default: DFTypeComplexSD
	DFType DetectionFunctionType

	// StepSecs is the analysis step size in seconds.
	// Default: 0.01161 (~12ms, ~86Hz resolution)
	StepSecs float32

	// MaxBinHz is the maximum frequency bin size in Hz.
	// Determines FFT window size. Default: 50 Hz
	MaxBinHz int

	// DBRise is the dB rise threshold for broadband detection.
	// Only used when DFType is DFTypeBroadband. Default: 3.0
	DBRise float64

	// AdaptiveWhitening enables spectral whitening.
	// Default: false
	AdaptiveWhitening bool

	// InputTempo is a tempo hint in BPM for the tracker.
	// Default: 120.0, set to 0 for fully automatic detection.
	InputTempo float64

	// ConstrainTempo forces the tracker to stay near InputTempo.
	// Default: false
	ConstrainTempo bool

	// Alpha is the beat tracking weight (0-1).
	// Higher values favor consistent tempo. Default: 0.9
	Alpha float64

	// Tightness controls how strictly beats follow the tempo.
	// Higher values = stricter. Default: 4.0
	Tightness float64

	// BeatsPerBar for downbeat detection.
	// Default: 4
	BeatsPerBar int
}

// SegmenterConfig holds configuration for structural segmentation.
type SegmenterConfig struct {
	// FeatureType specifies the feature extraction method.
	// Default: SegFeatureConstQ
	FeatureType SegmentFeatureType

	// HopSize is the analysis hop size in seconds.
	// Default: 0.2
	HopSize float64

	// WindowSize is the analysis window size in seconds.
	// Default: 0.6
	WindowSize float64

	// NumClusters is the number of segment types to detect.
	// Default: 10
	NumClusters int

	// NumHMMStates is the number of HMM states.
	// Default: 40
	NumHMMStates int
}

// DefaultSegmenterConfig returns the default segmenter configuration.
func DefaultSegmenterConfig() SegmenterConfig {
	cCfg := C.segmenter_default_config()
	return SegmenterConfig{
		FeatureType:  SegmentFeatureType(cCfg.feature_type),
		HopSize:      float64(cCfg.hop_size),
		WindowSize:   float64(cCfg.window_size),
		NumClusters:  int(cCfg.num_clusters),
		NumHMMStates: int(cCfg.num_hmm_states),
	}
}

func (cfg SegmenterConfig) toC() C.AnalyzerSegmenterConfig {
	var cCfg C.AnalyzerSegmenterConfig
	cCfg.feature_type = C.int(cfg.FeatureType)
	cCfg.hop_size = C.double(cfg.HopSize)
	cCfg.window_size = C.double(cfg.WindowSize)
	cCfg.num_clusters = C.int(cfg.NumClusters)
	cCfg.num_hmm_states = C.int(cfg.NumHMMStates)
	return cCfg
}

// QMSegment represents a detected structural segment.
type QMSegment struct {
	Start float64 // Start time in seconds
	End   float64 // End time in seconds
	Type  int     // Segment type (0 to NumClusters-1)
}

// QMCue represents a detected cue point.
type QMCue struct {
	Time       float64   // Time in seconds
	Type       QMCueType // Cue type
	TypeIndex  int       // Index within type (e.g., section type 0-9)
	Confidence float64   // Confidence score (0-1)
}

// DefaultQMConfig returns the default configuration matching Mixxx defaults.
func DefaultQMConfig() QMConfig {
	cCfg := C.analyzer_default_config()
	return QMConfig{
		DFType:            DetectionFunctionType(cCfg.df_type),
		StepSecs:          float32(cCfg.step_secs),
		MaxBinHz:          int(cCfg.max_bin_hz),
		DBRise:            float64(cCfg.db_rise),
		AdaptiveWhitening: cCfg.adaptive_whitening != 0,
		InputTempo:        float64(cCfg.input_tempo),
		ConstrainTempo:    cCfg.constrain_tempo != 0,
		Alpha:             float64(cCfg.alpha),
		Tightness:         float64(cCfg.tightness),
		BeatsPerBar:       int(cCfg.beats_per_bar),
	}
}

func (cfg QMConfig) toC() C.AnalyzerConfig {
	var cCfg C.AnalyzerConfig
	cCfg.df_type = C.int(cfg.DFType)
	cCfg.step_secs = C.float(cfg.StepSecs)
	cCfg.max_bin_hz = C.int(cfg.MaxBinHz)
	cCfg.db_rise = C.double(cfg.DBRise)
	if cfg.AdaptiveWhitening {
		cCfg.adaptive_whitening = 1
	}
	cCfg.input_tempo = C.double(cfg.InputTempo)
	if cfg.ConstrainTempo {
		cCfg.constrain_tempo = 1
	}
	cCfg.alpha = C.double(cfg.Alpha)
	cCfg.tightness = C.double(cfg.Tightness)
	cCfg.beats_per_bar = C.int(cfg.BeatsPerBar)
	return cCfg
}

// QMResult contains the complete analysis results from QM-DSP beat detection.
type QMResult struct {
	// Basic results
	BPM         float64   // Detected tempo in beats per minute
	Beats       []float64 // Beat timestamps in seconds
	SampleRate  int       // Audio sample rate in Hz
	TotalFrames int64     // Total number of audio frames
	Duration    float64   // Audio duration in seconds

	// Stage 1: Detection function values (onset strength over time)
	DetectionFunction []float64 // Raw detection function values
	StepSizeFrames    int       // Step size in samples between DF values
	WindowSize        int       // FFT window size used

	// Stage 2: Beat periods (tempo estimates per ~1.5s window)
	// Each value represents the estimated beat period in DF frame units
	BeatPeriods []int

	// Downbeat detection
	Downbeats          []int     // Indices into Beats array that are downbeats (first beat of bar)
	BeatSpectralDiff   []float64 // Spectral difference at each beat (for downbeat analysis)
	NumDownbeats       int       // Number of downbeats detected

	// Segmentation
	Segments        []QMSegment // Structural segments
	NumSegmentTypes int         // Number of distinct segment types

	// Cue points (derived from downbeats, phrases, segments)
	Cues []QMCue
}

// Bars returns the number of bars (assuming 4 beats per bar).
func (r *QMResult) Bars() float64 {
	if len(r.Beats) == 0 {
		return 0
	}
	return float64(len(r.Beats)) / 4.0
}

// DFTimeToSeconds converts a detection function frame index to seconds.
func (r *QMResult) DFTimeToSeconds(dfIndex int) float64 {
	return float64(dfIndex*r.StepSizeFrames) / float64(r.SampleRate)
}

// BeatPeriodToBPM converts a beat period (in DF frames) to BPM.
func (r *QMResult) BeatPeriodToBPM(period int) float64 {
	if period <= 0 {
		return 0
	}
	// period is in DF frame units
	// seconds per beat = period * step_size / sample_rate
	secondsPerBeat := float64(period*r.StepSizeFrames) / float64(r.SampleRate)
	return 60.0 / secondsPerBeat
}

// QMAnalyzer provides streaming beat detection using the QM-DSP algorithm.
type QMAnalyzer struct {
	handle *C.QMAnalyzer
	config QMConfig
}

// NewQMAnalyzer creates a new streaming beat analyzer.
// sampleRate: audio sample rate in Hz
// channels: number of audio channels (1 or 2)
// config: optional configuration (nil for defaults)
func NewQMAnalyzer(sampleRate, channels int, config *QMConfig) (*QMAnalyzer, error) {
	var cCfg *C.AnalyzerConfig
	var cfg QMConfig

	if config != nil {
		cfg = *config
		c := cfg.toC()
		cCfg = &c
	} else {
		cfg = DefaultQMConfig()
	}

	handle := C.analyzer_create(C.int(sampleRate), C.int(channels), cCfg)
	if handle == nil {
		return nil, errors.New("failed to create QM analyzer")
	}

	return &QMAnalyzer{
		handle: handle,
		config: cfg,
	}, nil
}

// Process feeds audio samples to the analyzer.
// samples: interleaved audio samples (float32)
// numFrames: number of frames (samples per channel)
func (a *QMAnalyzer) Process(samples []float32) error {
	if a.handle == nil {
		return errors.New("analyzer not initialized")
	}
	if len(samples) == 0 {
		return nil
	}

	// Determine number of frames based on interleaved samples
	// We don't know channels here, so just pass as-is
	ret := C.analyzer_process(a.handle, (*C.float)(&samples[0]), C.size_t(len(samples)))
	if ret != 0 {
		return fmt.Errorf("error processing samples: %d", ret)
	}
	return nil
}

// ProcessFrames feeds audio frames to the analyzer.
// samples: interleaved audio samples
// numFrames: explicit number of frames
func (a *QMAnalyzer) ProcessFrames(samples []float32, numFrames int) error {
	if a.handle == nil {
		return errors.New("analyzer not initialized")
	}
	if numFrames == 0 {
		return nil
	}

	ret := C.analyzer_process(a.handle, (*C.float)(&samples[0]), C.size_t(numFrames))
	if ret != 0 {
		return fmt.Errorf("error processing samples: %d", ret)
	}
	return nil
}

// DetectionFunctionCount returns the current number of detection function values.
func (a *QMAnalyzer) DetectionFunctionCount() int {
	if a.handle == nil {
		return 0
	}
	return int(C.analyzer_get_df_count(a.handle))
}

// Finalize completes analysis and returns results.
// segConfig is optional - pass nil to skip segmentation.
// After calling Finalize, the analyzer should be closed.
func (a *QMAnalyzer) Finalize(segConfig *SegmenterConfig) (*QMResult, error) {
	if a.handle == nil {
		return nil, errors.New("analyzer not initialized")
	}

	var cSegCfg *C.AnalyzerSegmenterConfig
	if segConfig != nil {
		c := segConfig.toC()
		cSegCfg = &c
	}

	cResult := C.analyzer_finalize(a.handle, cSegCfg)
	if cResult == nil {
		return nil, errors.New("finalization returned nil")
	}
	defer C.analyzer_free_result_ex(cResult)

	if cResult.error != nil {
		return nil, errors.New(C.GoString(cResult.error))
	}

	result := &QMResult{
		BPM:            float64(cResult.bpm),
		SampleRate:     int(cResult.sample_rate),
		TotalFrames:    int64(cResult.total_frames),
		Duration:       float64(cResult.duration),
		StepSizeFrames: int(cResult.step_size_frames),
		WindowSize:     int(cResult.window_size),
	}

	// Copy beats
	if cResult.num_beats > 0 && cResult.beats != nil {
		numBeats := int(cResult.num_beats)
		result.Beats = make([]float64, numBeats)
		beatsSlice := unsafe.Slice(cResult.beats, numBeats)
		for i := 0; i < numBeats; i++ {
			result.Beats[i] = float64(beatsSlice[i])
		}
	}

	// Copy detection function
	if cResult.df_length > 0 && cResult.detection_function != nil {
		dfLen := int(cResult.df_length)
		result.DetectionFunction = make([]float64, dfLen)
		dfSlice := unsafe.Slice(cResult.detection_function, dfLen)
		for i := 0; i < dfLen; i++ {
			result.DetectionFunction[i] = float64(dfSlice[i])
		}
	}

	// Copy beat periods
	if cResult.bp_length > 0 && cResult.beat_periods != nil {
		bpLen := int(cResult.bp_length)
		result.BeatPeriods = make([]int, bpLen)
		bpSlice := unsafe.Slice(cResult.beat_periods, bpLen)
		for i := 0; i < bpLen; i++ {
			result.BeatPeriods[i] = int(bpSlice[i])
		}
	}

	// Copy downbeat data
	if cResult.num_downbeats > 0 && cResult.downbeats != nil {
		numDb := int(cResult.num_downbeats)
		result.Downbeats = make([]int, numDb)
		result.NumDownbeats = numDb
		dbSlice := unsafe.Slice(cResult.downbeats, numDb)
		for i := 0; i < numDb; i++ {
			result.Downbeats[i] = int(dbSlice[i])
		}
	}

	// Copy beat spectral difference
	if cResult.bsd_length > 0 && cResult.beat_spectral_diff != nil {
		bsdLen := int(cResult.bsd_length)
		result.BeatSpectralDiff = make([]float64, bsdLen)
		bsdSlice := unsafe.Slice(cResult.beat_spectral_diff, bsdLen)
		for i := 0; i < bsdLen; i++ {
			result.BeatSpectralDiff[i] = float64(bsdSlice[i])
		}
	}

	// Copy segments
	if cResult.num_segments > 0 && cResult.segments != nil {
		numSeg := int(cResult.num_segments)
		result.Segments = make([]QMSegment, numSeg)
		result.NumSegmentTypes = int(cResult.num_segment_types)
		segSlice := unsafe.Slice(cResult.segments, numSeg)
		for i := 0; i < numSeg; i++ {
			result.Segments[i] = QMSegment{
				Start: float64(segSlice[i].start),
				End:   float64(segSlice[i].end),
				Type:  int(segSlice[i]._type),
			}
		}
	}

	// Copy cue points
	if cResult.num_cue_points > 0 && cResult.cue_points != nil {
		numCues := int(cResult.num_cue_points)
		result.Cues = make([]QMCue, numCues)
		cueSlice := unsafe.Slice(cResult.cue_points, numCues)
		for i := 0; i < numCues; i++ {
			result.Cues[i] = QMCue{
				Time:       float64(cueSlice[i].time),
				Type:       QMCueType(cueSlice[i]._type),
				TypeIndex:  int(cueSlice[i].type_index),
				Confidence: float64(cueSlice[i].confidence),
			}
		}
	}

	return result, nil
}

// Close releases the analyzer resources.
func (a *QMAnalyzer) Close() {
	if a.handle != nil {
		C.analyzer_destroy(a.handle)
		a.handle = nil
	}
}

// AnalyzeFileQM analyzes an audio file using QM-DSP with default configuration.
func AnalyzeFileQM(filepath string) (*QMResult, error) {
	return AnalyzeFileQMFull(filepath, nil, nil)
}

// AnalyzeFileQMConfig analyzes an audio file using QM-DSP with custom configuration.
func AnalyzeFileQMConfig(filepath string, config *QMConfig) (*QMResult, error) {
	return AnalyzeFileQMFull(filepath, config, nil)
}

// AnalyzeFileQMFull analyzes an audio file with full config including segmentation.
func AnalyzeFileQMFull(filepath string, config *QMConfig, segConfig *SegmenterConfig) (*QMResult, error) {
	cpath := C.CString(filepath)
	defer C.free(unsafe.Pointer(cpath))

	var cCfg *C.AnalyzerConfig
	if config != nil {
		c := config.toC()
		cCfg = &c
	}

	var cSegCfg *C.AnalyzerSegmenterConfig
	if segConfig != nil {
		c := segConfig.toC()
		cSegCfg = &c
	}

	cResult := C.analyzer_analyze_file_ex(cpath, cCfg, cSegCfg)
	if cResult == nil {
		return nil, errors.New("analyzer returned nil result")
	}
	defer C.analyzer_free_result_ex(cResult)

	if cResult.error != nil {
		return nil, errors.New(C.GoString(cResult.error))
	}

	result := &QMResult{
		BPM:            float64(cResult.bpm),
		SampleRate:     int(cResult.sample_rate),
		TotalFrames:    int64(cResult.total_frames),
		Duration:       float64(cResult.duration),
		StepSizeFrames: int(cResult.step_size_frames),
		WindowSize:     int(cResult.window_size),
	}

	// Copy beats
	if cResult.num_beats > 0 && cResult.beats != nil {
		numBeats := int(cResult.num_beats)
		result.Beats = make([]float64, numBeats)
		beatsSlice := unsafe.Slice(cResult.beats, numBeats)
		for i := 0; i < numBeats; i++ {
			result.Beats[i] = float64(beatsSlice[i])
		}
	}

	// Copy detection function
	if cResult.df_length > 0 && cResult.detection_function != nil {
		dfLen := int(cResult.df_length)
		result.DetectionFunction = make([]float64, dfLen)
		dfSlice := unsafe.Slice(cResult.detection_function, dfLen)
		for i := 0; i < dfLen; i++ {
			result.DetectionFunction[i] = float64(dfSlice[i])
		}
	}

	// Copy beat periods
	if cResult.bp_length > 0 && cResult.beat_periods != nil {
		bpLen := int(cResult.bp_length)
		result.BeatPeriods = make([]int, bpLen)
		bpSlice := unsafe.Slice(cResult.beat_periods, bpLen)
		for i := 0; i < bpLen; i++ {
			result.BeatPeriods[i] = int(bpSlice[i])
		}
	}

	// Copy downbeat data
	if cResult.num_downbeats > 0 && cResult.downbeats != nil {
		numDb := int(cResult.num_downbeats)
		result.Downbeats = make([]int, numDb)
		result.NumDownbeats = numDb
		dbSlice := unsafe.Slice(cResult.downbeats, numDb)
		for i := 0; i < numDb; i++ {
			result.Downbeats[i] = int(dbSlice[i])
		}
	}

	// Copy beat spectral difference
	if cResult.bsd_length > 0 && cResult.beat_spectral_diff != nil {
		bsdLen := int(cResult.bsd_length)
		result.BeatSpectralDiff = make([]float64, bsdLen)
		bsdSlice := unsafe.Slice(cResult.beat_spectral_diff, bsdLen)
		for i := 0; i < bsdLen; i++ {
			result.BeatSpectralDiff[i] = float64(bsdSlice[i])
		}
	}

	// Copy segments
	if cResult.num_segments > 0 && cResult.segments != nil {
		numSeg := int(cResult.num_segments)
		result.Segments = make([]QMSegment, numSeg)
		result.NumSegmentTypes = int(cResult.num_segment_types)
		segSlice := unsafe.Slice(cResult.segments, numSeg)
		for i := 0; i < numSeg; i++ {
			result.Segments[i] = QMSegment{
				Start: float64(segSlice[i].start),
				End:   float64(segSlice[i].end),
				Type:  int(segSlice[i]._type),
			}
		}
	}

	// Copy cue points
	if cResult.num_cue_points > 0 && cResult.cue_points != nil {
		numCues := int(cResult.num_cue_points)
		result.Cues = make([]QMCue, numCues)
		cueSlice := unsafe.Slice(cResult.cue_points, numCues)
		for i := 0; i < numCues; i++ {
			result.Cues[i] = QMCue{
				Time:       float64(cueSlice[i].time),
				Type:       QMCueType(cueSlice[i]._type),
				TypeIndex:  int(cueSlice[i].type_index),
				Confidence: float64(cueSlice[i].confidence),
			}
		}
	}

	return result, nil
}

// QMVersion returns the version of the QM-DSP analyzer library.
func QMVersion() string {
	return C.GoString(C.analyzer_version())
}
