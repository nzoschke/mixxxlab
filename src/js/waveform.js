import { LitElement, html, css } from 'https://cdn.jsdelivr.net/gh/lit/dist@3/core/lit-core.min.js';

class MixxWaveform extends LitElement {
  static properties = {
    beats: { type: Array },
    duration: { type: Number },
    currentTime: { type: Number },
    waveform: { type: Object },
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
    this.duration = 0;
    this.currentTime = 0;
    this.waveform = null;
    this.canvas = null;
    this.ctx = null;
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
    if (changed.has('beats') || changed.has('duration') || changed.has('waveform')) {
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

    // Draw waveform if available
    if (this.waveform?.peaks && this.waveform?.troughs) {
      const { peaks, troughs, pixels_per_sec } = this.waveform;
      const totalSamples = peaks.length;
      const playheadSample = this.currentTime * pixels_per_sec;

      for (let i = 0; i < totalSamples; i++) {
        const x = (i / totalSamples) * width;
        const isPast = i <= playheadSample;

        // Scale peaks and troughs to canvas height
        const peakY = peaks[i] * (height / 2) * 0.9;
        const troughY = troughs[i] * (height / 2) * 0.9;

        this.ctx.strokeStyle = isPast ? playedColor : waveformColor;
        this.ctx.lineWidth = Math.max(1, width / totalSamples);
        this.ctx.beginPath();
        this.ctx.moveTo(x, centerY - peakY);
        this.ctx.lineTo(x, centerY - troughY);
        this.ctx.stroke();
      }
    }

    // Draw beat markers on top of waveform
    if (this.beats.length > 0) {
      this.ctx.globalAlpha = 0.6;

      this.beats.forEach((beat, i) => {
        const x = (beat / this.duration) * width;
        const isPast = beat <= this.currentTime;
        const isDownbeat = i % 4 === 0;

        this.ctx.strokeStyle = isPast ? playedColor : waveformColor;
        this.ctx.lineWidth = isDownbeat ? 2 : 1;
        this.ctx.globalAlpha = isDownbeat ? 0.8 : 0.4;

        this.ctx.beginPath();
        this.ctx.moveTo(x, 0);
        this.ctx.lineTo(x, height);
        this.ctx.stroke();

        // Draw downbeat bar numbers
        if (isDownbeat) {
          this.ctx.fillStyle = isPast ? playedColor : waveformColor;
          this.ctx.font = '10px sans-serif';
          this.ctx.globalAlpha = 0.9;
          this.ctx.fillText(Math.floor(i / 4 + 1).toString(), x + 3, 12);
        }
      });

      this.ctx.globalAlpha = 1;
    }

    // Draw playhead
    const playheadX = (this.currentTime / this.duration) * width;
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
    const seekTime = (x / rect.width) * this.duration;

    this.dispatchEvent(new CustomEvent('seek', {
      detail: { time: seekTime },
      bubbles: true,
      composed: true
    }));
  }

  render() {
    return html`<canvas @click=${this.handleClick}></canvas>`;
  }
}

customElements.define('mixx-waveform', MixxWaveform);
