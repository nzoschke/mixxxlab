// Package analyzer provides Go bindings for beat detection.
// This file provides audio file loading and processing utilities.
package analysis

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/hajimehoshi/go-mp3"
)

// LoadAudioMono loads an audio file and returns mono float32 samples and sample rate.
func LoadAudioMono(path string) ([]float32, int, error) {
	ext := strings.ToLower(filepath.Ext(path))

	switch ext {
	case ".mp3":
		return loadMP3Mono(path)
	default:
		return nil, 0, fmt.Errorf("unsupported audio format: %s", ext)
	}
}

// Additional samples that go-mp3 produces compared to browser's decoder
// Measured: browser first transient at 48446, go-mp3 at 50735
// LAME header said 1365, so go-mp3 adds: 50735 - 48446 - 1365 = 924 samples
const goMP3DecoderDelay = 924

// Default encoder delay if we can't read it from the LAME header
const defaultEncoderDelay = 576

// readMP3Delay reads the total delay to skip for an MP3 file.
// Combines LAME encoder delay (from header) + go-mp3 decoder delay.
func readMP3Delay(path string) int {
	lameDelay := readLAMEEncoderDelay(path)
	return lameDelay + goMP3DecoderDelay
}

// readLAMEEncoderDelay reads the encoder delay from LAME/Xing header if present.
func readLAMEEncoderDelay(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return defaultEncoderDelay
	}
	defer f.Close()

	// Read first 4KB which should contain any Xing/LAME header
	buf := make([]byte, 4096)
	n, err := f.Read(buf)
	if err != nil || n < 200 {
		return defaultEncoderDelay
	}
	buf = buf[:n]

	// Look for "LAME" marker in the Xing/Info header
	// The LAME header contains encoder delay at offset 21 from "LAME"
	lameIdx := bytes.Index(buf, []byte("LAME"))
	if lameIdx == -1 {
		return defaultEncoderDelay
	}

	// LAME header structure: at offset 21 from "LAME" is a 3-byte field
	// containing encoder delay (12 bits) and padding (12 bits)
	delayOffset := lameIdx + 21
	if delayOffset+3 > len(buf) {
		return defaultEncoderDelay
	}

	// Encoder delay is in the upper 12 bits of the 24-bit value
	b := buf[delayOffset : delayOffset+3]
	delay := (int(b[0]) << 4) | (int(b[1]) >> 4)

	// Sanity check - delay should be reasonable (typically 576-1152)
	if delay < 0 || delay > 4096 {
		return defaultEncoderDelay
	}

	return delay
}

// loadMP3Mono loads an MP3 file and returns mono float32 samples.
func loadMP3Mono(path string) ([]float32, int, error) {
	// Read total delay (encoder + decoder)
	totalDelay := readMP3Delay(path)

	f, err := os.Open(path)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()

	decoder, err := mp3.NewDecoder(f)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create MP3 decoder: %w", err)
	}

	sampleRate := decoder.SampleRate()

	// Read all PCM data (16-bit stereo interleaved)
	pcmData, err := io.ReadAll(decoder)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to decode MP3: %w", err)
	}

	// Convert to mono float32
	// MP3 decoder outputs 16-bit signed stereo (4 bytes per sample pair)
	numSamplePairs := len(pcmData) / 4
	samples := make([]float32, numSamplePairs)

	for i := range numSamplePairs {
		offset := i * 4
		// Read left and right channels as signed 16-bit
		left := int16(binary.LittleEndian.Uint16(pcmData[offset:]))
		right := int16(binary.LittleEndian.Uint16(pcmData[offset+2:]))

		// Mix to mono and normalize to [-1, 1]
		mono := (float32(left) + float32(right)) / 2.0
		samples[i] = mono / 32768.0
	}

	// Skip delay at the start to match browser audio playback
	// Browser decoders compensate for MP3 encoder delay automatically
	if len(samples) > totalDelay {
		samples = samples[totalDelay:]
	}

	return samples, sampleRate, nil
}
