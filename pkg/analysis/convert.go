package analysis

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ConvertToONNX converts a TensorFlow SavedModel to ONNX format.
// Uses opset 17+ which has native STFT/DFT support.
//
// NOTE: The beat detection models use RFFT in a pattern that tf2onnx cannot convert:
// RFFT → StridedSlice (to extract real/imag parts). tf2onnx only supports RFFT → ComplexAbs.
// The ONNX file is created but contains unsupported RFFT ops that ONNX Runtime cannot execute.
// Use MLAnalyzer (Python subprocess) for inference instead.
func ConvertToONNX(savedModelPath, outputPath string, opset int) error {
	if opset < 17 {
		opset = 17 // Minimum for STFT support
	}

	script := fmt.Sprintf(`
import os
os.environ['TF_CPP_MIN_LOG_LEVEL'] = '3'
import warnings
warnings.filterwarnings('ignore')

import tf2onnx
from tf2onnx import tf_loader

print(f"tf2onnx version: {tf2onnx.__version__}")

# Check available extra opsets
try:
    from tf2onnx import constants
    print(f"Available domains: {list(constants.OPSET_TO_IR_VERSION.keys())[:5]}...")
except:
    pass

graph_def, inputs, outputs = tf_loader.from_saved_model(
    %q, None, None,
    tag='serve',
    signatures=['serving_default'],
)

model_proto, _ = tf2onnx.convert.from_graph_def(
    graph_def,
    input_names=inputs,
    output_names=outputs,
    opset=%d,
    output_path=%q,
)

print("OK")
`, savedModelPath, opset, outputPath)

	pythonPath := getPythonPath()
	cmd := exec.Command(pythonPath, "-c", script)
	cmd.Stderr = os.Stderr

	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("conversion failed: %w", err)
	}

	// Check that output ends with OK (may have other debug output before it)
	if !strings.Contains(string(output), "OK") {
		return fmt.Errorf("conversion may have failed: %s", output)
	}

	return nil
}

// VerifyONNX loads an ONNX model and runs a test inference.
func VerifyONNX(onnxPath string, inputLength int) (map[string][]int, error) {
	script := fmt.Sprintf(`
import os
os.environ['TF_CPP_MIN_LOG_LEVEL'] = '3'
import warnings
warnings.filterwarnings('ignore')

import onnxruntime as ort
import numpy as np
import json

print(f"ONNX Runtime version: {ort.__version__}")

# Try loading with different providers
providers = ['CPUExecutionProvider']

# Check for custom ops support
try:
    from onnxruntime_extensions import get_library_path
    so = ort.SessionOptions()
    so.register_custom_ops_library(get_library_path())
    session = ort.InferenceSession(%q, so, providers=providers)
    print("Loaded with onnxruntime-extensions")
except ImportError:
    session = ort.InferenceSession(%q, providers=providers)
    print("Loaded without extensions")

# Get model info
info = {
    'inputs': {i.name: list(i.shape) for i in session.get_inputs()},
    'outputs': {o.name: list(o.shape) for o in session.get_outputs()},
}
print(f"Inputs: {info['inputs']}")
print(f"Outputs: {info['outputs']}")

# Test inference
test_input = np.random.randn(1, %d).astype(np.float32)
outputs = session.run(None, {session.get_inputs()[0].name: test_input})
info['output_shapes'] = [list(o.shape) for o in outputs]
print(f"Output shapes: {info['output_shapes']}")

print(json.dumps(info))
`, onnxPath, onnxPath, inputLength)

	pythonPath := getPythonPath()
	cmd := exec.Command(pythonPath, "-c", script)
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("verification failed: %w", err)
	}

	result := make(map[string][]int)
	result["status"] = []int{1} // OK
	return result, nil
}

// EnsureConvertDeps ensures the conversion dependencies are up to date.
func EnsureConvertDeps() error {
	script := `
import subprocess
import sys

# Upgrade tf2onnx to latest for better STFT/DFT support
subprocess.check_call([sys.executable, '-m', 'pip', 'install', '-q', '--upgrade', 'tf2onnx'])

# Install onnxruntime-extensions for custom ops
subprocess.check_call([sys.executable, '-m', 'pip', 'install', '-q', 'onnxruntime-extensions'])

import tf2onnx
import onnxruntime as ort
print(f"tf2onnx: {tf2onnx.__version__}")
print(f"onnxruntime: {ort.__version__}")
print("OK")
`
	pythonPath := getPythonPath()
	cmd := exec.Command(pythonPath, "-c", script)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// getPythonPath returns the path to Python in the convert venv.
func getPythonPath() string {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return "python3"
	}
	baseDir := filepath.Dir(filepath.Dir(currentFile))

	// Try convert venv first (has tf2onnx with compatible deps)
	convertVenv := filepath.Join(baseDir, ".venv-convert", "bin", "python")
	if _, err := os.Stat(convertVenv); err == nil {
		return convertVenv
	}

	// Fall back to main venv
	mainVenv := filepath.Join(baseDir, ".venv", "bin", "python")
	if _, err := os.Stat(mainVenv); err == nil {
		return mainVenv
	}

	return "python3"
}
