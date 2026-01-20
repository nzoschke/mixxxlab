cmake_minimum_required(VERSION 3.16)
project(mixxx_analyzer VERSION 3.0.0 LANGUAGES C CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_POSITION_INDEPENDENT_CODE ON)

# Find required packages
find_package(PkgConfig REQUIRED)
pkg_check_modules(SNDFILE REQUIRED sndfile)

# qm-dsp source files
# The mixxx submodule is at the repo root level
set(QMDSP_DIR "${CMAKE_CURRENT_SOURCE_DIR}/../../../mixxx/lib/qm-dsp")

set(QMDSP_SOURCES
    # Core transforms
    ${QMDSP_DIR}/dsp/transforms/FFT.cpp

    # Onset detection
    ${QMDSP_DIR}/dsp/onsets/DetectionFunction.cpp
    ${QMDSP_DIR}/dsp/onsets/PeakPicking.cpp
    ${QMDSP_DIR}/dsp/phasevocoder/PhaseVocoder.cpp

    # Signal conditioning
    ${QMDSP_DIR}/dsp/signalconditioning/DFProcess.cpp
    ${QMDSP_DIR}/dsp/signalconditioning/FiltFilt.cpp
    ${QMDSP_DIR}/dsp/signalconditioning/Filter.cpp
    ${QMDSP_DIR}/dsp/signalconditioning/Framer.cpp

    # Tempo tracking
    ${QMDSP_DIR}/dsp/tempotracking/TempoTrack.cpp
    ${QMDSP_DIR}/dsp/tempotracking/TempoTrackV2.cpp
    ${QMDSP_DIR}/dsp/tempotracking/DownBeat.cpp

    # Rate conversion (needed for DownBeat)
    ${QMDSP_DIR}/dsp/rateconversion/Decimator.cpp

    # Segmentation
    ${QMDSP_DIR}/dsp/segmentation/ClusterMeltSegmenter.cpp
    ${QMDSP_DIR}/dsp/segmentation/Segmenter.cpp
    ${QMDSP_DIR}/dsp/segmentation/cluster_segmenter.c
    ${QMDSP_DIR}/dsp/segmentation/cluster_melt.c

    # Chromagram/ConstantQ (needed for segmentation)
    ${QMDSP_DIR}/dsp/chromagram/ConstantQ.cpp
    ${QMDSP_DIR}/dsp/chromagram/Chromagram.cpp

    # MFCC (needed for segmentation)
    ${QMDSP_DIR}/dsp/mfcc/MFCC.cpp

    # HMM (needed for segmentation)
    ${QMDSP_DIR}/hmm/hmm.c

    # Window functions
    ${QMDSP_DIR}/base/Pitch.cpp

    # Math utilities
    ${QMDSP_DIR}/maths/Correlation.cpp
    ${QMDSP_DIR}/maths/MathUtilities.cpp
    ${QMDSP_DIR}/maths/pca/pca.c
    ${QMDSP_DIR}/maths/KLDivergence.cpp

    # KissFFT
    ${QMDSP_DIR}/ext/kissfft/kiss_fft.c
    ${QMDSP_DIR}/ext/kissfft/tools/kiss_fftr.c
)

# Build shared library
add_library(mixxx_analyzer SHARED
    analyzer.cpp
    ${QMDSP_SOURCES}
)

target_include_directories(mixxx_analyzer
    PUBLIC
        ${CMAKE_CURRENT_SOURCE_DIR}
    PRIVATE
        ${QMDSP_DIR}
        ${QMDSP_DIR}/include
        ${SNDFILE_INCLUDE_DIRS}
)

target_link_directories(mixxx_analyzer
    PRIVATE
        ${SNDFILE_LIBRARY_DIRS}
)

target_link_libraries(mixxx_analyzer
    PRIVATE
        ${SNDFILE_LIBRARIES}
        "-framework Accelerate"  # Provides BLAS/LAPACK on macOS
)

target_compile_definitions(mixxx_analyzer PRIVATE
    # kissfft needs to use double precision for qm-dsp
    kiss_fft_scalar=double
    # Disable assertions in release builds
    $<$<CONFIG:Release>:NDEBUG>
)

# Suppress warnings from qm-dsp code
target_compile_options(mixxx_analyzer PRIVATE
    -Wno-deprecated-declarations
    -Wno-unused-variable
    -Wno-unused-parameter
)

# Install targets
install(TARGETS mixxx_analyzer
    LIBRARY DESTINATION lib
    ARCHIVE DESTINATION lib
)

install(FILES analyzer.h
    DESTINATION include
)

# Build static library as well for easier linking
add_library(mixxx_analyzer_static STATIC
    analyzer.cpp
    ${QMDSP_SOURCES}
)

target_include_directories(mixxx_analyzer_static
    PUBLIC
        ${CMAKE_CURRENT_SOURCE_DIR}
    PRIVATE
        ${QMDSP_DIR}
        ${QMDSP_DIR}/include
        ${SNDFILE_INCLUDE_DIRS}
)

target_link_directories(mixxx_analyzer_static
    PRIVATE
        ${SNDFILE_LIBRARY_DIRS}
)

target_link_libraries(mixxx_analyzer_static
    PRIVATE
        ${SNDFILE_LIBRARIES}
)

target_compile_definitions(mixxx_analyzer_static PRIVATE
    kiss_fft_scalar=double
)

target_compile_options(mixxx_analyzer_static PRIVATE
    -Wno-deprecated-declarations
    -Wno-unused-variable
    -Wno-unused-parameter
)

set_target_properties(mixxx_analyzer_static PROPERTIES
    OUTPUT_NAME mixxx_analyzer
)
