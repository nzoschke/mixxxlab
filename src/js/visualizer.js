import { LitElement, html, css } from 'https://cdn.jsdelivr.net/gh/lit/dist@3/core/lit-core.min.js';
import { BeatGrid } from './beat-grid.js';

class MixxVisualizer extends LitElement {
  static properties = {
    beats: { type: Array },
    bpm: { type: Number },
    currentTime: { type: Number },
  };

  static styles = css`
    :host {
      display: flex;
      flex-direction: column;
      height: 100%;
      gap: 0.5rem;
    }

    .beat-boxes {
      display: grid;
      grid-template-columns: repeat(2, 1fr);
      grid-template-rows: repeat(2, 1fr);
      gap: 0.5rem;
      flex: 1;
    }

    .beat-box {
      border-radius: 8px;
      display: flex;
      align-items: center;
      justify-content: center;
      font-size: 1.5rem;
      font-weight: bold;
      transition: transform 0.05s ease-out;
    }

    .beat-box.active {
      transform: scale(1.05);
    }

    .beat-1 {
      background: var(--beat-1);
      opacity: 0.3;
    }
    .beat-1.active {
      opacity: 1;
      box-shadow: 0 0 20px var(--beat-1);
    }

    .beat-2 {
      background: var(--beat-2);
      opacity: 0.3;
    }
    .beat-2.active {
      opacity: 1;
      box-shadow: 0 0 20px var(--beat-2);
    }

    .beat-3 {
      background: var(--beat-3);
      opacity: 0.3;
    }
    .beat-3.active {
      opacity: 1;
      box-shadow: 0 0 20px var(--beat-3);
    }

    .beat-4 {
      background: var(--beat-4);
      opacity: 0.3;
    }
    .beat-4.active {
      opacity: 1;
      box-shadow: 0 0 20px var(--beat-4);
    }

    .bar-info {
      text-align: center;
      font-size: 0.9rem;
      color: var(--text-secondary);
      padding: 0.5rem;
      background: var(--bg-tertiary);
      border-radius: 4px;
    }

    .bar-number {
      font-size: 1.25rem;
      font-weight: bold;
      color: var(--text-primary);
    }
  `;

  constructor() {
    super();
    this.beats = [];
    this.bpm = 0;
    this.currentTime = 0;
    this.beatGrid = null;
    this.animationId = null;
  }

  connectedCallback() {
    super.connectedCallback();
    window.addEventListener('timeupdate', this.handleTimeUpdate.bind(this));
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    window.removeEventListener('timeupdate', this.handleTimeUpdate.bind(this));
    if (this.animationId) {
      cancelAnimationFrame(this.animationId);
    }
  }

  handleTimeUpdate(e) {
    if (e.detail?.time !== undefined) {
      this.currentTime = e.detail.time;
    }
  }

  updated(changed) {
    if (changed.has('beats') || changed.has('bpm')) {
      this.beatGrid = new BeatGrid(this.beats, this.bpm);
    }
  }

  get activeBeat() {
    if (!this.beatGrid) return -1;
    return this.beatGrid.getBeatInBar(this.currentTime);
  }

  get currentBar() {
    if (!this.beatGrid) return 1;
    return this.beatGrid.getBar(this.currentTime);
  }

  get totalBars() {
    return Math.ceil(this.beats.length / 4);
  }

  render() {
    const active = this.activeBeat;

    return html`
      <div class="beat-boxes">
        <div class="beat-box beat-1 ${active === 0 ? 'active' : ''}">1</div>
        <div class="beat-box beat-2 ${active === 1 ? 'active' : ''}">2</div>
        <div class="beat-box beat-3 ${active === 2 ? 'active' : ''}">3</div>
        <div class="beat-box beat-4 ${active === 3 ? 'active' : ''}">4</div>
      </div>
      <div class="bar-info">
        Bar <span class="bar-number">${this.currentBar}</span> / ${this.totalBars}
      </div>
    `;
  }
}

customElements.define('mixx-visualizer', MixxVisualizer);
