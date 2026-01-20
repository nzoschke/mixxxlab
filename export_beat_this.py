#!/usr/bin/env -S uv run
# /// script
# requires-python = ">=3.9"
# dependencies = [
#     "torch>=2.0",
#     "torchaudio>=2.0",
#     "onnx>=1.14",
#     "onnxscript",
#     "beat_this @ git+https://github.com/CPJKU/beat_this.git",
# ]
# ///
"""
Export beat_this models to ONNX format for Go inference.

This script exports two ONNX models:
1. mel.onnx - Mel spectrogram preprocessing
2. model_small.onnx or model_full.onnx - Main beat tracker

Usage:
    uv run export_beat_this.py              # Export small model (default)
    uv run export_beat_this.py --full       # Export full model
    uv run export_beat_this.py --both       # Export both models
"""

from __future__ import annotations

import argparse
import os
from pathlib import Path

import torch
import torch.nn as nn
import torchaudio


class LogMelSpectrogram(nn.Module):
    """Mel spectrogram extraction matching beat_this preprocessing."""

    def __init__(
        self,
        sample_rate: int = 22050,
        n_fft: int = 1024,
        hop_length: int = 441,
        n_mels: int = 128,
        f_min: float = 30.0,
        f_max: float = 11000.0,
    ):
        super().__init__()
        self.sample_rate = sample_rate
        self.hop_length = hop_length

        # Create mel spectrogram transform
        self.mel_spec = torchaudio.transforms.MelSpectrogram(
            sample_rate=sample_rate,
            n_fft=n_fft,
            hop_length=hop_length,
            n_mels=n_mels,
            f_min=f_min,
            f_max=f_max,
            power=1.0,  # Magnitude spectrogram
            norm="slaney",
            mel_scale="slaney",
        )

    def forward(self, audio: torch.Tensor) -> torch.Tensor:
        """
        Convert audio to log mel spectrogram.

        Args:
            audio: (batch, samples) or (samples,) audio at 22050 Hz

        Returns:
            (batch, time, n_mels) log mel spectrogram
        """
        if audio.dim() == 1:
            audio = audio.unsqueeze(0)

        # Compute mel spectrogram: (batch, n_mels, time)
        mel = self.mel_spec(audio)

        # Apply log transform: log1p(1000 * mel)
        mel = torch.log1p(1000 * mel)

        # Transpose to (batch, time, n_mels) for model input
        mel = mel.transpose(1, 2)

        return mel


def export_mel_spectrogram(output_dir: Path) -> Path:
    """Export mel spectrogram preprocessing to ONNX."""
    print("Exporting mel spectrogram model...")

    model = LogMelSpectrogram()
    model.eval()

    # Create example input (1 second of audio at 22050 Hz)
    example_audio = torch.randn(1, 22050)

    output_path = output_dir / "mel.onnx"

    # Export to ONNX using legacy TorchScript-based exporter
    with torch.no_grad():
        torch.onnx.export(
            model,
            example_audio,
            output_path,
            input_names=["audio"],
            output_names=["mel_spectrogram"],
            dynamic_axes={
                "audio": {0: "batch", 1: "samples"},
                "mel_spectrogram": {0: "batch", 1: "time"},
            },
            opset_version=17,
            do_constant_folding=True,
            dynamo=False,
        )

    print(f"  Saved: {output_path}")
    return output_path


def export_beat_model(output_dir: Path, model_size: str = "small") -> Path:
    """
    Export beat_this model to ONNX.

    Args:
        output_dir: Directory to save ONNX model
        model_size: "small" (~8MB) or "full" (~78MB)
    """
    print(f"Exporting beat_this model ({model_size})...")

    # Import beat_this and load pretrained model
    from beat_this.inference import File2Beats

    # Map user-friendly names to actual checkpoint names
    checkpoint_map = {
        "small": "small0",
        "full": "final0",
    }
    checkpoint_name = checkpoint_map.get(model_size, model_size)

    # Load the model
    device = "cpu"
    f2b = File2Beats(checkpoint_path=checkpoint_name, device=device)
    model = f2b.model
    model.eval()

    # Get the actual model architecture
    # beat_this uses a TCN-based architecture
    # Input shape: (batch, time, 128) mel spectrogram
    # Output shape: (batch, time, 2) - beat and downbeat logits

    # Create example input (150 frames ~ 3 seconds)
    example_mel = torch.randn(1, 150, 128)

    output_path = output_dir / f"model_{model_size}.onnx"

    # Export to ONNX using legacy TorchScript-based exporter
    # The new dynamo-based exporter has issues with beat_this's complex architecture
    with torch.no_grad():
        torch.onnx.export(
            model,
            example_mel,
            output_path,
            input_names=["mel_spectrogram"],
            output_names=["logits"],
            dynamic_axes={
                "mel_spectrogram": {0: "batch", 1: "time"},
                "logits": {0: "batch", 1: "time"},
            },
            opset_version=17,  # Use older opset for better compatibility
            do_constant_folding=True,
            dynamo=False,  # Use legacy TorchScript exporter
        )

    # Print model size
    size_mb = output_path.stat().st_size / (1024 * 1024)
    print(f"  Saved: {output_path} ({size_mb:.1f} MB)")

    return output_path


def verify_onnx_model(model_path: Path) -> bool:
    """Verify ONNX model loads correctly."""
    import onnx

    try:
        model = onnx.load(str(model_path))
        onnx.checker.check_model(model)
        print(f"  Verified: {model_path.name}")
        return True
    except Exception as e:
        print(f"  Error verifying {model_path.name}: {e}")
        return False


def main():
    parser = argparse.ArgumentParser(
        description="Export beat_this models to ONNX format."
    )
    parser.add_argument(
        "--output-dir",
        type=Path,
        default=Path("models/beat_this"),
        help="Output directory for ONNX models",
    )
    parser.add_argument(
        "--full",
        action="store_true",
        help="Export full model instead of small",
    )
    parser.add_argument(
        "--both",
        action="store_true",
        help="Export both small and full models",
    )
    parser.add_argument(
        "--skip-mel",
        action="store_true",
        help="Skip mel spectrogram export (if already exists)",
    )

    args = parser.parse_args()

    # Create output directory
    args.output_dir.mkdir(parents=True, exist_ok=True)
    print(f"Output directory: {args.output_dir}")

    # Export mel spectrogram model
    if not args.skip_mel:
        mel_path = export_mel_spectrogram(args.output_dir)
        verify_onnx_model(mel_path)

    # Export beat tracker model(s)
    if args.both:
        small_path = export_beat_model(args.output_dir, "small")
        verify_onnx_model(small_path)
        full_path = export_beat_model(args.output_dir, "full")
        verify_onnx_model(full_path)
    elif args.full:
        full_path = export_beat_model(args.output_dir, "full")
        verify_onnx_model(full_path)
    else:
        small_path = export_beat_model(args.output_dir, "small")
        verify_onnx_model(small_path)

    print("\nExport complete!")
    print("\nUsage:")
    print("  go run ./cmd/app analyze --analyzer=beat-this <audio_file>")


if __name__ == "__main__":
    main()
