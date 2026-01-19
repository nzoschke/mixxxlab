// Package analyzer provides Go bindings for Mixxx beat detection.
// It wraps the qm-dsp library (Queen Mary DSP) for BPM and beat grid analysis.
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
	"unsafe"
)

// AnalyzeOut contains the analysis results from beat detection.
type AnalyzeOut struct {
	BPM         float64   // BPM is the detected tempo in beats per minute.
	Beats       []float64 // Beats contains the timestamps of detected beats in seconds.
	SampleRate  int       // SampleRate is the sample rate of the audio file in Hz.
	TotalFrames int64     // TotalFrames is the total number of audio frames in the file.
	Duration    float64   // Duration is the total duration of the audio file in seconds.

}

// Bars returns the number of bars (4 beats per bar) in the track.
func (r *AnalyzeOut) Bars() float64 {
	if len(r.Beats) == 0 {
		return 0
	}
	return float64(len(r.Beats)) / 4.0
}

// AnalyzeFile analyzes an audio file and returns BPM and beat grid information.
// Supported formats include FLAC, WAV, AIFF, OGG, and MP3 (via libsndfile).
func AnalyzeFile(filepath string) (*AnalyzeOut, error) {
	cpath := C.CString(filepath)
	defer C.free(unsafe.Pointer(cpath))

	cresult := C.analyzer_analyze_file(cpath)
	if cresult == nil {
		return nil, errors.New("analyzer returned nil result")
	}
	defer C.analyzer_free_result(cresult)

	if cresult.error != nil {
		errMsg := C.GoString(cresult.error)
		return nil, errors.New(errMsg)
	}

	result := &AnalyzeOut{
		BPM:         float64(cresult.bpm),
		SampleRate:  int(cresult.sample_rate),
		TotalFrames: int64(cresult.total_frames),
		Duration:    float64(cresult.duration),
	}

	// Copy beats array
	if cresult.num_beats > 0 && cresult.beats != nil {
		numBeats := int(cresult.num_beats)
		result.Beats = make([]float64, numBeats)
		beatsSlice := unsafe.Slice(cresult.beats, numBeats)
		for i := 0; i < numBeats; i++ {
			result.Beats[i] = float64(beatsSlice[i])
		}
	}

	return result, nil
}

// Version returns the version of the analyzer library.
func Version() string {
	return C.GoString(C.analyzer_version())
}
