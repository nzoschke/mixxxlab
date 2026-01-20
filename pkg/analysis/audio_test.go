package analysis

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/go-mp3"
)

// TestMP3Decoding analyzes MP3 decoding to help diagnose timing offset issues.
// Run with: go test -v -run TestMP3Decoding ./pkg/analysis/
func TestMP3Decoding(t *testing.T) {
	// Find a test MP3 file
	musicDir := "../../music"
	var testFile string

	err := filepath.Walk(musicDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && filepath.Ext(path) == ".mp3" && testFile == "" {
			testFile = path
		}
		return nil
	})

	if err != nil || testFile == "" {
		t.Skip("No MP3 files found in music directory")
	}

	t.Logf("Testing with file: %s", testFile)

	// Read LAME delay
	lameDelay := readLAMEEncoderDelay(testFile)
	t.Logf("LAME encoder delay from header: %d samples", lameDelay)

	totalDelay := readMP3Delay(testFile)
	t.Logf("Total delay (LAME + decoder): %d samples", totalDelay)

	// Load raw samples WITHOUT delay compensation
	f, err := os.Open(testFile)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	defer f.Close()

	// Decode using go-mp3 directly to get raw samples
	samples, sampleRate, err := loadMP3MonoRaw(testFile)
	if err != nil {
		t.Fatalf("Failed to decode: %v", err)
	}

	t.Logf("Sample rate: %d Hz", sampleRate)
	t.Logf("Total samples: %d (%.2f seconds)", len(samples), float64(len(samples))/float64(sampleRate))

	// Find first significant transient (above threshold)
	threshold := float32(0.1)
	firstTransient := -1
	for i, s := range samples {
		if s > threshold || s < -threshold {
			firstTransient = i
			break
		}
	}

	if firstTransient >= 0 {
		t.Logf("First transient (>%.1f) at sample %d = %.4f seconds",
			threshold, firstTransient, float64(firstTransient)/float64(sampleRate))
	} else {
		t.Log("No transient found above threshold")
	}

	// Analyze first few frames worth of samples
	t.Log("\nFirst 100ms of audio (sample magnitudes):")
	frameSamples := sampleRate / 10 // 100ms worth
	if frameSamples > len(samples) {
		frameSamples = len(samples)
	}

	// Find max in each 10ms chunk
	chunkSize := sampleRate / 100 // 10ms
	for i := 0; i < 10 && i*chunkSize < frameSamples; i++ {
		start := i * chunkSize
		end := start + chunkSize
		if end > len(samples) {
			end = len(samples)
		}

		maxVal := float32(0)
		for j := start; j < end; j++ {
			if samples[j] > maxVal {
				maxVal = samples[j]
			}
			if -samples[j] > maxVal {
				maxVal = -samples[j]
			}
		}

		startMs := float64(start) * 1000 / float64(sampleRate)
		t.Logf("  %5.1f-%5.1fms: max=%.4f", startMs, startMs+10, maxVal)
	}

	// Also test with delay compensation
	samplesWithDelay, _, err := LoadAudioMono(testFile)
	if err != nil {
		t.Fatalf("Failed to load with delay compensation: %v", err)
	}

	t.Logf("\nAfter delay compensation:")
	t.Logf("Samples removed: %d", len(samples)-len(samplesWithDelay))
	t.Logf("Time shifted: %.4f seconds", float64(len(samples)-len(samplesWithDelay))/float64(sampleRate))

	// Find the strongest transient in first 10 seconds and report its position
	t.Log("\n=== Finding strong transients for comparison ===")
	searchSamples := min(sampleRate*10, len(samples)) // First 10 seconds

	// Find the maximum amplitude position (raw samples)
	maxAmp := float32(0)
	maxPos := 0
	for i := 0; i < searchSamples; i++ {
		amp := samples[i]
		if amp < 0 {
			amp = -amp
		}
		if amp > maxAmp {
			maxAmp = amp
			maxPos = i
		}
	}
	t.Logf("Strongest transient (raw): sample %d = %.4f seconds (amp=%.4f)",
		maxPos, float64(maxPos)/float64(sampleRate), maxAmp)

	// Same for compensated samples
	searchCompensated := min(sampleRate*10, len(samplesWithDelay))
	maxAmpComp := float32(0)
	maxPosComp := 0
	for i := 0; i < searchCompensated; i++ {
		amp := samplesWithDelay[i]
		if amp < 0 {
			amp = -amp
		}
		if amp > maxAmpComp {
			maxAmpComp = amp
			maxPosComp = i
		}
	}
	t.Logf("Strongest transient (compensated): sample %d = %.4f seconds (amp=%.4f)",
		maxPosComp, float64(maxPosComp)/float64(sampleRate), maxAmpComp)

	t.Log("\n=== Compare with browser ===")
	t.Log("Open http://localhost:8080/src/audio-test.html and select the same file.")
	t.Log("The browser should report the strongest transient at the SAME time if delay compensation is correct.")
}

// loadMP3MonoRaw loads MP3 without any delay compensation (for testing)
func loadMP3MonoRaw(path string) ([]float32, int, error) {
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

	pcmData, err := io.ReadAll(decoder)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to decode MP3: %w", err)
	}

	numSamplePairs := len(pcmData) / 4
	samples := make([]float32, numSamplePairs)

	for i := range numSamplePairs {
		offset := i * 4
		left := int16(binary.LittleEndian.Uint16(pcmData[offset:]))
		right := int16(binary.LittleEndian.Uint16(pcmData[offset+2:]))
		mono := (float32(left) + float32(right)) / 2.0
		samples[i] = mono / 32768.0
	}

	return samples, sampleRate, nil
}
