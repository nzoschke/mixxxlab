// analyzer.cpp - Implementation of C API for Mixxx beat detection
// Wraps qm-dsp library (Queen Mary DSP) for two-stage beat detection,
// downbeat detection, and segmentation

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
#include "dsp/tempotracking/DownBeat.h"
#include "dsp/segmentation/ClusterMeltSegmenter.h"
#include "maths/MathUtilities.h"

namespace {

// Default analysis parameters matching Mixxx
constexpr float kDefaultStepSecs = 0.01161f;  // ~86 Hz resolution (~12ms)
constexpr int kDefaultMaxBinHz = 50;          // Hz
constexpr double kDefaultDBRise = 3.0;
constexpr double kDefaultInputTempo = 120.0;
constexpr double kDefaultAlpha = 0.9;
constexpr double kDefaultTightness = 4.0;
constexpr int kDefaultBeatsPerBar = 4;

// Default segmenter parameters
constexpr int kDefaultSegFeatureType = FEATURE_TYPE_CONSTQ;
constexpr double kDefaultSegHopSize = 0.2;
constexpr double kDefaultSegWindowSize = 0.6;
constexpr int kDefaultSegNumClusters = 10;
constexpr int kDefaultSegNumHMMStates = 40;

// Decimation factor for downbeat analysis (matches qm-dsp recommendation)
constexpr size_t kDownbeatDecimationFactor = 16;

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

// Helper to convert float to double
void convertFloatToDouble(const float* src, double* dst, size_t count) {
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

AnalyzerSegmenterConfig getEffectiveSegConfig(const AnalyzerSegmenterConfig* config) {
    AnalyzerSegmenterConfig cfg;
    if (config) {
        cfg = *config;
    } else {
        cfg = segmenter_default_config();
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

    // Audio buffer for downbeat/segmentation analysis
    std::vector<float> audioBuffer;

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
    cfg.beats_per_bar = kDefaultBeatsPerBar;
    return cfg;
}

AnalyzerSegmenterConfig segmenter_default_config(void) {
    AnalyzerSegmenterConfig cfg;
    cfg.feature_type = kDefaultSegFeatureType;
    cfg.hop_size = kDefaultSegHopSize;
    cfg.window_size = kDefaultSegWindowSize;
    cfg.num_clusters = kDefaultSegNumClusters;
    cfg.num_hmm_states = kDefaultSegNumHMMStates;
    return cfg;
}

AnalyzerResult* analyzer_analyze_file(const char* filepath) {
    return analyzer_analyze_file_config(filepath, nullptr);
}

AnalyzerResult* analyzer_analyze_file_config(const char* filepath, const AnalyzerConfig* config) {
    AnalyzerResultEx* exResult = analyzer_analyze_file_ex(filepath, config, nullptr);
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

AnalyzerResultEx* analyzer_analyze_file_ex(const char* filepath,
                                           const AnalyzerConfig* config,
                                           const AnalyzerSegmenterConfig* seg_config) {
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
    AnalyzerResultEx* finalResult = analyzer_finalize(analyzer, seg_config);
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
    free(result->downbeats);
    free(result->beat_spectral_diff);
    free(result->segments);
    free(result->cue_points);
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

    // Store mono audio for downbeat/segmentation analysis
    for (size_t i = 0; i < num_frames; ++i) {
        analyzer->audioBuffer.push_back(static_cast<float>(monoBuffer[i]));
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

AnalyzerResultEx* analyzer_finalize(QMAnalyzer* analyzer, const AnalyzerSegmenterConfig* seg_config) {
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

    // === Downbeat Detection ===
    int beatsPerBar = analyzer->config.beats_per_bar > 0 ?
        analyzer->config.beats_per_bar : kDefaultBeatsPerBar;

    if (beats.size() >= 4 && !analyzer->audioBuffer.empty()) {
        // Create downbeat detector
        DownBeat downbeat(static_cast<float>(analyzer->sampleRate),
                          kDownbeatDecimationFactor,
                          analyzer->stepSizeFrames);
        downbeat.setBeatsPerBar(beatsPerBar);

        // Push audio through decimator
        size_t blockSize = analyzer->stepSizeFrames;
        for (size_t i = 0; i + blockSize <= analyzer->audioBuffer.size(); i += blockSize) {
            downbeat.pushAudioBlock(analyzer->audioBuffer.data() + i);
        }

        // Get decimated audio
        size_t decimatedLength = 0;
        const float* decimatedAudio = downbeat.getBufferedAudio(decimatedLength);

        if (decimatedAudio && decimatedLength > 0) {
            // Find downbeats
            std::vector<int> downbeatIndices;
            downbeat.findDownBeats(decimatedAudio, decimatedLength, beats, downbeatIndices);

            // Store downbeats
            result->num_downbeats = downbeatIndices.size();
            result->downbeats = static_cast<int*>(
                malloc(sizeof(int) * result->num_downbeats));
            if (result->downbeats) {
                std::copy(downbeatIndices.begin(), downbeatIndices.end(), result->downbeats);
            }

            // Get beat spectral differences
            std::vector<double> beatSD;
            downbeat.getBeatSD(beatSD);
            result->bsd_length = beatSD.size();
            result->beat_spectral_diff = static_cast<double*>(
                malloc(sizeof(double) * result->bsd_length));
            if (result->beat_spectral_diff) {
                std::copy(beatSD.begin(), beatSD.end(), result->beat_spectral_diff);
            }
        }
    }

    // === Segmentation ===
    if (seg_config && !analyzer->audioBuffer.empty()) {
        AnalyzerSegmenterConfig segCfg = getEffectiveSegConfig(seg_config);

        ClusterMeltSegmenterParams params;
        params.featureType = static_cast<feature_types>(
            segCfg.feature_type > 0 ? segCfg.feature_type : kDefaultSegFeatureType);
        params.hopSize = segCfg.hop_size > 0 ? segCfg.hop_size : kDefaultSegHopSize;
        params.windowSize = segCfg.window_size > 0 ? segCfg.window_size : kDefaultSegWindowSize;
        params.nHMMStates = segCfg.num_hmm_states > 0 ? segCfg.num_hmm_states : kDefaultSegNumHMMStates;
        params.nclusters = segCfg.num_clusters > 0 ? segCfg.num_clusters : kDefaultSegNumClusters;

        ClusterMeltSegmenter segmenter(params);
        segmenter.initialise(analyzer->sampleRate);

        int windowSize = segmenter.getWindowsize();
        int hopSize = segmenter.getHopsize();

        // Convert audio to double for segmenter
        std::vector<double> audioDouble(analyzer->audioBuffer.size());
        convertFloatToDouble(analyzer->audioBuffer.data(), audioDouble.data(),
                            analyzer->audioBuffer.size());

        // Extract features
        for (size_t i = 0; i + windowSize <= audioDouble.size(); i += hopSize) {
            segmenter.extractFeatures(audioDouble.data() + i, windowSize);
        }

        // Segment
        segmenter.segment(segCfg.num_clusters);

        // Get segmentation results
        const Segmentation& segmentation = segmenter.getSegmentation();

        result->num_segments = segmentation.segments.size();
        result->num_segment_types = segmentation.nsegtypes;
        result->segments = static_cast<AnalyzerSegment*>(
            malloc(sizeof(AnalyzerSegment) * result->num_segments));

        if (result->segments) {
            for (size_t i = 0; i < result->num_segments; ++i) {
                result->segments[i].start = static_cast<double>(
                    segmentation.segments[i].start) / analyzer->sampleRate;
                result->segments[i].end = static_cast<double>(
                    segmentation.segments[i].end) / analyzer->sampleRate;
                result->segments[i].type = segmentation.segments[i].type;
            }
        }
    }

    // === Generate Cue Points ===
    std::vector<CuePoint> cues;

    // Add phrase cues (every 4 or 8 bars based on downbeats)
    if (result->num_downbeats > 0 && result->downbeats) {
        int phraseBars = 8; // 8-bar phrases
        for (size_t i = 0; i < result->num_downbeats; i += phraseBars) {
            int beatIdx = result->downbeats[i];
            if (beatIdx >= 0 && static_cast<size_t>(beatIdx) < result->num_beats) {
                CuePoint cue;
                cue.time = result->beats[beatIdx];
                cue.type = CUE_TYPE_PHRASE;
                cue.type_index = static_cast<int>(i / phraseBars);
                cue.confidence = 0.8;
                cues.push_back(cue);
            }
        }
    }

    // Add section cues from segmentation
    if (result->num_segments > 0 && result->segments) {
        for (size_t i = 0; i < result->num_segments; ++i) {
            CuePoint cue;
            cue.time = result->segments[i].start;
            cue.type = CUE_TYPE_SECTION;
            cue.type_index = result->segments[i].type;
            cue.confidence = 0.7;
            cues.push_back(cue);
        }
    }

    // Sort cues by time
    std::sort(cues.begin(), cues.end(),
              [](const CuePoint& a, const CuePoint& b) { return a.time < b.time; });

    // Store cue points
    result->num_cue_points = cues.size();
    result->cue_points = static_cast<CuePoint*>(
        malloc(sizeof(CuePoint) * result->num_cue_points));
    if (result->cue_points) {
        std::copy(cues.begin(), cues.end(), result->cue_points);
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
    return "3.0.0-mixxx-qmdsp-full";
}

} // extern "C"
