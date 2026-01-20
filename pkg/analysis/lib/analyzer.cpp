// analyzer.cpp - Implementation of C API for Mixxx beat detection
// Wraps qm-dsp library (Queen Mary DSP) for two-stage beat detection

#include "analyzer.h"

#include <cstring>
#include <cstdlib>
#include <cmath>
#include <vector>
#include <memory>
#include <algorithm>

#include <sndfile.h>

// qm-dsp includes
#include "dsp/onsets/DetectionFunction.h"
#include "dsp/tempotracking/TempoTrackV2.h"
#include "maths/MathUtilities.h"

namespace {

// Default analysis parameters matching Mixxx
constexpr float kDefaultStepSecs = 0.01161f;  // ~86 Hz resolution (~12ms)
constexpr int kDefaultMaxBinHz = 50;          // Hz
constexpr double kDefaultDBRise = 3.0;
constexpr double kDefaultInputTempo = 120.0;
constexpr double kDefaultAlpha = 0.9;
constexpr double kDefaultTightness = 4.0;

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

DFConfig makeDetectionFunctionConfig(const AnalyzerConfig& cfg, int stepSizeFrames, int windowSize) {
    DFConfig config;
    config.DFType = cfg.df_type > 0 ? cfg.df_type : DF_COMPLEXSD;
    config.stepSize = stepSizeFrames;
    config.frameLength = windowSize;
    config.dbRise = cfg.db_rise > 0 ? cfg.db_rise : kDefaultDBRise;
    config.adaptiveWhitening = cfg.adaptive_whitening != 0;
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

AnalyzerConfig getEffectiveConfig(const AnalyzerConfig* config) {
    AnalyzerConfig cfg;
    if (config) {
        cfg = *config;
    } else {
        cfg = analyzer_default_config();
    }
    return cfg;
}

} // namespace

// Streaming analyzer state
struct QMAnalyzer {
    int sampleRate;
    int channels;
    AnalyzerConfig config;
    int stepSizeFrames;
    int windowSize;

    std::unique_ptr<DetectionFunction> detectionFunction;
    std::vector<double> detectionResults;
    std::vector<double> overlapBuffer;
    size_t overlapPos;
    int64_t totalFramesProcessed;

    QMAnalyzer(int sr, int ch, const AnalyzerConfig& cfg)
        : sampleRate(sr)
        , channels(ch)
        , config(cfg)
        , overlapPos(0)
        , totalFramesProcessed(0)
    {
        float stepSecs = cfg.step_secs > 0 ? cfg.step_secs : kDefaultStepSecs;
        int maxBinHz = cfg.max_bin_hz > 0 ? cfg.max_bin_hz : kDefaultMaxBinHz;

        stepSizeFrames = static_cast<int>(sampleRate * stepSecs);
        windowSize = MathUtilities::nextPowerOfTwo(sampleRate / maxBinHz);

        auto dfConfig = makeDetectionFunctionConfig(cfg, stepSizeFrames, windowSize);
        detectionFunction = std::make_unique<DetectionFunction>(dfConfig);

        overlapBuffer.resize(windowSize, 0.0);
    }
};

extern "C" {

AnalyzerConfig analyzer_default_config(void) {
    AnalyzerConfig cfg;
    cfg.df_type = DF_COMPLEXSD;
    cfg.step_secs = kDefaultStepSecs;
    cfg.max_bin_hz = kDefaultMaxBinHz;
    cfg.db_rise = kDefaultDBRise;
    cfg.adaptive_whitening = 0;
    cfg.input_tempo = kDefaultInputTempo;
    cfg.constrain_tempo = 0;
    cfg.alpha = kDefaultAlpha;
    cfg.tightness = kDefaultTightness;
    return cfg;
}

AnalyzerResult* analyzer_analyze_file(const char* filepath) {
    return analyzer_analyze_file_config(filepath, nullptr);
}

AnalyzerResult* analyzer_analyze_file_config(const char* filepath, const AnalyzerConfig* config) {
    AnalyzerResultEx* exResult = analyzer_analyze_file_ex(filepath, config);
    if (!exResult) {
        return nullptr;
    }

    // Convert extended result to simple result
    auto* result = static_cast<AnalyzerResult*>(calloc(1, sizeof(AnalyzerResult)));
    if (!result) {
        analyzer_free_result_ex(exResult);
        return nullptr;
    }

    result->bpm = exResult->bpm;
    result->num_beats = exResult->num_beats;
    result->sample_rate = exResult->sample_rate;
    result->total_frames = exResult->total_frames;
    result->duration = exResult->duration;
    result->error = exResult->error;
    exResult->error = nullptr; // Transfer ownership

    // Transfer beats array
    result->beats = exResult->beats;
    exResult->beats = nullptr;

    // Free extended data but not the transferred fields
    analyzer_free_result_ex(exResult);

    return result;
}

AnalyzerResultEx* analyzer_analyze_file_ex(const char* filepath, const AnalyzerConfig* config) {
    auto* result = static_cast<AnalyzerResultEx*>(calloc(1, sizeof(AnalyzerResultEx)));
    if (!result) {
        return nullptr;
    }

    AnalyzerConfig cfg = getEffectiveConfig(config);

    // Open audio file with libsndfile
    SF_INFO sfinfo;
    memset(&sfinfo, 0, sizeof(sfinfo));

    SNDFILE* sndfile = sf_open(filepath, SFM_READ, &sfinfo);
    if (!sndfile) {
        result->error = strdup_safe(sf_strerror(nullptr));
        return result;
    }

    // Create streaming analyzer
    QMAnalyzer* analyzer = analyzer_create(sfinfo.samplerate, sfinfo.channels, &cfg);
    if (!analyzer) {
        sf_close(sndfile);
        result->error = strdup_safe("Failed to create analyzer");
        return result;
    }

    // Store basic info
    result->sample_rate = sfinfo.samplerate;
    result->total_frames = sfinfo.frames;
    result->duration = static_cast<double>(sfinfo.frames) / sfinfo.samplerate;

    // Read and process audio in chunks
    const size_t chunkSize = 4096;
    std::vector<float> readBuffer(chunkSize * sfinfo.channels);

    while (true) {
        sf_count_t framesRead = sf_readf_float(sndfile, readBuffer.data(), chunkSize);
        if (framesRead <= 0) {
            break;
        }

        int err = analyzer_process(analyzer, readBuffer.data(), framesRead);
        if (err != 0) {
            sf_close(sndfile);
            analyzer_destroy(analyzer);
            result->error = strdup_safe("Error processing audio");
            return result;
        }
    }

    sf_close(sndfile);

    // Finalize and get results
    AnalyzerResultEx* finalResult = analyzer_finalize(analyzer);
    analyzer_destroy(analyzer);

    if (!finalResult) {
        result->error = strdup_safe("Failed to finalize analysis");
        return result;
    }

    // Copy the info we already have
    finalResult->sample_rate = result->sample_rate;
    finalResult->total_frames = result->total_frames;
    finalResult->duration = result->duration;

    free(result);
    return finalResult;
}

void analyzer_free_result(AnalyzerResult* result) {
    if (!result) return;
    free(result->beats);
    free(result->error);
    free(result);
}

void analyzer_free_result_ex(AnalyzerResultEx* result) {
    if (!result) return;
    free(result->beats);
    free(result->error);
    free(result->detection_function);
    free(result->beat_periods);
    free(result);
}

// === Streaming API ===

QMAnalyzer* analyzer_create(int sample_rate, int channels, const AnalyzerConfig* config) {
    if (sample_rate <= 0 || channels <= 0 || channels > 2) {
        return nullptr;
    }

    AnalyzerConfig cfg = getEffectiveConfig(config);

    try {
        return new QMAnalyzer(sample_rate, channels, cfg);
    } catch (...) {
        return nullptr;
    }
}

int analyzer_process(QMAnalyzer* analyzer, const float* samples, size_t num_frames) {
    if (!analyzer || !samples || num_frames == 0) {
        return -1;
    }

    // Convert to mono double
    std::vector<double> monoBuffer(num_frames);
    if (analyzer->channels == 1) {
        convertToDouble(samples, monoBuffer.data(), num_frames);
    } else {
        downmixToMono(samples, monoBuffer.data(), num_frames);
    }

    // Process through overlap buffer
    std::vector<double> windowBuffer(analyzer->windowSize);

    for (size_t i = 0; i < num_frames; ++i) {
        analyzer->overlapBuffer[analyzer->overlapPos] = monoBuffer[i];
        analyzer->overlapPos++;
        analyzer->totalFramesProcessed++;

        // When we have a full window, process it
        if (analyzer->overlapPos >= static_cast<size_t>(analyzer->windowSize)) {
            // Copy window data
            std::copy(analyzer->overlapBuffer.begin(),
                      analyzer->overlapBuffer.end(),
                      windowBuffer.begin());

            // Process and get detection value
            double df = analyzer->detectionFunction->processTimeDomain(windowBuffer.data());
            analyzer->detectionResults.push_back(df);

            // Shift overlap buffer by step size
            size_t shift = analyzer->stepSizeFrames;
            if (shift < static_cast<size_t>(analyzer->windowSize)) {
                std::copy(analyzer->overlapBuffer.begin() + shift,
                          analyzer->overlapBuffer.end(),
                          analyzer->overlapBuffer.begin());
                analyzer->overlapPos = analyzer->windowSize - shift;
            } else {
                analyzer->overlapPos = 0;
            }
        }
    }

    return 0;
}

AnalyzerResultEx* analyzer_finalize(QMAnalyzer* analyzer) {
    if (!analyzer) {
        return nullptr;
    }

    auto* result = static_cast<AnalyzerResultEx*>(calloc(1, sizeof(AnalyzerResultEx)));
    if (!result) {
        return nullptr;
    }

    result->step_size_frames = analyzer->stepSizeFrames;
    result->window_size = analyzer->windowSize;
    result->sample_rate = analyzer->sampleRate;
    result->total_frames = analyzer->totalFramesProcessed;
    result->duration = static_cast<double>(analyzer->totalFramesProcessed) / analyzer->sampleRate;

    if (analyzer->detectionResults.size() < 4) {
        result->error = strdup_safe("Not enough audio data for beat detection");
        return result;
    }

    // Store raw detection function (including first 2 values)
    result->df_length = analyzer->detectionResults.size();
    result->detection_function = static_cast<double*>(
        malloc(sizeof(double) * result->df_length));
    if (result->detection_function) {
        std::copy(analyzer->detectionResults.begin(),
                  analyzer->detectionResults.end(),
                  result->detection_function);
    }

    // Skip first 2 results (may contain noise from initialization)
    size_t nonZeroCount = analyzer->detectionResults.size();
    while (nonZeroCount > 0 && analyzer->detectionResults[nonZeroCount - 1] <= 0.0) {
        --nonZeroCount;
    }

    if (nonZeroCount <= 2) {
        result->error = strdup_safe("No valid detection results");
        return result;
    }

    std::vector<double> df;
    df.reserve(nonZeroCount - 2);
    for (size_t i = 2; i < nonZeroCount; ++i) {
        df.push_back(analyzer->detectionResults[i]);
    }

    // Run tempo tracker - Stage 1: Calculate beat periods
    TempoTrackV2 tempoTracker(analyzer->sampleRate, analyzer->stepSizeFrames);

    std::vector<int> beatPeriod(df.size() / 128 + 1);

    double inputTempo = analyzer->config.input_tempo > 0 ?
        analyzer->config.input_tempo : kDefaultInputTempo;
    bool constrainTempo = analyzer->config.constrain_tempo != 0;

    tempoTracker.calculateBeatPeriod(df, beatPeriod, inputTempo, constrainTempo);

    // Store beat periods
    result->bp_length = beatPeriod.size();
    result->beat_periods = static_cast<int*>(malloc(sizeof(int) * result->bp_length));
    if (result->beat_periods) {
        std::copy(beatPeriod.begin(), beatPeriod.end(), result->beat_periods);
    }

    // Stage 2: Calculate actual beat positions
    std::vector<double> beats;

    double alpha = analyzer->config.alpha > 0 ?
        analyzer->config.alpha : kDefaultAlpha;
    double tightness = analyzer->config.tightness > 0 ?
        analyzer->config.tightness : kDefaultTightness;

    tempoTracker.calculateBeats(df, beatPeriod, beats, alpha, tightness);

    if (beats.empty()) {
        result->error = strdup_safe("No beats detected");
        return result;
    }

    // Convert beat positions from DF units to seconds
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
        double framePos = (beats[i] + 2) * analyzer->stepSizeFrames +
                          analyzer->stepSizeFrames / 2.0;
        result->beats[i] = framePos / analyzer->sampleRate;
    }

    // Calculate BPM from beat positions
    if (beats.size() >= 2) {
        double totalInterval = 0.0;
        for (size_t i = 1; i < result->num_beats; ++i) {
            totalInterval += result->beats[i] - result->beats[i - 1];
        }
        double avgInterval = totalInterval / (result->num_beats - 1);
        result->bpm = 60.0 / avgInterval;
    }

    return result;
}

void analyzer_destroy(QMAnalyzer* analyzer) {
    delete analyzer;
}

size_t analyzer_get_df_count(QMAnalyzer* analyzer) {
    if (!analyzer) return 0;
    return analyzer->detectionResults.size();
}

const char* analyzer_version(void) {
    return "2.0.0-mixxx-qmdsp";
}

} // extern "C"
