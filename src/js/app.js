import { LitElement, html, css } from 'https://cdn.jsdelivr.net/gh/lit/dist@3/core/lit-core.min.js';
import './audio-player.js';
import './waveform.js';
import './beat-grid.js';
import './visualizer.js';

class MixxApp extends LitElement {
  static properties = {
    tracks: { type: Array },
    currentTrack: { type: Object },
    analysis: { type: Object },
    loading: { type: Boolean },
    selectedAnalyzer: { type: String },
  };

  static styles = css`
    :host {
      display: flex;
      flex-direction: column;
      height: 100%;
      background: var(--bg-primary);
    }

    header {
      padding: 1rem;
      background: var(--bg-secondary);
      border-bottom: 1px solid var(--bg-tertiary);
    }

    h1 {
      font-size: 1.5rem;
      font-weight: 600;
      margin-bottom: 0.5rem;
    }

    .content {
      display: flex;
      flex: 1;
      overflow: hidden;
    }

    .sidebar {
      width: 300px;
      background: var(--bg-secondary);
      border-right: 1px solid var(--bg-tertiary);
      overflow-y: auto;
    }

    .track-list {
      list-style: none;
    }

    .track-item {
      padding: 0.75rem 1rem;
      cursor: pointer;
      border-bottom: 1px solid var(--bg-tertiary);
      transition: background 0.2s;
    }

    .track-item:hover {
      background: var(--bg-tertiary);
    }

    .track-item.active {
      background: var(--accent-dim);
    }

    .track-item.no-analysis {
      opacity: 0.5;
    }

    .track-name {
      font-size: 0.9rem;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
    }

    .track-status {
      font-size: 0.75rem;
      color: var(--text-secondary);
      margin-top: 0.25rem;
    }

    .main {
      flex: 1;
      display: flex;
      flex-direction: column;
      padding: 1rem;
      gap: 1rem;
      overflow: hidden;
    }

    .visualizer-row {
      display: flex;
      gap: 1rem;
      min-height: 150px;
    }

    .waveform-container {
      flex: 1;
      background: var(--waveform-bg);
      border-radius: 8px;
      overflow: hidden;
    }

    .beat-indicator {
      width: 200px;
    }

    .analyzer-selector {
      display: flex;
      gap: 0.5rem;
      margin-bottom: 0.5rem;
    }

    .analyzer-btn {
      padding: 0.5rem 1rem;
      background: var(--bg-tertiary);
      border: none;
      border-radius: 4px;
      color: var(--text-primary);
      cursor: pointer;
      font-size: 0.85rem;
    }

    .analyzer-btn.active {
      background: var(--accent);
    }

    .analyzer-btn:disabled {
      opacity: 0.3;
      cursor: not-allowed;
    }

    .loading {
      display: flex;
      align-items: center;
      justify-content: center;
      height: 100%;
      color: var(--text-secondary);
    }

    .empty-state {
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: center;
      height: 100%;
      color: var(--text-secondary);
    }

    .empty-state p {
      margin-bottom: 1rem;
    }
  `;

  constructor() {
    super();
    this.tracks = [];
    this.currentTrack = null;
    this.analysis = null;
    this.loading = true;
    this.selectedAnalyzer = 'ml-python';
  }

  connectedCallback() {
    super.connectedCallback();
    this.fetchTracks();
  }

  async fetchTracks() {
    try {
      const response = await fetch('/api/music');
      this.tracks = await response.json();
    } catch (e) {
      console.error('Failed to fetch tracks:', e);
    } finally {
      this.loading = false;
    }
  }

  async selectTrack(track) {
    this.currentTrack = track;
    this.analysis = null;

    if (track.has_json) {
      try {
        const response = await fetch(`/api/music/${track.json_path}`);
        this.analysis = await response.json();

        // Select first available analyzer
        if (this.analysis.analyzers) {
          const analyzers = Object.keys(this.analysis.analyzers);
          if (!analyzers.includes(this.selectedAnalyzer)) {
            this.selectedAnalyzer = analyzers[0];
          }
        }
      } catch (e) {
        console.error('Failed to fetch analysis:', e);
      }
    }
  }

  selectAnalyzer(name) {
    this.selectedAnalyzer = name;
  }

  get currentBeats() {
    if (!this.analysis?.analyzers?.[this.selectedAnalyzer]) return [];
    return this.analysis.analyzers[this.selectedAnalyzer].beats || [];
  }

  get currentBPM() {
    if (!this.analysis?.analyzers?.[this.selectedAnalyzer]) return 0;
    return this.analysis.analyzers[this.selectedAnalyzer].bpm || 0;
  }

  get availableAnalyzers() {
    if (!this.analysis?.analyzers) return [];
    return Object.keys(this.analysis.analyzers);
  }

  render() {
    if (this.loading) {
      return html`<div class="loading">Loading tracks...</div>`;
    }

    return html`
      <header>
        <h1>Beat Grid Visualizer</h1>
      </header>
      <div class="content">
        <aside class="sidebar">
          <ul class="track-list">
            ${this.tracks.map(track => html`
              <li
                class="track-item ${track === this.currentTrack ? 'active' : ''} ${!track.has_json ? 'no-analysis' : ''}"
                @click=${() => this.selectTrack(track)}
              >
                <div class="track-name">${track.name}</div>
                <div class="track-status">
                  ${track.has_json ? 'Analyzed' : 'No analysis'}
                </div>
              </li>
            `)}
          </ul>
        </aside>
        <main class="main">
          ${this.currentTrack ? this.renderPlayer() : this.renderEmptyState()}
        </main>
      </div>
    `;
  }

  renderEmptyState() {
    return html`
      <div class="empty-state">
        <p>Select a track from the sidebar to begin</p>
      </div>
    `;
  }

  renderPlayer() {
    return html`
      ${this.analysis ? html`
        <div class="analyzer-selector">
          ${this.availableAnalyzers.map(name => {
            const a = this.analysis.analyzers[name];
            const hasError = a.error;
            return html`
              <button
                class="analyzer-btn ${name === this.selectedAnalyzer ? 'active' : ''}"
                ?disabled=${hasError}
                @click=${() => this.selectAnalyzer(name)}
                title=${hasError ? a.error : `${a.bpm?.toFixed(1)} BPM, ${a.beats?.length} beats`}
              >
                ${name}
                ${hasError ? ' (error)' : ` (${a.bpm?.toFixed(0)})`}
              </button>
            `;
          })}
        </div>
      ` : ''}

      <div class="visualizer-row">
        <div class="waveform-container">
          <mixx-waveform
            .beats=${this.currentBeats}
            .duration=${this.analysis?.duration || 0}
            .waveform=${this.analysis?.waveform || null}
          ></mixx-waveform>
        </div>
        <div class="beat-indicator">
          <mixx-visualizer
            .beats=${this.currentBeats}
            .bpm=${this.currentBPM}
          ></mixx-visualizer>
        </div>
      </div>

      <mixx-transport
        .track=${this.currentTrack}
        .duration=${this.analysis?.duration || 0}
        .bpm=${this.currentBPM}
      ></mixx-transport>
    `;
  }
}

customElements.define('mixx-app', MixxApp);

// Transport component
class MixxTransport extends LitElement {
  static properties = {
    track: { type: Object },
    duration: { type: Number },
    bpm: { type: Number },
    playing: { type: Boolean },
    currentTime: { type: Number },
  };

  static styles = css`
    :host {
      display: block;
      background: var(--bg-secondary);
      border-radius: 8px;
      padding: 1rem;
    }

    .controls {
      display: flex;
      align-items: center;
      gap: 1rem;
    }

    .play-btn {
      width: 50px;
      height: 50px;
      border-radius: 50%;
      border: none;
      background: var(--accent);
      color: white;
      font-size: 1.25rem;
      cursor: pointer;
      display: flex;
      align-items: center;
      justify-content: center;
    }

    .play-btn:hover {
      filter: brightness(1.1);
    }

    .time {
      font-family: monospace;
      font-size: 1rem;
      color: var(--text-secondary);
    }

    .info {
      margin-left: auto;
      text-align: right;
      font-size: 0.9rem;
      color: var(--text-secondary);
    }

    .bpm {
      font-size: 1.25rem;
      color: var(--accent);
      font-weight: 600;
    }

    audio {
      display: none;
    }
  `;

  constructor() {
    super();
    this.playing = false;
    this.currentTime = 0;
    this.audioEl = null;
  }

  updated(changed) {
    if (changed.has('track') && this.track) {
      this.loadTrack();
    }
  }

  loadTrack() {
    this.playing = false;
    this.currentTime = 0;

    if (this.audioEl) {
      this.audioEl.pause();
      this.audioEl.src = `/api/music/${this.track.path}`;
      this.audioEl.load();
    }
  }

  firstUpdated() {
    this.audioEl = this.renderRoot.querySelector('audio');
    this.audioEl.addEventListener('timeupdate', () => {
      this.currentTime = this.audioEl.currentTime;
      // Dispatch event for other components
      this.dispatchEvent(new CustomEvent('timeupdate', {
        detail: { time: this.currentTime },
        bubbles: true,
        composed: true
      }));
    });
    this.audioEl.addEventListener('ended', () => {
      this.playing = false;
    });

    // Listen for seek events from waveform
    window.addEventListener('seek', (e) => {
      if (e.detail?.time !== undefined && this.audioEl) {
        this.audioEl.currentTime = e.detail.time;
      }
    });
  }

  togglePlay() {
    if (this.playing) {
      this.audioEl.pause();
    } else {
      this.audioEl.play();
    }
    this.playing = !this.playing;
  }

  formatTime(seconds) {
    const m = Math.floor(seconds / 60);
    const s = Math.floor(seconds % 60);
    return `${m}:${s.toString().padStart(2, '0')}`;
  }

  render() {
    return html`
      <audio></audio>
      <div class="controls">
        <button class="play-btn" @click=${this.togglePlay}>
          ${this.playing ? '⏸' : '▶'}
        </button>
        <span class="time">
          ${this.formatTime(this.currentTime)} / ${this.formatTime(this.duration)}
        </span>
        <div class="info">
          <div class="bpm">${this.bpm?.toFixed(1) || '—'} BPM</div>
        </div>
      </div>
    `;
  }
}

customElements.define('mixx-transport', MixxTransport);
