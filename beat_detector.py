#!/usr/bin/env -S uv run
# /// script
# requires-python = ">=3.9"
# dependencies = [
#     "numpy",
#     "scipy",
#     "soundfile",
#     "tensorflow>=2.15",
# ]
# ///
"""
Beat detection using TensorFlow models.

This module provides beat detection functionality using pre-trained
TensorFlow SavedModel models. It takes raw audio and outputs beat
timestamps and estimated BPM.

Usage:
    uv run beat_detector.py [--model PATH] [--json] audio_file
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import warnings
from pathlib import Path
from typing import NamedTuple

# Suppress TensorFlow and other warnings for cleaner output
os.environ['TF_CPP_MIN_LOG_LEVEL'] = '3'
warnings.filterwarnings('ignore')

import numpy as np
import soundfile as sf
import tensorflow as tf
from scipy import signal
from scipy.signal import find_peaks

# Suppress TensorFlow logging
tf.get_logger().setLevel('ERROR')

# Default model path in rekordbox
REKORDBOX_MODELS = "/Applications/rekordbox 7/rekordbox.app/Contents/Resources/models"


class BeatDetectionResult(NamedTuple):
    """Result of beat detection analysis."""
    bpm: float
    beats: list[float]  # Beat timestamps in seconds
    sample_rate: int
    duration: float
    total_frames: int


class BeatDetector:
    """Beat detector using TensorFlow models."""

    # Model expects 44100 Hz sample rate with 441 sample hop (10ms)
    TARGET_SAMPLE_RATE = 44100
    HOP_SIZE = 441  # 10ms at 44100 Hz

    def __init__(self, model_path: str | Path):
        """
        Initialize the beat detector.

        Args:
            model_path: Path to the TensorFlow SavedModel directory.
        """
        self.model_path = Path(model_path)
        self.model = tf.saved_model.load(str(self.model_path))
        self._infer = self.model.signatures.get('serving_default')
        if self._infer is None:
            # Try to use the model directly as a callable
            self._infer = self.model

    def load_audio(self, audio_path: str | Path) -> tuple[np.ndarray, int]:
        """
        Load audio file and convert to mono float32.

        Args:
            audio_path: Path to the audio file.

        Returns:
            Tuple of (audio samples as float32, sample rate).
        """
        audio, sr = sf.read(audio_path, dtype='float32')

        # Convert to mono if stereo
        if audio.ndim > 1:
            audio = np.mean(audio, axis=1)

        return audio, sr

    def resample(self, audio: np.ndarray, orig_sr: int, target_sr: int) -> np.ndarray:
        """
        Resample audio to target sample rate.

        Args:
            audio: Audio samples.
            orig_sr: Original sample rate.
            target_sr: Target sample rate.

        Returns:
            Resampled audio.
        """
        if orig_sr == target_sr:
            return audio

        # Calculate resampling ratio
        ratio = target_sr / orig_sr
        new_length = int(len(audio) * ratio)

        # Use scipy's resample for high-quality resampling
        resampled = signal.resample(audio, new_length)
        return resampled.astype(np.float32)

    def detect_beats(self, audio_path: str | Path) -> BeatDetectionResult:
        """
        Detect beats in an audio file.

        Args:
            audio_path: Path to the audio file.

        Returns:
            BeatDetectionResult with BPM, beat timestamps, and metadata.
        """
        # Load and preprocess audio
        audio, orig_sr = self.load_audio(audio_path)
        duration = len(audio) / orig_sr
        total_frames = len(audio)

        # Resample to target sample rate
        audio = self.resample(audio, orig_sr, self.TARGET_SAMPLE_RATE)

        # Run inference
        # Model expects shape (batch, samples)
        audio_tensor = tf.constant(audio[np.newaxis, :], dtype=tf.float32)

        try:
            # Try calling with named input
            output = self._infer(fltp=audio_tensor)
        except TypeError:
            # Fall back to positional argument
            output = self._infer(audio_tensor)

        # Extract beat activation from output
        # Model outputs: output_1 (spectrogram features), output_2 (beat activation with 2 columns)
        # Column 0 of output_2 is the beat activation
        if isinstance(output, dict):
            if 'output_2' in output:
                activation = output['output_2'].numpy().squeeze()[:, 0]
            else:
                activation = list(output.values())[0].numpy().squeeze()
        else:
            activation = output.numpy().squeeze()

        # Post-process to find beat positions
        beats = self._find_beats(activation)

        # Convert frame indices to timestamps
        beat_times = [b * self.HOP_SIZE / self.TARGET_SAMPLE_RATE for b in beats]

        # Estimate BPM from beat intervals
        bpm = self._estimate_bpm(beat_times)

        return BeatDetectionResult(
            bpm=bpm,
            beats=beat_times,
            sample_rate=orig_sr,
            duration=duration,
            total_frames=total_frames,
        )

    def _find_beats(
        self,
        activation: np.ndarray,
        threshold: float = 0.1,
        min_distance: int = 40,
    ) -> list[int]:
        """
        Find beat positions from activation function.

        Uses scipy's find_peaks with minimum distance constraint.

        Args:
            activation: Beat activation function from model (values in [0, 1]).
            threshold: Minimum activation threshold for beats.
            min_distance: Minimum distance between peaks in frames.
                         40 frames = 400ms at 10ms hop, allowing up to 150 BPM.

        Returns:
            List of beat frame indices.
        """
        # Find peaks with height threshold, minimum distance, and prominence
        peaks, _ = find_peaks(
            activation,
            height=threshold,
            distance=min_distance,
            prominence=0.05,
        )

        return peaks.tolist()

    def _estimate_bpm(self, beat_times: list[float]) -> float:
        """
        Estimate BPM from beat timestamps.

        Args:
            beat_times: List of beat timestamps in seconds.

        Returns:
            Estimated BPM.
        """
        if len(beat_times) < 2:
            return 0.0

        # Calculate inter-beat intervals
        intervals = np.diff(beat_times)

        # Filter out outliers (intervals outside reasonable BPM range 60-200)
        min_interval = 60 / 200  # 200 BPM
        max_interval = 60 / 60   # 60 BPM

        valid_intervals = intervals[(intervals >= min_interval) & (intervals <= max_interval)]

        if len(valid_intervals) == 0:
            # Fall back to median of all intervals
            median_interval = np.median(intervals)
        else:
            median_interval = np.median(valid_intervals)

        if median_interval > 0:
            bpm = 60.0 / median_interval
        else:
            bpm = 0.0

        return round(bpm, 2)


def main():
    """CLI entry point."""
    parser = argparse.ArgumentParser(
        description='Detect beats in audio files using ML models.'
    )
    parser.add_argument(
        'audio_file',
        help='Path to the audio file to analyze.'
    )
    parser.add_argument(
        '--model',
        default=f'{REKORDBOX_MODELS}/detect_beat/model8',
        help=f'Path to the TensorFlow SavedModel directory (default: rekordbox model8).'
    )
    parser.add_argument(
        '--json',
        action='store_true',
        help='Output results as JSON.'
    )
    parser.add_argument(
        '--beats-only',
        action='store_true',
        help='Only output beat timestamps, one per line.'
    )

    args = parser.parse_args()
    model_path = Path(args.model)

    # Verify model exists
    if not model_path.exists():
        print(f'Error: Model not found at {model_path}', file=sys.stderr)
        print('rekordbox 7 must be installed.', file=sys.stderr)
        sys.exit(1)

    try:
        detector = BeatDetector(model_path)
        result = detector.detect_beats(args.audio_file)

        if args.beats_only:
            for beat in result.beats:
                print(f'{beat:.6f}')
        elif args.json:
            output = {
                'bpm': result.bpm,
                'beats': result.beats,
                'sample_rate': result.sample_rate,
                'duration': result.duration,
                'total_frames': result.total_frames,
                'num_beats': len(result.beats),
                'bars': len(result.beats) / 4.0,
            }
            print(json.dumps(output, indent=2))
        else:
            print(f'BPM: {result.bpm:.2f}')
            print(f'Beats: {len(result.beats)}')
            print(f'Bars: {len(result.beats) / 4:.1f}')
            print(f'Duration: {result.duration:.2f}s')
            print(f'Sample rate: {result.sample_rate} Hz')
            if result.beats:
                print(f'First 5 beats: {", ".join(f"{b:.3f}" for b in result.beats[:5])}')

    except Exception as e:
        print(f'Error: {e}', file=sys.stderr)
        sys.exit(1)


if __name__ == '__main__':
    main()
