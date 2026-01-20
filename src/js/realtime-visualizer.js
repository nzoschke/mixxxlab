import { LitElement, html, css } from 'https://cdn.jsdelivr.net/gh/lit/dist@3/core/lit-core.min.js';

// Real-time audio visualizer using Web Audio API
class MixxRealtimeVisualizer extends LitElement {
  static properties = {
    audioEngine: { type: Object },
  };

  static styles = css`
    :host {
      display: block;
      height: 100%;
      background: var(--waveform-bg);
    }

    canvas {
      width: 100%;
      height: 100%;
    }

    .label {
      position: absolute;
      top: 4px;
      left: 8px;
      font-size: 10px;
      color: rgba(255, 255, 255, 0.5);
      pointer-events: none;
    }

    .container {
      position: relative;
      width: 100%;
      height: 100%;
    }
  `;

  constructor() {
    super();
    this.audioEngine = null;
    this.analyser = null;
    this.canvas = null;
    this.ctx = null;
    this.dataArray = null;
    this.rafId = null;
    this.connected = false;
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    this.stopVisualization();
  }

  firstUpdated() {
    this.canvas = this.renderRoot.querySelector('canvas');
    this.ctx = this.canvas.getContext('2d');
    this.setupCanvas();
    window.addEventListener('resize', () => this.setupCanvas());
  }

  updated(changed) {
    if (changed.has('audioEngine') && this.audioEngine) {
      this.connectToEngine();
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

  connectToEngine() {
    if (this.connected || !this.audioEngine) return;

    // Use the engine's analyser directly (already connected in engine)
    this.analyser = this.audioEngine.analyser;

    if (this.analyser) {
      // Create data array for time domain data
      this.dataArray = new Uint8Array(this.analyser.frequencyBinCount);
      this.connected = true;
      this.startVisualization();
    }
  }

  startVisualization() {
    const draw = () => {
      this.rafId = requestAnimationFrame(draw);

      if (!this.analyser || !this.ctx || !this.canvas) return;

      // Get time domain data (waveform)
      this.analyser.getByteTimeDomainData(this.dataArray);

      // Check if there's actual audio playing (not just silence)
      const hasSignal = this.dataArray.some(v => Math.abs(v - 128) > 2);

      // Only redraw if there's signal (keeps last frame when paused)
      if (hasSignal) {
        this.lastDataArray = new Uint8Array(this.dataArray);
      }

      this.drawWaveform(this.lastDataArray || this.dataArray);
    };

    draw();
  }

  drawWaveform(dataArray) {
    if (!this.ctx || !this.canvas || !dataArray) return;

    const rect = this.canvas.getBoundingClientRect();
    const width = rect.width;
    const height = rect.height;

    // Clear
    this.ctx.fillStyle = getComputedStyle(document.documentElement)
      .getPropertyValue('--waveform-bg') || '#0a0a1a';
    this.ctx.fillRect(0, 0, width, height);

    // Draw center line
    this.ctx.strokeStyle = 'rgba(255, 255, 255, 0.1)';
    this.ctx.lineWidth = 1;
    this.ctx.beginPath();
    this.ctx.moveTo(0, height / 2);
    this.ctx.lineTo(width, height / 2);
    this.ctx.stroke();

    // Draw waveform
    this.ctx.strokeStyle = '#00ff88';
    this.ctx.lineWidth = 2;
    this.ctx.beginPath();

    const bufferLength = dataArray.length;
    const sliceWidth = width / bufferLength;
    let x = 0;

    for (let i = 0; i < bufferLength; i++) {
      const v = dataArray[i] / 128.0; // normalize to 0-2
      const y = (v * height) / 2;

      if (i === 0) {
        this.ctx.moveTo(x, y);
      } else {
        this.ctx.lineTo(x, y);
      }

      x += sliceWidth;
    }

    this.ctx.stroke();

    // Draw playhead in center
    this.ctx.strokeStyle = '#fff';
    this.ctx.lineWidth = 2;
    this.ctx.beginPath();
    this.ctx.moveTo(width / 2, 0);
    this.ctx.lineTo(width / 2, height);
    this.ctx.stroke();
  }

  stopVisualization() {
    if (this.rafId) {
      cancelAnimationFrame(this.rafId);
      this.rafId = null;
    }
  }

  render() {
    return html`
      <div class="container">
        <canvas></canvas>
        <div class="label">REALTIME</div>
      </div>
    `;
  }
}

customElements.define('mixx-realtime-visualizer', MixxRealtimeVisualizer);
