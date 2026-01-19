package analyzer

import (
	"math"

	"gonum.org/v1/gonum/dsp/fourier"
)

// STFTConfig describes parameters for STFT computation.
type GoSTFTConfig struct {
	FFTSize    int // FFT window size (e.g., 1024, 2048, 4096)
	HopSize    int // Hop between frames (e.g., 441 for 10ms at 44100Hz)
	WindowSize int // Analysis window size (usually same as FFTSize)
}

// DefaultSTFTConfigs returns the 3 STFT configurations used by the beat detection model.
func DefaultSTFTConfigs() []GoSTFTConfig {
	return []GoSTFTConfig{
		{FFTSize: 1024, HopSize: 441, WindowSize: 1024},
		{FFTSize: 2048, HopSize: 441, WindowSize: 2048},
		{FFTSize: 4096, HopSize: 441, WindowSize: 4096},
	}
}

// STFT computes Short-Time Fourier Transform.
// Returns [frames][bins] magnitude spectrum.
func STFT(samples []float64, cfg GoSTFTConfig) [][]float64 {
	window := hannWindow(cfg.WindowSize)
	fft := fourier.NewFFT(cfg.FFTSize)

	// Number of output frames
	numFrames := (len(samples) - cfg.WindowSize) / cfg.HopSize
	if numFrames <= 0 {
		return nil
	}

	// Number of frequency bins (RFFT output)
	numBins := cfg.FFTSize/2 + 1

	result := make([][]float64, numFrames)
	frame := make([]float64, cfg.FFTSize)

	for i := 0; i < numFrames; i++ {
		start := i * cfg.HopSize

		// Clear frame and apply window
		for j := range frame {
			frame[j] = 0
		}
		for j := 0; j < cfg.WindowSize && start+j < len(samples); j++ {
			frame[j] = samples[start+j] * window[j]
		}

		// Compute FFT
		coeffs := fft.Coefficients(nil, frame)

		// Extract magnitude for positive frequencies only (RFFT)
		// Normalize: 2/N for one-sided spectrum (except DC and Nyquist)
		// scipy uses this normalization for single-sided spectra
		scale := 2.0 / float64(cfg.FFTSize)
		result[i] = make([]float64, numBins)
		for j := 0; j < numBins; j++ {
			re := real(coeffs[j])
			im := imag(coeffs[j])
			s := scale
			if j == 0 || j == numBins-1 {
				s = 1.0 / float64(cfg.FFTSize) // DC and Nyquist aren't doubled
			}
			result[i][j] = math.Sqrt(re*re+im*im) * s
		}
	}

	return result
}

// STFTComplex computes STFT and returns complex coefficients [frames][bins][2] (real, imag).
func STFTComplex(samples []float64, cfg GoSTFTConfig) [][][2]float64 {
	window := hannWindow(cfg.WindowSize)
	fft := fourier.NewFFT(cfg.FFTSize)

	numFrames := (len(samples) - cfg.WindowSize) / cfg.HopSize
	if numFrames <= 0 {
		return nil
	}

	numBins := cfg.FFTSize/2 + 1

	result := make([][][2]float64, numFrames)
	frame := make([]float64, cfg.FFTSize)

	for i := 0; i < numFrames; i++ {
		start := i * cfg.HopSize

		for j := range frame {
			frame[j] = 0
		}
		for j := 0; j < cfg.WindowSize && start+j < len(samples); j++ {
			frame[j] = samples[start+j] * window[j]
		}

		coeffs := fft.Coefficients(nil, frame)

		// Normalize: 2/N for one-sided spectrum (except DC and Nyquist)
		scale := 2.0 / float64(cfg.FFTSize)
		result[i] = make([][2]float64, numBins)
		for j := 0; j < numBins; j++ {
			s := scale
			if j == 0 || j == numBins-1 {
				s = 1.0 / float64(cfg.FFTSize)
			}
			result[i][j][0] = real(coeffs[j]) * s
			result[i][j][1] = imag(coeffs[j]) * s
		}
	}

	return result
}

// hannWindow generates a Hann window of given size.
func hannWindow(size int) []float64 {
	w := make([]float64, size)
	for i := range w {
		w[i] = 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/float64(size-1)))
	}
	return w
}

// ComputeMultiScaleSTFT computes STFT at multiple scales as used by the beat detector.
// Returns 3 magnitude spectrograms for FFT sizes 1024, 2048, 4096.
func ComputeMultiScaleSTFT(samples []float64) [3][][]float64 {
	configs := DefaultSTFTConfigs()
	var result [3][][]float64
	for i, cfg := range configs {
		result[i] = STFT(samples, cfg)
	}
	return result
}
