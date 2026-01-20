// analyzer.h - C API for Mixxx beat detection
// Wraps qm-dsp library for BPM and beat grid analysis

#ifndef MIXXX_ANALYZER_H
#define MIXXX_ANALYZER_H

#ifdef __cplusplus
extern "C" {
#endif

#include <stdint.h>
#include <stddef.h>

// Detection function types (matching qm-dsp)
typedef enum {
    DF_TYPE_HFC = 1,        // High-Frequency Content
    DF_TYPE_SPECDIFF = 2,   // Spectral Difference
    DF_TYPE_PHASEDEV = 3,   // Phase Deviation
    DF_TYPE_COMPLEXSD = 4,  // Complex Spectral Difference (default, best for beats)
    DF_TYPE_BROADBAND = 5   // Broadband Energy Rise
} DFType;

// Configuration for the analyzer
typedef struct {
    int df_type;            // Detection function type (DFType enum)
    float step_secs;        // Step size in seconds (default: 0.01161 = ~12ms)
    int max_bin_hz;         // Maximum bin size in Hz (default: 50)
    double db_rise;         // dB rise threshold for broadband (default: 3.0)
    int adaptive_whitening; // Enable adaptive whitening (default: 0)
    double input_tempo;     // Input tempo hint in BPM (default: 120, 0 = auto)
    int constrain_tempo;    // Constrain to input tempo (default: 0)
    double alpha;           // Beat tracking alpha (default: 0.9)
    double tightness;       // Beat tracking tightness (default: 4.0)
} AnalyzerConfig;

// Analysis result structure
typedef struct {
    double bpm;             // Detected BPM
    double* beats;          // Array of beat positions in seconds
    size_t num_beats;       // Number of beats detected
    int sample_rate;        // Sample rate of the audio
    int64_t total_frames;   // Total number of frames in the audio
    double duration;        // Duration in seconds
    char* error;            // Error message if analysis failed (NULL if success)
} AnalyzerResult;

// Extended result with two-stage process data
typedef struct {
    // Basic results
    double bpm;
    double* beats;
    size_t num_beats;
    int sample_rate;
    int64_t total_frames;
    double duration;
    char* error;

    // Stage 1: Detection function values
    double* detection_function;
    size_t df_length;
    int step_size_frames;
    int window_size;

    // Stage 2: Beat periods (tempo estimates per ~1.5s window)
    int* beat_periods;
    size_t bp_length;
} AnalyzerResultEx;

// Opaque handle for streaming analyzer
typedef struct QMAnalyzer QMAnalyzer;

// Get default configuration
AnalyzerConfig analyzer_default_config(void);

// Analyze an audio file and return BPM and beat grid information
// Returns NULL on failure, caller must free result with analyzer_free_result
AnalyzerResult* analyzer_analyze_file(const char* filepath);

// Analyze with custom configuration
AnalyzerResult* analyzer_analyze_file_config(const char* filepath, const AnalyzerConfig* config);

// Analyze and return extended results including detection function and beat periods
AnalyzerResultEx* analyzer_analyze_file_ex(const char* filepath, const AnalyzerConfig* config);

// Free the analysis result
void analyzer_free_result(AnalyzerResult* result);
void analyzer_free_result_ex(AnalyzerResultEx* result);

// === Streaming API ===

// Create a streaming analyzer for processing audio in chunks
// sample_rate: audio sample rate in Hz
// channels: number of audio channels (1 or 2)
// config: optional configuration (NULL for defaults)
QMAnalyzer* analyzer_create(int sample_rate, int channels, const AnalyzerConfig* config);

// Process a chunk of audio samples
// samples: interleaved audio samples (float32)
// num_frames: number of frames (samples per channel)
// Returns 0 on success, non-zero on error
int analyzer_process(QMAnalyzer* analyzer, const float* samples, size_t num_frames);

// Finalize analysis and get results
// Returns extended results, caller must free with analyzer_free_result_ex
AnalyzerResultEx* analyzer_finalize(QMAnalyzer* analyzer);

// Destroy the streaming analyzer
void analyzer_destroy(QMAnalyzer* analyzer);

// Get the current number of detection function values computed
size_t analyzer_get_df_count(QMAnalyzer* analyzer);

// Get the version of the analyzer library
const char* analyzer_version(void);

#ifdef __cplusplus
}
#endif

#endif // MIXXX_ANALYZER_H
