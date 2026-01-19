// Package analyzer provides Go bindings for beat detection.
// This file provides audio file loading and processing utilities.
package analysis

import (
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

// loadMP3Mono loads an MP3 file and returns mono float32 samples.
func loadMP3Mono(path string) ([]float32, int, error) {
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

	for i := 0; i < numSamplePairs; i++ {
		offset := i * 4
		// Read left and right channels as signed 16-bit
		left := int16(binary.LittleEndian.Uint16(pcmData[offset:]))
		right := int16(binary.LittleEndian.Uint16(pcmData[offset+2:]))

		// Mix to mono and normalize to [-1, 1]
		mono := (float32(left) + float32(right)) / 2.0
		samples[i] = mono / 32768.0
	}

	return samples, sampleRate, nil
}
