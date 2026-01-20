// AudioEngine - Web Audio API playback using AudioBufferSourceNode
// Provides sample-accurate playback with proper handling of one-shot sources

export class AudioEngine extends EventTarget {
  constructor() {
    super();
    this.audioContext = null;
    this.audioBuffer = null;
    this.sourceNode = null;
    this.analyserNode = null;
    this.gainNode = null;

    // Playback state
    this._playing = false;
    this._startOffset = 0;      // Position in audio when playback started
    this._startContextTime = 0; // audioContext.currentTime when playback started
    this._duration = 0;

    // RAF for timeupdate
    this._rafId = null;
  }

  // Initialize audio context (must be called after user gesture)
  async _ensureContext() {
    if (!this.audioContext) {
      this.audioContext = new (window.AudioContext || window.webkitAudioContext)();

      // Create analyser node
      this.analyserNode = this.audioContext.createAnalyser();
      this.analyserNode.fftSize = 2048;
      this.analyserNode.smoothingTimeConstant = 0.3;

      // Create gain node for volume control
      this.gainNode = this.audioContext.createGain();

      // Connect: source -> analyser -> gain -> destination
      this.analyserNode.connect(this.gainNode);
      this.gainNode.connect(this.audioContext.destination);
    }

    // Resume if suspended (browser autoplay policy)
    if (this.audioContext.state === 'suspended') {
      await this.audioContext.resume();
    }
  }

  // Load audio from URL
  async load(url) {
    await this._ensureContext();

    // Stop any current playback
    this._stopSource();
    this._playing = false;
    this._startOffset = 0;

    this.dispatchEvent(new CustomEvent('loadstart'));

    try {
      const response = await fetch(url);
      if (!response.ok) {
        throw new Error(`HTTP ${response.status}`);
      }

      const arrayBuffer = await response.arrayBuffer();
      this.audioBuffer = await this.audioContext.decodeAudioData(arrayBuffer);
      this._duration = this.audioBuffer.duration;

      this.dispatchEvent(new CustomEvent('canplay'));
      this.dispatchEvent(new CustomEvent('durationchange', {
        detail: { duration: this._duration }
      }));
    } catch (e) {
      this.dispatchEvent(new CustomEvent('error', { detail: { error: e } }));
      throw e;
    }
  }

  // Start or resume playback
  play() {
    if (!this.audioBuffer || this._playing) return;

    this._ensureContext();
    this._createAndStartSource(this._startOffset);
    this._playing = true;

    this.dispatchEvent(new CustomEvent('play'));
    this._startTimeLoop();
  }

  // Pause playback (remember position)
  pause() {
    if (!this._playing) return;

    // Calculate current position before stopping
    this._startOffset = this.getCurrentTime();
    this._stopSource();
    this._playing = false;

    this.dispatchEvent(new CustomEvent('pause'));
    this._stopTimeLoop();
  }

  // Seek to specific time
  seek(time) {
    // Clamp to valid range
    time = Math.max(0, Math.min(time, this._duration));

    const wasPlaying = this._playing;

    // Stop current playback
    if (this._playing) {
      this._stopSource();
      this._stopTimeLoop();
    }

    // Update position
    this._startOffset = time;

    // Resume if was playing
    if (wasPlaying && this.audioBuffer) {
      this._createAndStartSource(time);
      this._startTimeLoop();
    }

    // Dispatch seek event
    this.dispatchEvent(new CustomEvent('seeked', { detail: { time } }));
  }

  // Get current playback position
  getCurrentTime() {
    if (!this._playing) {
      return this._startOffset;
    }

    const elapsed = this.audioContext.currentTime - this._startContextTime;
    const currentTime = this._startOffset + elapsed;

    // Clamp to duration
    return Math.min(currentTime, this._duration);
  }

  // Create a new source node and start it
  _createAndStartSource(offset) {
    // Stop any existing source
    this._stopSource();

    // Create new source
    this.sourceNode = this.audioContext.createBufferSource();
    this.sourceNode.buffer = this.audioBuffer;
    this.sourceNode.connect(this.analyserNode);

    // Handle natural end of playback
    this.sourceNode.onended = () => {
      // Only dispatch ended if we actually reached the end
      const currentTime = this.getCurrentTime();
      if (currentTime >= this._duration - 0.1) {
        this._playing = false;
        this._startOffset = 0;
        this._stopTimeLoop();
        this.dispatchEvent(new CustomEvent('ended'));
      }
    };

    // Record timing
    this._startContextTime = this.audioContext.currentTime;
    this._startOffset = offset;
    this._playing = true;

    // Start playback from offset
    this.sourceNode.start(0, offset);
  }

  // Stop the current source node
  _stopSource() {
    if (this.sourceNode) {
      this.sourceNode.onended = null; // Prevent ended event from manual stop
      try {
        this.sourceNode.stop();
      } catch (e) {
        // Ignore errors if already stopped
      }
      this.sourceNode.disconnect();
      this.sourceNode = null;
    }
  }

  // RAF loop for timeupdate events
  _startTimeLoop() {
    const update = () => {
      if (this._playing) {
        this._rafId = requestAnimationFrame(update);
      }
    };
    this._rafId = requestAnimationFrame(update);
  }

  _stopTimeLoop() {
    if (this._rafId) {
      cancelAnimationFrame(this._rafId);
      this._rafId = null;
    }
  }

  // Getters
  get analyser() {
    return this.analyserNode;
  }

  get duration() {
    return this._duration;
  }

  get playing() {
    return this._playing;
  }

  get paused() {
    return !this._playing;
  }

  // Cleanup
  dispose() {
    this._stopSource();
    this._stopTimeLoop();

    if (this.audioContext) {
      this.audioContext.close();
      this.audioContext = null;
    }

    this.audioBuffer = null;
    this.analyserNode = null;
    this.gainNode = null;
  }
}
