#!/usr/bin/env -S uv run
# /// script
# requires-python = ">=3.10"
# dependencies = [
#     "numpy",
#     "scipy",
#     "soundfile",
#     "torch",
#     "transformers",
# ]
# ///
"""
Music structure analysis using SongFormer.

Detects musical phrases/sections: intro, verse, chorus, bridge, instrumental, outro, etc.
Uses the ASLP-lab/SongFormer model from HuggingFace.

Reference: https://github.com/ASLP-lab/SongFormer
"""

from __future__ import annotations

import argparse
import json
import os
import sys
from pathlib import Path

import numpy as np
import soundfile as sf
import torch


# Target sample rate for SongFormer
TARGET_SR = 16000


def load_audio(audio_path: str | Path) -> tuple[np.ndarray, float]:
    """Load and preprocess audio to mono at target sample rate."""
    from scipy import signal

    audio, sr = sf.read(audio_path, dtype='float32')

    # Convert to mono
    if audio.ndim > 1:
        audio = np.mean(audio, axis=1)

    # Resample if needed
    if sr != TARGET_SR:
        ratio = TARGET_SR / sr
        new_length = int(len(audio) * ratio)
        audio = signal.resample(audio, new_length).astype(np.float32)

    duration = len(audio) / TARGET_SR
    return audio, duration


def analyze_structure(audio_path: str | Path) -> list[dict]:
    """
    Analyze music structure using SongFormer.

    Returns list of phrases with time and label.
    """
    from transformers import AutoModel, AutoProcessor

    # Load model from HuggingFace
    model_name = "ASLP-lab/SongFormer"

    try:
        processor = AutoProcessor.from_pretrained(model_name, trust_remote_code=True)
        model = AutoModel.from_pretrained(model_name, trust_remote_code=True)
    except Exception as e:
        # Fall back to local inference if HF model not available
        raise RuntimeError(f"Could not load SongFormer model: {e}")

    # Load audio
    audio, duration = load_audio(audio_path)

    # Process with model
    device = "cuda" if torch.cuda.is_available() else "cpu"
    model = model.to(device)
    model.eval()

    # Prepare input
    inputs = processor(audio, sampling_rate=TARGET_SR, return_tensors="pt")
    inputs = {k: v.to(device) for k, v in inputs.items()}

    # Run inference
    with torch.no_grad():
        outputs = model(**inputs)

    # Parse predictions into phrases
    phrases = []
    predictions = outputs.predictions  # List of (timestamp, label) tuples

    for i, (timestamp, label) in enumerate(predictions):
        phrase = {
            "time": float(timestamp),
            "label": label.lower(),
        }

        # Calculate duration to next phrase
        if i < len(predictions) - 1:
            phrase["duration"] = float(predictions[i + 1][0]) - float(timestamp)
        else:
            phrase["duration"] = duration - float(timestamp)

        phrases.append(phrase)

    return phrases


def analyze_structure_fallback(audio_path: str | Path) -> list[dict]:
    """
    Fallback structure analysis using simple energy-based segmentation.
    Used when SongFormer model is not available.
    """
    from scipy import signal as scipy_signal
    from scipy.signal import find_peaks

    audio, duration = load_audio(audio_path)

    # Compute energy in windows
    window_sec = 2.0
    hop_sec = 0.5
    window_samples = int(window_sec * TARGET_SR)
    hop_samples = int(hop_sec * TARGET_SR)

    times = []
    energies = []

    for i in range(0, len(audio) - window_samples, hop_samples):
        window = audio[i:i + window_samples]
        rms = np.sqrt(np.mean(window ** 2))
        times.append(i / TARGET_SR)
        energies.append(rms)

    times = np.array(times)
    energies = np.array(energies)

    # Normalize energy
    energies = energies / (np.max(energies) + 1e-8)

    # Compute energy derivative
    energy_diff = np.gradient(energies)

    # Find significant changes
    phrases = []

    # Intro: first moment of significant energy
    for i, (t, e) in enumerate(zip(times, energies)):
        if e > 0.1:
            phrases.append({"time": float(t), "label": "intro"})
            break

    # Find energy drops and rises for section boundaries
    min_distance = int(8.0 / hop_sec)  # 8 seconds minimum between sections

    # Rising edges (potential drops/chorus starts)
    rise_score = np.maximum(0, energy_diff)
    rise_peaks, _ = find_peaks(rise_score, height=0.05, distance=min_distance)

    # Falling edges (potential breakdown starts)
    fall_score = np.maximum(0, -energy_diff)
    fall_peaks, _ = find_peaks(fall_score, height=0.05, distance=min_distance)

    # Combine and sort
    section_times = []

    for peak in rise_peaks:
        if energies[peak] > 0.5:  # High energy = chorus/drop
            section_times.append((times[peak], "chorus"))
        else:
            section_times.append((times[peak], "verse"))

    for peak in fall_peaks:
        if energies[peak] < 0.3:  # Low energy = breakdown
            section_times.append((times[peak], "bridge"))

    # Sort by time and filter duplicates
    section_times.sort(key=lambda x: x[0])

    for t, label in section_times:
        # Don't add if too close to existing phrase
        if not phrases or t - phrases[-1]["time"] >= 8.0:
            phrases.append({"time": float(t), "label": label})

    # Outro: where energy stays low until end
    for i in range(len(times) - 1, len(times) // 2, -1):
        remaining_energy = np.mean(energies[i:])
        if remaining_energy < 0.2 and i > 0 and energies[i-1] > 0.3:
            if not phrases or times[i] - phrases[-1]["time"] >= 8.0:
                phrases.append({"time": float(times[i]), "label": "outro"})
            break

    # Sort by time
    phrases.sort(key=lambda x: x["time"])

    # Calculate durations
    for i in range(len(phrases)):
        if i < len(phrases) - 1:
            phrases[i]["duration"] = phrases[i + 1]["time"] - phrases[i]["time"]
        else:
            phrases[i]["duration"] = duration - phrases[i]["time"]

    return phrases


def main():
    parser = argparse.ArgumentParser(description='Analyze music structure using SongFormer.')
    parser.add_argument('audio_file', help='Path to the audio file to analyze.')
    parser.add_argument('--json', action='store_true', help='Output as JSON.')
    parser.add_argument('--fallback', action='store_true', help='Use fallback analyzer (no ML model).')

    args = parser.parse_args()

    if not os.path.exists(args.audio_file):
        print(f'Error: File not found: {args.audio_file}', file=sys.stderr)
        sys.exit(1)

    try:
        if args.fallback:
            phrases = analyze_structure_fallback(args.audio_file)
        else:
            try:
                phrases = analyze_structure(args.audio_file)
            except Exception as e:
                print(f"Warning: SongFormer model failed ({e}), using fallback", file=sys.stderr)
                phrases = analyze_structure_fallback(args.audio_file)
    except Exception as e:
        print(f'Error analyzing file: {e}', file=sys.stderr)
        sys.exit(1)

    if args.json:
        output = {"phrases": phrases}
        print(json.dumps(output, indent=2))
    else:
        print(f'Detected {len(phrases)} phrases:\n')
        for i, phrase in enumerate(phrases, 1):
            minutes = int(phrase["time"] // 60)
            seconds = phrase["time"] % 60
            dur = phrase.get("duration", 0)
            print(f'  {i}. [{phrase["label"]:12}] {minutes}:{seconds:05.2f}  ({dur:.1f}s)')


if __name__ == '__main__':
    main()
