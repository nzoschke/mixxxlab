import { LitElement, html, css } from 'https://cdn.jsdelivr.net/gh/lit/dist@3/core/lit-core.min.js';

class MixxWaveform extends LitElement {
  static properties = {
    beats: { type: Array },
    cuePoints: { type: Array },
    phrases: { type: Array },
    duration: { type: Number },
    currentTime: { type: Number },
    waveform: { type: Object },
    zoom: { type: Number },
  };

  static styles = css`
    :host {
      display: block;
      height: 100%;
      background: var(--waveform-bg);
      cursor: pointer;
    }

    canvas {
      width: 100%;
      height: 100%;
    }
  `;

  // Audio playback latency compensation (seconds)
  // Set to 0 - all compensation is done in backend waveform generation
  static AUDIO_LATENCY = 0;

  constructor() {
    super();
    this.beats = [];
    this.cuePoints = [];
    this.phrases = [];
    this.duration = 0;
    this.currentTime = 0;
    this.waveform = null;
    this.canvas = null;
    this.ctx = null;
    this.zoom = 1; // 1 = full track, higher = more zoomed in
    this.minZoom = 1;
    this.maxZoom = 32;
  }

  // Get latency-compensated time for display
  get displayTime() {
    return this.currentTime + MixxWaveform.AUDIO_LATENCY;
  }

  // Cue point type colors
  static CUE_COLORS = {
    // Python ML cue types
    intro: '#00ff00',      // Green
    drop: '#ff0000',       // Red
    breakdown: '#0088ff',  // Blue
    buildup: '#ffaa00',    // Orange
    outro: '#ff00ff',      // Magenta
    // QM-DSP cue types
    downbeat: '#00ffcc',   // Cyan
    phrase: '#ff6600',     // Orange-red
    section: '#ffff00',    // Yellow
    energy: '#ff0066',     // Hot pink
  };

  // Phrase/section colors (for SongFormer music structure)
  static PHRASE_COLORS = {
    intro: '#4CAF50',        // Green
    verse: '#2196F3',        // Blue
    chorus: '#FF9800',       // Orange
    bridge: '#9C27B0',       // Purple
    instrumental: '#607D8B', // Blue Grey
    outro: '#795548',        // Brown
    silence: '#37474F',      // Dark Grey
    'pre-chorus': '#E91E63', // Pink
    buildup: '#E91E63',      // Pink
    breakdown: '#00BCD4',    // Cyan
  };

  connectedCallback() {
    super.connectedCallback();
    window.addEventListener('timeupdate', this.handleTimeUpdate.bind(this));
    window.addEventListener('resize', this.handleResize.bind(this));
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    window.removeEventListener('timeupdate', this.handleTimeUpdate.bind(this));
    window.removeEventListener('resize', this.handleResize.bind(this));
  }

  handleTimeUpdate(e) {
    if (e.detail?.time !== undefined) {
      this.currentTime = e.detail.time;
      this.draw();
    }
  }

  handleResize() {
    this.setupCanvas();
    this.draw();
  }

  firstUpdated() {
    this.canvas = this.renderRoot.querySelector('canvas');
    this.ctx = this.canvas.getContext('2d');
    this.setupCanvas();
    this.draw();
  }

  updated(changed) {
    if (changed.has('beats') || changed.has('cuePoints') || changed.has('phrases') || changed.has('duration') || changed.has('waveform') || changed.has('zoom')) {
      this.draw();
    }
  }

  // Calculate the visible time range based on zoom level, centered on playhead
  getViewport() {
    const visibleDuration = this.duration / this.zoom;
    let viewStart = this.displayTime - visibleDuration / 2;

    // Clamp to valid range
    viewStart = Math.max(0, Math.min(viewStart, this.duration - visibleDuration));

    return {
      start: viewStart,
      end: viewStart + visibleDuration,
      duration: visibleDuration
    };
  }

  // Convert time to x coordinate in current viewport
  timeToX(time, width) {
    const viewport = this.getViewport();
    return ((time - viewport.start) / viewport.duration) * width;
  }

  // Convert x coordinate to time in current viewport
  xToTime(x, width) {
    const viewport = this.getViewport();
    return viewport.start + (x / width) * viewport.duration;
  }

  setupCanvas() {
    if (!this.canvas) return;

    const rect = this.canvas.getBoundingClientRect();
    const dpr = window.devicePixelRatio || 1;
    this.canvas.width = rect.width * dpr;
    this.canvas.height = rect.height * dpr;
    this.ctx.scale(dpr, dpr);
  }

  draw() {
    if (!this.ctx || !this.canvas) return;

    const rect = this.canvas.getBoundingClientRect();
    const width = rect.width;
    const height = rect.height;
    const centerY = height / 2;

    // Clear
    this.ctx.fillStyle = getComputedStyle(document.documentElement)
      .getPropertyValue('--waveform-bg') || '#0a0a1a';
    this.ctx.fillRect(0, 0, width, height);

    if (!this.duration) return;

    const waveformColor = getComputedStyle(document.documentElement)
      .getPropertyValue('--waveform-peak') || '#3498db';
    const playedColor = getComputedStyle(document.documentElement)
      .getPropertyValue('--waveform-played') || '#e94560';

    const viewport = this.getViewport();

    // Draw phrase regions (background only - labels drawn later on top)
    if (this.phrases && this.phrases.length > 0) {
      this.phrases.forEach((phrase) => {
        const phraseEnd = phrase.time + (phrase.duration || 0);

        // Skip phrases outside viewport
        if (phraseEnd < viewport.start || phrase.time > viewport.end) return;

        const startX = this.timeToX(Math.max(phrase.time, viewport.start), width);
        const endX = this.timeToX(Math.min(phraseEnd, viewport.end), width);
        const phraseWidth = endX - startX;

        if (phraseWidth < 1) return;

        const color = MixxWaveform.PHRASE_COLORS[phrase.label] || '#666666';

        // Draw background region with transparency
        this.ctx.fillStyle = color;
        this.ctx.globalAlpha = 0.15;
        this.ctx.fillRect(startX, 0, phraseWidth, height);

        this.ctx.globalAlpha = 1;
      });
    }

    // Draw waveform if available
    if (this.waveform?.peaks && this.waveform?.troughs) {
      const { peaks, troughs, pixels_per_sec } = this.waveform;
      const totalSamples = peaks.length;
      const playheadSample = this.displayTime * pixels_per_sec;

      // Calculate which samples are visible in the viewport
      const startSample = Math.floor(viewport.start * pixels_per_sec);
      const endSample = Math.ceil(viewport.end * pixels_per_sec);

      for (let i = startSample; i < endSample && i < totalSamples; i++) {
        if (i < 0) continue;
        const sampleTime = i / pixels_per_sec;
        const x = this.timeToX(sampleTime, width);
        const isPast = i <= playheadSample;

        // Scale peaks and troughs to canvas height
        const peakY = peaks[i] * (height / 2) * 0.9;
        const troughY = troughs[i] * (height / 2) * 0.9;

        this.ctx.strokeStyle = isPast ? playedColor : waveformColor;
        // Adjust line width based on zoom for better visibility
        this.ctx.lineWidth = Math.max(1, (width / (endSample - startSample)) * 0.8);
        this.ctx.beginPath();
        this.ctx.moveTo(x, centerY - peakY);
        this.ctx.lineTo(x, centerY - troughY);
        this.ctx.stroke();
      }
    }

    // Draw beat markers on top of waveform
    // Adjust detail level based on zoom to reduce visual noise
    if (this.beats.length > 0) {
      // Calculate beats per pixel to determine detail level
      const beatsInView = this.beats.filter(b => b >= viewport.start && b <= viewport.end).length;
      const beatsPerPixel = beatsInView / width;

      // Determine which beats to show based on density
      // At low zoom: only every 4 bars (16 beats)
      // At medium zoom: every bar (4 beats)
      // At high zoom: every beat
      // Bar numbers always shown on downbeats
      let beatInterval = 1;

      if (beatsPerPixel > 0.3) {
        // Very zoomed out: show every 4 bars
        beatInterval = 16;
      } else if (beatsPerPixel > 0.1) {
        // Zoomed out: show every bar (downbeats only)
        beatInterval = 4;
      } else if (beatsPerPixel > 0.05) {
        // Medium zoom: show downbeats
        beatInterval = 4;
      } else if (beatsPerPixel > 0.02) {
        // More zoomed: show every 2 beats
        beatInterval = 2;
      }
      // else: show all beats

      this.beats.forEach((beat, i) => {
        // Only draw beats within the viewport
        if (beat < viewport.start || beat > viewport.end) return;

        const x = this.timeToX(beat, width);
        const isPast = beat <= this.displayTime;
        const isDownbeat = i % 4 === 0;
        const barNumber = Math.floor(i / 4) + 1;

        // Skip beats based on interval
        if (beatInterval > 1) {
          if (beatInterval === 16 && i % 16 !== 0) return;
          if (beatInterval === 4 && i % 4 !== 0) return;
          if (beatInterval === 2 && i % 2 !== 0) return;
        }

        if (isDownbeat) {
          // Downbeats: bright white/yellow, thick line
          this.ctx.strokeStyle = isPast ? '#ffcc00' : '#ffffff';
          this.ctx.lineWidth = 2;
          this.ctx.globalAlpha = 0.85;
          this.ctx.beginPath();
          this.ctx.moveTo(x, 0);
          this.ctx.lineTo(x, height);
          this.ctx.stroke();

          // Always draw bar number with background on downbeats
          const barNum = barNumber.toString();
          this.ctx.font = 'bold 10px sans-serif';
          const textWidth = this.ctx.measureText(barNum).width;

          // Background pill
          this.ctx.fillStyle = isPast ? 'rgba(255, 204, 0, 0.9)' : 'rgba(255, 255, 255, 0.9)';
          this.ctx.globalAlpha = 1;
          this.ctx.beginPath();
          this.ctx.roundRect(x + 2, 2, textWidth + 6, 13, 3);
          this.ctx.fill();

          // Text
          this.ctx.fillStyle = '#000';
          this.ctx.fillText(barNum, x + 5, 12);
        } else {
          // Regular beats: subtle lines (only shown when zoomed in enough)
          this.ctx.strokeStyle = isPast ? 'rgba(255, 255, 255, 0.4)' : 'rgba(255, 255, 255, 0.25)';
          this.ctx.lineWidth = 1;
          this.ctx.globalAlpha = 1;
          this.ctx.beginPath();
          this.ctx.moveTo(x, 0);
          this.ctx.lineTo(x, height);
          this.ctx.stroke();
        }
      });

      this.ctx.globalAlpha = 1;
    }

    // Draw cue points (on top of beat markers)
    if (this.cuePoints && this.cuePoints.length > 0) {
      this.cuePoints.forEach((cue, i) => {
        // Only draw cues within the viewport
        if (cue.time < viewport.start || cue.time > viewport.end) return;

        const x = this.timeToX(cue.time, width);
        const color = MixxWaveform.CUE_COLORS[cue.type] || '#ffffff';

        // Draw cue marker line
        this.ctx.strokeStyle = color;
        this.ctx.lineWidth = 2;
        this.ctx.globalAlpha = 0.9;
        this.ctx.beginPath();
        this.ctx.moveTo(x, 0);
        this.ctx.lineTo(x, height - 14);
        this.ctx.stroke();

        // Draw cue marker triangle at bottom (pointing up)
        this.ctx.fillStyle = color;
        this.ctx.globalAlpha = 1;
        this.ctx.beginPath();
        this.ctx.moveTo(x, height - 14);
        this.ctx.lineTo(x - 5, height - 4);
        this.ctx.lineTo(x + 5, height - 4);
        this.ctx.closePath();
        this.ctx.fill();

        // Draw label at bottom (centered on marker)
        const label = cue.name || cue.type;
        this.ctx.font = 'bold 9px sans-serif';
        const textWidth = this.ctx.measureText(label).width;

        // Background pill for label (centered)
        this.ctx.fillStyle = color;
        this.ctx.globalAlpha = 0.95;
        this.ctx.beginPath();
        this.ctx.roundRect(x - textWidth/2 - 3, height - 4, textWidth + 6, 12, 2);
        this.ctx.fill();

        // Label text (centered)
        this.ctx.fillStyle = '#000';
        this.ctx.globalAlpha = 1;
        this.ctx.textAlign = 'center';
        this.ctx.fillText(label, x, height + 5);
        this.ctx.textAlign = 'left';
      });
      this.ctx.globalAlpha = 1;
    }

    // Draw phrase markers (on top of everything)
    if (this.phrases && this.phrases.length > 0) {
      this.phrases.forEach((phrase) => {
        // Skip phrases outside viewport
        if (phrase.time > viewport.end || phrase.time < viewport.start) return;

        const x = this.timeToX(phrase.time, width);
        const color = MixxWaveform.PHRASE_COLORS[phrase.label] || '#666666';

        // Draw phrase marker line
        this.ctx.strokeStyle = color;
        this.ctx.lineWidth = 3;
        this.ctx.globalAlpha = 0.9;
        this.ctx.beginPath();
        this.ctx.moveTo(x, 0);
        this.ctx.lineTo(x, height - 14);
        this.ctx.stroke();

        // Draw phrase marker triangle at bottom (pointing up)
        this.ctx.fillStyle = color;
        this.ctx.globalAlpha = 1;
        this.ctx.beginPath();
        this.ctx.moveTo(x, height - 14);
        this.ctx.lineTo(x - 5, height - 4);
        this.ctx.lineTo(x + 5, height - 4);
        this.ctx.closePath();
        this.ctx.fill();

        // Draw label at bottom (centered on marker)
        const label = phrase.label;
        this.ctx.font = 'bold 9px sans-serif';
        const textWidth = this.ctx.measureText(label).width;

        // Background pill for label (centered)
        this.ctx.fillStyle = color;
        this.ctx.globalAlpha = 0.95;
        this.ctx.beginPath();
        this.ctx.roundRect(x - textWidth/2 - 3, height - 4, textWidth + 6, 12, 2);
        this.ctx.fill();

        // Label text (centered)
        this.ctx.fillStyle = '#fff';
        this.ctx.globalAlpha = 1;
        this.ctx.textAlign = 'center';
        this.ctx.fillText(label, x, height + 5);
        this.ctx.textAlign = 'left';
      });
      this.ctx.globalAlpha = 1;
    }

    // Draw playhead
    const playheadX = this.timeToX(this.displayTime, width);
    this.ctx.strokeStyle = '#fff';
    this.ctx.lineWidth = 2;
    this.ctx.beginPath();
    this.ctx.moveTo(playheadX, 0);
    this.ctx.lineTo(playheadX, height);
    this.ctx.stroke();

    // Draw zoom indicator when zoomed in
    if (this.zoom > 1) {
      this.ctx.fillStyle = 'rgba(255, 255, 255, 0.7)';
      this.ctx.font = '11px sans-serif';
      this.ctx.fillText(`${this.zoom.toFixed(1)}x`, width - 35, 14);
    }
  }

  handleClick(e) {
    if (!this.duration) return;

    const rect = this.canvas.getBoundingClientRect();
    const x = e.clientX - rect.left;

    // Convert click position to time
    // Don't apply latency compensation here - seek to exact visual position
    const viewport = this.getViewport();
    const clickTime = viewport.start + (x / rect.width) * viewport.duration;

    // Clamp to valid range
    const clampedTime = Math.max(0, Math.min(clickTime, this.duration));

    this.dispatchEvent(new CustomEvent('seek', {
      detail: { time: clampedTime },
      bubbles: true,
      composed: true
    }));
  }

  handleWheel(e) {
    if (!this.duration) return;

    e.preventDefault();

    // Zoom in/out based on scroll direction
    const zoomFactor = 1.2;
    if (e.deltaY < 0) {
      // Scroll up = zoom in
      this.zoom = Math.min(this.maxZoom, this.zoom * zoomFactor);
    } else {
      // Scroll down = zoom out
      this.zoom = Math.max(this.minZoom, this.zoom / zoomFactor);
    }

    // Notify parent of zoom change
    this.dispatchEvent(new CustomEvent('zoomchange', {
      detail: { zoom: this.zoom },
      bubbles: true,
      composed: true
    }));

    this.draw();
  }

  render() {
    return html`<canvas @click=${this.handleClick} @wheel=${this.handleWheel}></canvas>`;
  }
}

customElements.define('mixx-waveform', MixxWaveform);

// Overview waveform - always shows full track with viewport indicator
class MixxWaveformOverview extends LitElement {
  static properties = {
    beats: { type: Array },
    cuePoints: { type: Array },
    phrases: { type: Array },
    duration: { type: Number },
    currentTime: { type: Number },
    waveform: { type: Object },
    zoom: { type: Number },
  };

  static styles = css`
    :host {
      display: block;
      height: 100%;
      background: var(--waveform-bg);
      cursor: pointer;
    }

    canvas {
      width: 100%;
      height: 100%;
    }
  `;

  constructor() {
    super();
    this.beats = [];
    this.cuePoints = [];
    this.phrases = [];
    this.duration = 0;
    this.currentTime = 0;
    this.waveform = null;
    this.zoom = 1;
    this.canvas = null;
    this.ctx = null;
  }

  // Get latency-compensated time for display (use same value as main waveform)
  get displayTime() {
    return this.currentTime + MixxWaveform.AUDIO_LATENCY;
  }

  connectedCallback() {
    super.connectedCallback();
    window.addEventListener('timeupdate', this.handleTimeUpdate.bind(this));
    window.addEventListener('resize', this.handleResize.bind(this));
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    window.removeEventListener('timeupdate', this.handleTimeUpdate.bind(this));
    window.removeEventListener('resize', this.handleResize.bind(this));
  }

  handleTimeUpdate(e) {
    if (e.detail?.time !== undefined) {
      this.currentTime = e.detail.time;
      this.draw();
    }
  }

  handleResize() {
    this.setupCanvas();
    this.draw();
  }

  firstUpdated() {
    this.canvas = this.renderRoot.querySelector('canvas');
    this.ctx = this.canvas.getContext('2d');
    this.setupCanvas();
    this.draw();
  }

  updated(changed) {
    if (changed.has('beats') || changed.has('cuePoints') || changed.has('phrases') || changed.has('duration') || changed.has('waveform') || changed.has('zoom')) {
      this.draw();
    }
  }

  setupCanvas() {
    if (!this.canvas) return;

    const rect = this.canvas.getBoundingClientRect();
    const dpr = window.devicePixelRatio || 1;
    this.canvas.width = rect.width * dpr;
    this.canvas.height = rect.height * dpr;
    this.ctx.scale(dpr, dpr);
  }

  // Calculate viewport (same logic as main waveform)
  getViewport() {
    const visibleDuration = this.duration / this.zoom;
    let viewStart = this.displayTime - visibleDuration / 2;
    viewStart = Math.max(0, Math.min(viewStart, this.duration - visibleDuration));

    return {
      start: viewStart,
      end: viewStart + visibleDuration,
      duration: visibleDuration
    };
  }

  draw() {
    if (!this.ctx || !this.canvas) return;

    const rect = this.canvas.getBoundingClientRect();
    const width = rect.width;
    const height = rect.height;
    const centerY = height / 2;

    // Clear
    this.ctx.fillStyle = getComputedStyle(document.documentElement)
      .getPropertyValue('--waveform-bg') || '#0a0a1a';
    this.ctx.fillRect(0, 0, width, height);

    if (!this.duration) return;

    const waveformColor = getComputedStyle(document.documentElement)
      .getPropertyValue('--waveform-peak') || '#3498db';
    const playedColor = getComputedStyle(document.documentElement)
      .getPropertyValue('--waveform-played') || '#e94560';

    // Draw waveform (always full track)
    if (this.waveform?.peaks && this.waveform?.troughs) {
      const { peaks, troughs, pixels_per_sec } = this.waveform;
      const totalSamples = peaks.length;
      const playheadSample = this.displayTime * pixels_per_sec;

      for (let i = 0; i < totalSamples; i++) {
        const x = (i / totalSamples) * width;
        const isPast = i <= playheadSample;

        const peakY = peaks[i] * (height / 2) * 0.85;
        const troughY = troughs[i] * (height / 2) * 0.85;

        this.ctx.strokeStyle = isPast ? playedColor : waveformColor;
        this.ctx.lineWidth = Math.max(1, width / totalSamples);
        this.ctx.globalAlpha = 0.6;
        this.ctx.beginPath();
        this.ctx.moveTo(x, centerY - peakY);
        this.ctx.lineTo(x, centerY - troughY);
        this.ctx.stroke();
      }
      this.ctx.globalAlpha = 1;
    }

    // Draw cue point markers (triangles pointing up at bottom with lines)
    if (this.cuePoints && this.cuePoints.length > 0) {
      this.cuePoints.forEach((cue) => {
        const x = (cue.time / this.duration) * width;
        const color = MixxWaveform.CUE_COLORS[cue.type] || '#ffffff';

        // Draw vertical line
        this.ctx.strokeStyle = color;
        this.ctx.lineWidth = 1.5;
        this.ctx.globalAlpha = 0.7;
        this.ctx.beginPath();
        this.ctx.moveTo(x, 0);
        this.ctx.lineTo(x, height - 6);
        this.ctx.stroke();

        // Draw small triangle marker at bottom (pointing up)
        this.ctx.fillStyle = color;
        this.ctx.globalAlpha = 1;
        this.ctx.beginPath();
        this.ctx.moveTo(x, height - 6);
        this.ctx.lineTo(x - 4, height);
        this.ctx.lineTo(x + 4, height);
        this.ctx.closePath();
        this.ctx.fill();
      });
      this.ctx.globalAlpha = 1;
    }

    // Draw phrase markers (triangles pointing up at bottom with lines)
    if (this.phrases && this.phrases.length > 0) {
      this.phrases.forEach((phrase) => {
        const x = (phrase.time / this.duration) * width;
        const color = MixxWaveform.PHRASE_COLORS[phrase.label] || '#666666';

        // Draw vertical line
        this.ctx.strokeStyle = color;
        this.ctx.lineWidth = 2;
        this.ctx.globalAlpha = 0.8;
        this.ctx.beginPath();
        this.ctx.moveTo(x, 0);
        this.ctx.lineTo(x, height - 6);
        this.ctx.stroke();

        // Draw small triangle marker at bottom (pointing up)
        this.ctx.fillStyle = color;
        this.ctx.globalAlpha = 1;
        this.ctx.beginPath();
        this.ctx.moveTo(x, height - 6);
        this.ctx.lineTo(x - 4, height);
        this.ctx.lineTo(x + 4, height);
        this.ctx.closePath();
        this.ctx.fill();
      });
      this.ctx.globalAlpha = 1;
    }

    // Draw viewport indicator when zoomed
    if (this.zoom > 1) {
      const viewport = this.getViewport();
      const vpStartX = (viewport.start / this.duration) * width;
      const vpEndX = (viewport.end / this.duration) * width;
      const vpWidth = vpEndX - vpStartX;

      // Dim areas outside viewport
      this.ctx.fillStyle = 'rgba(0, 0, 0, 0.5)';
      this.ctx.fillRect(0, 0, vpStartX, height);
      this.ctx.fillRect(vpEndX, 0, width - vpEndX, height);

      // Draw viewport border
      this.ctx.strokeStyle = 'rgba(255, 255, 255, 0.8)';
      this.ctx.lineWidth = 2;
      this.ctx.strokeRect(vpStartX, 1, vpWidth, height - 2);
    }

    // Draw playhead
    const playheadX = (this.displayTime / this.duration) * width;
    this.ctx.strokeStyle = '#fff';
    this.ctx.lineWidth = 2;
    this.ctx.beginPath();
    this.ctx.moveTo(playheadX, 0);
    this.ctx.lineTo(playheadX, height);
    this.ctx.stroke();
  }

  handleClick(e) {
    if (!this.duration) return;

    const rect = this.canvas.getBoundingClientRect();
    const x = e.clientX - rect.left;
    // Subtract latency when seeking so audio lands where user clicked
    const seekTime = (x / rect.width) * this.duration - MixxWaveform.AUDIO_LATENCY;
    const clampedTime = Math.max(0, Math.min(seekTime, this.duration));

    this.dispatchEvent(new CustomEvent('seek', {
      detail: { time: clampedTime },
      bubbles: true,
      composed: true
    }));
  }

  render() {
    return html`<canvas @click=${this.handleClick}></canvas>`;
  }
}

customElements.define('mixx-waveform-overview', MixxWaveformOverview);
