// Beat grid tracking utilities

export class BeatGrid {
  constructor(beats, bpm) {
    this.beats = beats || [];
    this.bpm = bpm || 0;
  }

  // Get the current beat index at a given time
  getBeatIndex(time) {
    if (!this.beats.length) return -1;

    for (let i = 0; i < this.beats.length; i++) {
      if (this.beats[i] > time) {
        return i - 1;
      }
    }
    return this.beats.length - 1;
  }

  // Get beat position within bar (0-3)
  getBeatInBar(time) {
    const idx = this.getBeatIndex(time);
    if (idx < 0) return 0;
    return idx % 4;
  }

  // Get current bar number (1-based)
  getBar(time) {
    const idx = this.getBeatIndex(time);
    if (idx < 0) return 1;
    return Math.floor(idx / 4) + 1;
  }

  // Check if we're on a beat (within tolerance)
  isOnBeat(time, tolerance = 0.05) {
    const idx = this.getBeatIndex(time);
    if (idx < 0 && this.beats.length > 0) {
      return Math.abs(this.beats[0] - time) < tolerance;
    }
    if (idx >= 0 && idx < this.beats.length) {
      return Math.abs(this.beats[idx] - time) < tolerance;
    }
    return false;
  }

  // Get time until next beat
  timeToNextBeat(time) {
    const idx = this.getBeatIndex(time);
    const nextIdx = idx + 1;
    if (nextIdx < this.beats.length) {
      return this.beats[nextIdx] - time;
    }
    return Infinity;
  }

  // Get fraction through current beat (0-1)
  getBeatPhase(time) {
    const idx = this.getBeatIndex(time);
    if (idx < 0) {
      if (this.beats.length > 0 && time < this.beats[0]) {
        return 0;
      }
      return 0;
    }
    const nextIdx = idx + 1;
    if (nextIdx >= this.beats.length) return 1;

    const beatStart = this.beats[idx];
    const beatEnd = this.beats[nextIdx];
    const beatDuration = beatEnd - beatStart;

    if (beatDuration <= 0) return 0;
    return (time - beatStart) / beatDuration;
  }
}
