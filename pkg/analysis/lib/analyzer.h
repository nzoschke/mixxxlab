// analyzer.h - C API for Mixxx beat detection
// Wraps qm-dsp library for BPM and beat grid analysis

#ifndef MIXXX_ANALYZER_H
#define MIXXX_ANALYZER_H

#ifdef __cplusplus
extern "C" {
#endif

#include <stdint.h>
#include <stddef.h>

// Analysis result structure
typedef struct {
    double bpm;           // Detected BPM
    double* beats;        // Array of beat positions in seconds
    size_t num_beats;     // Number of beats detected
    int sample_rate;      // Sample rate of the audio
    int64_t total_frames; // Total number of frames in the audio
    double duration;      // Duration in seconds
    char* error;          // Error message if analysis failed (NULL if success)
} AnalyzerResult;

// Analyze an audio file and return BPM and beat grid information
// Returns NULL on failure, caller must free result with analyzer_free_result
AnalyzerResult* analyzer_analyze_file(const char* filepath);

// Free the analysis result
void analyzer_free_result(AnalyzerResult* result);

// Get the version of the analyzer library
const char* analyzer_version(void);

#ifdef __cplusplus
}
#endif

#endif // MIXXX_ANALYZER_H
