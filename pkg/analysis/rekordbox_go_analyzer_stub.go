//go:build !tensorflow

// Package analyzer provides Go bindings for beat detection.
// This file provides stubs when TensorFlow is not available.
package analysis

import "fmt"

// TFAnalyzer is a stub when TensorFlow is not available.
type TFAnalyzer struct{}

// NewTFAnalyzer returns an error when TensorFlow is not available.
func NewTFAnalyzer() (*TFAnalyzer, error) {
	return nil, fmt.Errorf("TensorFlow support not compiled (build with -tags=tensorflow)")
}

// NewTFAnalyzerWithModel returns an error when TensorFlow is not available.
func NewTFAnalyzerWithModel(modelPath string) (*TFAnalyzer, error) {
	return nil, fmt.Errorf("TensorFlow support not compiled (build with -tags=tensorflow)")
}

// Close is a no-op for the stub.
func (a *TFAnalyzer) Close() error {
	return nil
}

// AnalyzeFile returns an error when TensorFlow is not available.
func (a *TFAnalyzer) AnalyzeFile(audioPath string) (*MLAnalyzeOut, error) {
	return nil, fmt.Errorf("TensorFlow support not compiled (build with -tags=tensorflow)")
}

// AnalyzeSamples returns an error when TensorFlow is not available.
func (a *TFAnalyzer) AnalyzeSamples(samples []float32, sampleRate int) (*MLAnalyzeOut, error) {
	return nil, fmt.Errorf("TensorFlow support not compiled (build with -tags=tensorflow)")
}
