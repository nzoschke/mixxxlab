cmake_minimum_required(VERSION 3.16)
project(mixxx_analyzer VERSION 1.0.0 LANGUAGES C CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_POSITION_INDEPENDENT_CODE ON)

# Find required packages
find_package(PkgConfig REQUIRED)
pkg_check_modules(SNDFILE REQUIRED sndfile)

# qm-dsp source files (subset needed for beat detection)
# The mixxx submodule is at the repo root level
set(QMDSP_DIR "${CMAKE_CURRENT_SOURCE_DIR}/../../../mixxx/lib/qm-dsp")

set(QMDSP_SOURCES
    ${QMDSP_DIR}/dsp/onsets/DetectionFunction.cpp
    ${QMDSP_DIR}/dsp/onsets/PeakPicking.cpp
    ${QMDSP_DIR}/dsp/phasevocoder/PhaseVocoder.cpp
    ${QMDSP_DIR}/dsp/signalconditioning/DFProcess.cpp
    ${QMDSP_DIR}/dsp/signalconditioning/FiltFilt.cpp
    ${QMDSP_DIR}/dsp/signalconditioning/Filter.cpp
    ${QMDSP_DIR}/dsp/signalconditioning/Framer.cpp
    ${QMDSP_DIR}/dsp/tempotracking/TempoTrack.cpp
    ${QMDSP_DIR}/dsp/tempotracking/TempoTrackV2.cpp
    ${QMDSP_DIR}/dsp/transforms/FFT.cpp
    ${QMDSP_DIR}/maths/Correlation.cpp
    ${QMDSP_DIR}/maths/MathUtilities.cpp
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
)

target_compile_definitions(mixxx_analyzer PRIVATE
    # kissfft needs to use double precision for qm-dsp
    kiss_fft_scalar=double
    # Disable assertions in release builds
    $<$<CONFIG:Release>:NDEBUG>
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

set_target_properties(mixxx_analyzer_static PROPERTIES
    OUTPUT_NAME mixxx_analyzer
)
