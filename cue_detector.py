#!/usr/bin/env -S uv run
# /// script
# requires-python = ">=3.9"
# dependencies = [
#     "numpy",
#     "scipy",
#     "soundfile",
#     "onnxruntime",
# ]
# ///
"""
Cue point detection using SampleCNN features.

Instead of using Rekordbox's opaque cue_prediction_model, we use the
samplecnn features to detect cue points based on:
1. Energy changes (drops and rises)
2. Feature distance (section boundaries)
3. First significant audio after silence

This gives us interpretable, controllable cue point detection.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
from pathlib import Path
from typing import NamedTuple

import numpy as np
import soundfile as sf
from scipy import signal
from scipy.signal import find_peaks
import onnxruntime as ort


REKORDBOX_MODELS = "/Applications/rekordbox 7/rekordbox.app/Contents/Resources/models"
TARGET_SR = 44100
SEGMENT_LENGTH = 59049  # ~1.34 seconds at 44100 Hz
SEGMENT_DURATION = SEGMENT_LENGTH / TARGET_SR


class CuePoint(NamedTuple):
    """A detected cue point."""
    time: float
    type: str  # 'intro', 'drop', 'breakdown', 'buildup', 'outro'
    confidence: float
    name: str


class CueDetector:
    """Detect cue points using SampleCNN audio features."""

    def __init__(self, model_path: str | Path | None = None):
        if model_path is None:
            model_path = f"{REKORDBOX_MODELS}/detect_cue_model/samplecnn.onnx"

        self.model = ort.InferenceSession(str(model_path))

    def load_audio(self, audio_path: str | Path) -> tuple[np.ndarray, float]:
        """Load and preprocess audio."""
        audio, sr = sf.read(audio_path, dtype='float32')
        if audio.ndim > 1:
            audio = np.mean(audio, axis=1)

        if sr != TARGET_SR:
            ratio = TARGET_SR / sr
            new_length = int(len(audio) * ratio)
            audio = signal.resample(audio, new_length).astype(np.float32)

        duration = len(audio) / TARGET_SR
        return audio, duration

    def extract_features(self, audio: np.ndarray, hop_seconds: float = 0.5) -> tuple[np.ndarray, np.ndarray, np.ndarray]:
        """
        Extract features from audio at regular intervals.

        Returns:
            times: Time positions for each feature vector
            features: Feature matrix (num_segments, 512)
            tags: Tag matrix (num_segments, 50)
        """
        hop_samples = int(hop_seconds * TARGET_SR)
        num_segments = (len(audio) - SEGMENT_LENGTH) // hop_samples + 1

        times = []
        features = []
        tags = []

        for i in range(num_segments):
            start = i * hop_samples
            end = start + SEGMENT_LENGTH

            if end > len(audio):
                break

            segment = audio[start:end].reshape(1, -1).astype(np.float32)
            outputs = self.model.run(None, {'waveform': segment})

            times.append(start / TARGET_SR)
            tags.append(outputs[0][0])
            features.append(outputs[1][0])

        return np.array(times), np.array(features), np.array(tags)

    def compute_energy(self, audio: np.ndarray, hop_seconds: float = 0.5) -> tuple[np.ndarray, np.ndarray]:
        """Compute RMS energy over time."""
        hop_samples = int(hop_seconds * TARGET_SR)
        window_samples = int(1.0 * TARGET_SR)  # 1 second window

        times = []
        energies = []

        for i in range(0, len(audio) - window_samples, hop_samples):
            window = audio[i:i + window_samples]
            rms = np.sqrt(np.mean(window ** 2))
            times.append(i / TARGET_SR)
            energies.append(rms)

        return np.array(times), np.array(energies)

    def detect_cue_points(
        self,
        audio_path: str | Path,
        max_cues: int = 8,
        min_distance: float = 8.0,
    ) -> list[CuePoint]:
        """
        Detect cue points in an audio file.

        Args:
            audio_path: Path to audio file
            max_cues: Maximum number of cue points to return
            min_distance: Minimum seconds between cue points

        Returns:
            List of CuePoint objects
        """
        audio, duration = self.load_audio(audio_path)

        # Extract features and energy
        times, features, tags = self.extract_features(audio, hop_seconds=0.5)
        energy_times, energy = self.compute_energy(audio, hop_seconds=0.5)

        # Normalize energy
        energy = energy / (np.max(energy) + 1e-8)

        # Compute feature distance (how different each segment is from neighbors)
        feature_dist = np.zeros(len(features))
        for i in range(1, len(features) - 1):
            dist_prev = np.linalg.norm(features[i] - features[i-1])
            dist_next = np.linalg.norm(features[i] - features[i+1])
            feature_dist[i] = (dist_prev + dist_next) / 2

        # Normalize
        feature_dist = feature_dist / (np.max(feature_dist) + 1e-8)

        # Compute energy derivative (for drops and rises)
        energy_interp = np.interp(times, energy_times, energy)
        energy_diff = np.gradient(energy_interp)

        # Find potential cue points
        candidates = []

        # 1. Find intro point (first significant audio)
        for i, (t, e) in enumerate(zip(times, energy_interp)):
            if e > 0.1:  # First point with > 10% energy
                candidates.append({
                    'time': t,
                    'type': 'intro',
                    'score': 1.0,
                    'name': 'Intro'
                })
                break

        # 2. Find drops (energy increases after low energy)
        drop_score = np.zeros(len(times))
        for i in range(5, len(times)):
            # Look for: low energy followed by high energy
            prev_energy = np.mean(energy_interp[max(0, i-5):i])
            curr_energy = energy_interp[i]
            if prev_energy < 0.3 and curr_energy > 0.5:
                drop_score[i] = (curr_energy - prev_energy) * feature_dist[i]

        drop_peaks, _ = find_peaks(drop_score, height=0.1, distance=int(min_distance / 0.5))
        for peak in drop_peaks:
            candidates.append({
                'time': float(times[peak]),
                'type': 'drop',
                'score': float(drop_score[peak]),
                'name': 'Drop'
            })

        # 3. Find breakdowns (energy decreases significantly)
        breakdown_score = np.zeros(len(times))
        for i in range(5, len(times)):
            prev_energy = np.mean(energy_interp[max(0, i-5):i])
            curr_energy = energy_interp[i]
            if prev_energy > 0.5 and curr_energy < 0.3:
                breakdown_score[i] = (prev_energy - curr_energy) * feature_dist[i]

        breakdown_peaks, _ = find_peaks(breakdown_score, height=0.1, distance=int(min_distance / 0.5))
        for peak in breakdown_peaks:
            candidates.append({
                'time': float(times[peak]),
                'type': 'breakdown',
                'score': float(breakdown_score[peak]),
                'name': 'Breakdown'
            })

        # 4. Find section boundaries (high feature distance)
        section_peaks, _ = find_peaks(feature_dist, height=0.5, distance=int(min_distance / 0.5))
        for peak in section_peaks:
            # Only add if not already covered by drop/breakdown
            t = times[peak]
            if not any(abs(c['time'] - t) < min_distance for c in candidates):
                candidates.append({
                    'time': float(t),
                    'type': 'section',
                    'score': float(feature_dist[peak]) * 0.5,
                    'name': f'Section'
                })

        # 5. Find outro (where energy stays low until end)
        for i in range(len(times) - 1, len(times) // 2, -1):
            remaining_energy = np.mean(energy_interp[i:])
            if remaining_energy < 0.2 and energy_interp[i-1] > 0.3:
                candidates.append({
                    'time': float(times[i]),
                    'type': 'outro',
                    'score': 0.8,
                    'name': 'Outro'
                })
                break

        # Sort by score and filter
        candidates.sort(key=lambda x: x['score'], reverse=True)

        # Remove duplicates within min_distance
        filtered = []
        for c in candidates:
            if not any(abs(f['time'] - c['time']) < min_distance for f in filtered):
                filtered.append(c)
            if len(filtered) >= max_cues:
                break

        # Sort by time and assign numbers
        filtered.sort(key=lambda x: x['time'])
        cue_points = []
        for i, c in enumerate(filtered):
            cue_points.append(CuePoint(
                time=c['time'],
                type=c['type'],
                confidence=min(1.0, c['score']),
                name=f"{c['name']}"
            ))

        return cue_points


def main():
    parser = argparse.ArgumentParser(description='Detect cue points in audio files.')
    parser.add_argument('audio_file', help='Path to the audio file to analyze.')
    parser.add_argument('--json', action='store_true', help='Output as JSON.')
    parser.add_argument('--max-cues', type=int, default=8, help='Maximum cue points.')
    parser.add_argument('--min-distance', type=float, default=8.0, help='Minimum seconds between cues.')

    args = parser.parse_args()

    if not os.path.exists(args.audio_file):
        print(f'Error: File not found: {args.audio_file}', file=sys.stderr)
        sys.exit(1)

    detector = CueDetector()
    cue_points = detector.detect_cue_points(
        args.audio_file,
        max_cues=args.max_cues,
        min_distance=args.min_distance
    )

    if args.json:
        output = {
            'cue_points': [
                {
                    'time': cp.time,
                    'type': cp.type,
                    'confidence': cp.confidence,
                    'name': cp.name
                }
                for cp in cue_points
            ]
        }
        print(json.dumps(output, indent=2))
    else:
        print(f'Detected {len(cue_points)} cue points:\n')
        for i, cp in enumerate(cue_points, 1):
            minutes = int(cp.time // 60)
            seconds = cp.time % 60
            print(f'  {i}. [{cp.type:10}] {minutes}:{seconds:05.2f}  {cp.name} (conf: {cp.confidence:.2f})')


if __name__ == '__main__':
    main()
