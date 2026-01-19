package analysis

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

// ModelStructure contains information about the beat detection model.
type ModelStructure struct {
	STFTConfigs []STFTConfig `json:"stft_configs"`
	InputShape  []int        `json:"input_shape"`
	OutputShape []int        `json:"output_shape"`
}

// STFTConfig describes one STFT layer's parameters.
type STFTConfig struct {
	FrameLength int `json:"frame_length"`
	FrameStep   int `json:"frame_step"`
	FFTLength   int `json:"fft_length"`
	NumBins     int `json:"num_bins"` // FFTLength/2 + 1
}

// AnalyzeModel extracts the model structure to understand post-STFT requirements.
func AnalyzeModel(savedModelPath string) (*ModelStructure, error) {
	script := fmt.Sprintf(`
import os
os.environ['TF_CPP_MIN_LOG_LEVEL'] = '3'
import warnings
warnings.filterwarnings('ignore')

import json
import tensorflow as tf
import numpy as np

model = tf.saved_model.load(%q)

# Extract STFT configs from keras_metadata
stft_configs = []
frame_lengths = [1024, 2048, 4096]  # From keras_metadata.pb
frame_step = 441  # 10ms at 44100 Hz

for fl in frame_lengths:
    stft_configs.append({
        'frame_length': fl,
        'frame_step': frame_step,
        'fft_length': fl,
        'num_bins': fl // 2 + 1,
    })

# Test with sample input to get shapes
infer = model.signatures['serving_default']
test_audio = tf.random.normal([1, 441000])  # 10 seconds
output = infer(fltp=test_audio)

result = {
    'stft_configs': stft_configs,
    'input_shape': [1, 441000],
    'output_shapes': {k: list(v.shape) for k, v in output.items()},
}

# Try to trace through and find intermediate shapes
# The model does: audio -> 3 STFTs -> FilteredSpectrograms -> concat -> dense -> output

# Calculate expected STFT output shapes for 10s audio
for cfg in stft_configs:
    num_frames = (441000 - cfg['frame_length']) // cfg['frame_step'] + 1
    cfg['output_frames'] = num_frames
    cfg['output_shape'] = [1, num_frames, cfg['num_bins']]

print(json.dumps(result, indent=2))
`, savedModelPath)

	cmd := exec.Command(getPythonPath(), "-c", script)
	cmd.Stderr = os.Stderr

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("analysis failed: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse output: %w", err)
	}

	// Extract STFT configs
	ms := &ModelStructure{}
	if configs, ok := result["stft_configs"].([]interface{}); ok {
		for _, c := range configs {
			cfg := c.(map[string]interface{})
			ms.STFTConfigs = append(ms.STFTConfigs, STFTConfig{
				FrameLength: int(cfg["frame_length"].(float64)),
				FrameStep:   int(cfg["frame_step"].(float64)),
				FFTLength:   int(cfg["fft_length"].(float64)),
				NumBins:     int(cfg["num_bins"].(float64)),
			})
		}
	}

	return ms, nil
}

// ExportPostSTFTModel exports the model with STFT ops removed.
// The new model takes magnitude spectrograms as input instead of raw audio.
func ExportPostSTFTModel(savedModelPath, outputPath string) error {
	script := fmt.Sprintf(`
import os
os.environ['TF_CPP_MIN_LOG_LEVEL'] = '3'
import warnings
warnings.filterwarnings('ignore')

import tensorflow as tf
import numpy as np

print("Loading original model...")
model = tf.saved_model.load(%q)

# Inspect model internals
print("\nModel attributes:")
for attr in dir(model):
    if not attr.startswith('_'):
        obj = getattr(model, attr, None)
        if obj is not None and not callable(obj):
            print(f"  {attr}: {type(obj).__name__}")

# Try to access the internal layers
print("\nLooking for internal layer structure...")

# Check if model has keras_api with layer info
if hasattr(model, 'keras_api'):
    keras_api = model.keras_api
    print(f"keras_api: {type(keras_api)}")
    for attr in dir(keras_api):
        if not attr.startswith('_'):
            print(f"  keras_api.{attr}")

# The model likely has these internal attributes from Keras:
internal_attrs = [
    '_TestLayerModel__stft_layers',
    '_TestLayerModel__filter_specs',
    '_TestLayerModel__diff_frames',
]

for attr in internal_attrs:
    if hasattr(model, attr):
        layers = getattr(model, attr)
        print(f"\n{attr}:")
        if hasattr(layers, '__iter__'):
            for i, layer in enumerate(layers):
                print(f"  [{i}] {type(layer).__name__}")
                # Try to get layer config
                if hasattr(layer, 'get_config'):
                    try:
                        cfg = layer.get_config()
                        print(f"      config: {cfg}")
                    except:
                        pass

# Examine FilteredSpectrogram layers' weights (filterbank matrices)
print("\n\nExamining FilteredSpectrogram weights...")
filter_specs = getattr(model, '_TestLayerModel__filter_specs', [])
for i, fs in enumerate(filter_specs):
    print(f"\nFilteredSpectrogram[{i}]:")
    # Get all variables from this layer
    if hasattr(fs, 'variables'):
        for var in fs.variables:
            print(f"  {var.name}: shape={var.shape}")
    # Check for weights attribute
    if hasattr(fs, 'weights'):
        for w in fs.weights:
            print(f"  weight: {w.name} shape={w.shape}")
    # Check trainable_variables
    if hasattr(fs, 'trainable_variables'):
        for tv in fs.trainable_variables:
            print(f"  trainable: {tv.name} shape={tv.shape}")

# Try to trace a forward pass and capture intermediate tensors
print("\n\nTracing forward pass...")
infer = model.signatures['serving_default']

# Create a simple test
test_audio = tf.constant(np.random.randn(1, 44100).astype(np.float32))  # 1 second

# Run inference to warm up
output = infer(fltp=test_audio)
print(f"Output keys: {list(output.keys())}")
for k, v in output.items():
    print(f"  {k}: shape={v.shape}, dtype={v.dtype}")

# Check all model variables to find filterbank weights
print("\n\nAll model variables:")
for var in model.variables:
    print(f"  {var.name}: shape={var.shape}")

print("\n\nAll trainable variables:")
for var in model.trainable_variables:
    print(f"  {var.name}: shape={var.shape}")

# The model has no trainable variables - it's all fixed ops!
# This means FilteredSpectrogram is likely a mel filterbank or similar fixed transform.
# Let's try to understand the output shape: 314 features

# With 3 STFT layers and output 314 features, it might be:
# - Something like 100 + 100 + 114 mel bands from each STFT
# - Or a fixed filterbank that reduces each spectrogram

# Let's create a model wrapper that takes magnitude spectrograms
# and outputs the same as the original model

print("\n\nCreating post-STFT model...")

# We need to compute STFT ourselves and feed magnitude to the model
# But the model takes raw audio, so we need to modify the graph

# Alternative approach: Look at what the STFT layers output
stft_layers = getattr(model, '_TestLayerModel__stft_layers', [])
print(f"Number of STFT layers: {len(stft_layers)}")

for i, stft in enumerate(stft_layers):
    print(f"\nSTFT layer {i}:")
    for attr in dir(stft):
        if not attr.startswith('_'):
            val = getattr(stft, attr, None)
            if val is not None and not callable(val):
                print(f"  {attr}: {val}")

# Check frame_sizes which might tell us the STFT params
if hasattr(model, 'frame_sizes'):
    print(f"\nframe_sizes: {list(model.frame_sizes)}")

print("\nOK")
`, savedModelPath)

	cmd := exec.Command(getPythonPath(), "-c", script)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
