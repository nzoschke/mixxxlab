// analyzer.cpp - Implementation of C API for Mixxx beat detection

#include "analyzer.h"

#include <cstring>
#include <cstdlib>
#include <cmath>
#include <vector>
#include <memory>

#include <sndfile.h>

// qm-dsp includes
#include "dsp/onsets/DetectionFunction.h"
#include "dsp/tempotracking/TempoTrackV2.h"
#include "maths/MathUtilities.h"

namespace {

// Analysis parameters matching Mixxx defaults
constexpr float kStepSecs = 0.01161f;  // ~86 Hz resolution
constexpr int kMaximumBinSizeHz = 50;  // Hz

// Helper to downmix stereo to mono
void downmixToMono(const float* stereo, double* mono, size_t frames) {
    for (size_t i = 0; i < frames; ++i) {
        mono[i] = (static_cast<double>(stereo[i * 2]) +
                   static_cast<double>(stereo[i * 2 + 1])) * 0.5;
    }
}

// Helper to convert mono float samples to double
void convertToDouble(const float* src, double* dst, size_t count) {
    for (size_t i = 0; i < count; ++i) {
        dst[i] = static_cast<double>(src[i]);
    }
}

DFConfig makeDetectionFunctionConfig(int stepSizeFrames, int windowSize) {
    DFConfig config;
    config.DFType = DF_COMPLEXSD;
    config.stepSize = stepSizeFrames;
    config.frameLength = windowSize;
    config.dbRise = 3;
    config.adaptiveWhitening = false;
    config.whiteningRelaxCoeff = -1;
    config.whiteningFloor = -1;
    return config;
}

char* strdup_safe(const char* s) {
    if (!s) return nullptr;
    size_t len = strlen(s) + 1;
    char* copy = static_cast<char*>(malloc(len));
    if (copy) {
        memcpy(copy, s, len);
    }
    return copy;
}

} // namespace

extern "C" {

AnalyzerResult* analyzer_analyze_file(const char* filepath) {
    auto* result = static_cast<AnalyzerResult*>(calloc(1, sizeof(AnalyzerResult)));
    if (!result) {
        return nullptr;
    }

    // Open audio file with libsndfile
    SF_INFO sfinfo;
    memset(&sfinfo, 0, sizeof(sfinfo));

    SNDFILE* sndfile = sf_open(filepath, SFM_READ, &sfinfo);
    if (!sndfile) {
        result->error = strdup_safe(sf_strerror(nullptr));
        return result;
    }

    // Store basic info
    result->sample_rate = sfinfo.samplerate;
    result->total_frames = sfinfo.frames;
    result->duration = static_cast<double>(sfinfo.frames) / sfinfo.samplerate;

    // Calculate step and window sizes
    int stepSizeFrames = static_cast<int>(sfinfo.samplerate * kStepSecs);
    int windowSize = MathUtilities::nextPowerOfTwo(sfinfo.samplerate / kMaximumBinSizeHz);

    // Initialize detection function
    auto dfConfig = makeDetectionFunctionConfig(stepSizeFrames, windowSize);
    DetectionFunction detectionFunction(dfConfig);

    // Read and process audio in chunks
    const size_t chunkSize = 4096;
    std::vector<float> readBuffer(chunkSize * sfinfo.channels);
    std::vector<double> monoBuffer(chunkSize);
    std::vector<double> windowBuffer(windowSize);
    std::vector<double> detectionResults;

    // Overlap-add buffer for windowed processing
    std::vector<double> overlapBuffer(windowSize, 0.0);
    size_t overlapPos = 0;
    size_t totalFramesProcessed = 0;

    while (true) {
        sf_count_t framesRead = sf_readf_float(sndfile, readBuffer.data(), chunkSize);
        if (framesRead <= 0) {
            break;
        }

        // Convert to mono double
        if (sfinfo.channels == 1) {
            convertToDouble(readBuffer.data(), monoBuffer.data(), framesRead);
        } else if (sfinfo.channels == 2) {
            downmixToMono(readBuffer.data(), monoBuffer.data(), framesRead);
        } else {
            // For multi-channel, just take first two channels as stereo
            for (sf_count_t i = 0; i < framesRead; ++i) {
                monoBuffer[i] = (static_cast<double>(readBuffer[i * sfinfo.channels]) +
                                 static_cast<double>(readBuffer[i * sfinfo.channels + 1])) * 0.5;
            }
        }

        // Add to overlap buffer and process windows
        for (sf_count_t i = 0; i < framesRead; ++i) {
            overlapBuffer[overlapPos] = monoBuffer[i];
            overlapPos++;
            totalFramesProcessed++;

            // When we have a full window, process it
            if (overlapPos >= static_cast<size_t>(windowSize)) {
                // Copy window data
                std::copy(overlapBuffer.begin(), overlapBuffer.end(), windowBuffer.begin());

                // Process and get detection value
                double df = detectionFunction.processTimeDomain(windowBuffer.data());
                detectionResults.push_back(df);

                // Shift overlap buffer by step size
                size_t shift = stepSizeFrames;
                if (shift < static_cast<size_t>(windowSize)) {
                    std::copy(overlapBuffer.begin() + shift, overlapBuffer.end(), overlapBuffer.begin());
                    overlapPos = windowSize - shift;
                } else {
                    overlapPos = 0;
                }
            }
        }
    }

    sf_close(sndfile);

    if (detectionResults.size() < 4) {
        result->error = strdup_safe("Not enough audio data for beat detection");
        return result;
    }

    // Skip first 2 results (may contain noise from initialization)
    size_t nonZeroCount = detectionResults.size();
    while (nonZeroCount > 0 && detectionResults[nonZeroCount - 1] <= 0.0) {
        --nonZeroCount;
    }

    if (nonZeroCount <= 2) {
        result->error = strdup_safe("No valid detection results");
        return result;
    }

    std::vector<double> df;
    df.reserve(nonZeroCount - 2);
    for (size_t i = 2; i < nonZeroCount; ++i) {
        df.push_back(detectionResults[i]);
    }

    // Run tempo tracker
    TempoTrackV2 tempoTracker(sfinfo.samplerate, stepSizeFrames);

    std::vector<int> beatPeriod(df.size() / 128 + 1);
    tempoTracker.calculateBeatPeriod(df, beatPeriod);

    std::vector<double> beats;
    tempoTracker.calculateBeats(df, beatPeriod, beats);

    if (beats.empty()) {
        result->error = strdup_safe("No beats detected");
        return result;
    }

    // Convert beat positions from DF units to seconds
    // Beat position in DF units * step size in samples / sample rate = seconds
    // We also add back the 2 skipped frames at the start
    result->num_beats = beats.size();
    result->beats = static_cast<double*>(malloc(sizeof(double) * beats.size()));
    if (!result->beats) {
        result->error = strdup_safe("Memory allocation failed");
        result->num_beats = 0;
        return result;
    }

    for (size_t i = 0; i < beats.size(); ++i) {
        // beats[i] is in DF frame units (relative to df array which starts at frame 2)
        // Convert to actual frame position, then to seconds
        double framePos = (beats[i] + 2) * stepSizeFrames + stepSizeFrames / 2.0;
        result->beats[i] = framePos / sfinfo.samplerate;
    }

    // Calculate BPM from beat positions
    if (beats.size() >= 2) {
        // Calculate average beat interval
        double totalInterval = 0.0;
        for (size_t i = 1; i < result->num_beats; ++i) {
            totalInterval += result->beats[i] - result->beats[i - 1];
        }
        double avgInterval = totalInterval / (result->num_beats - 1);
        result->bpm = 60.0 / avgInterval;
    }

    return result;
}

void analyzer_free_result(AnalyzerResult* result) {
    if (!result) return;
    free(result->beats);
    free(result->error);
    free(result);
}

const char* analyzer_version(void) {
    return "1.0.0-mixxx";
}

} // extern "C"
