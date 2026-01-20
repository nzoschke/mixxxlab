import { LitElement, html, css } from 'https://cdn.jsdelivr.net/gh/lit/dist@3/core/lit-core.min.js';
import { AudioEngine } from './audio-engine.js';
import './waveform.js';
import './beat-grid.js';
import './visualizer.js';
import './realtime-visualizer.js';

class MixxApp extends LitElement {
  static properties = {
    tracks: { type: Array },
    currentTrack: { type: Object },
    analysis: { type: Object },
    loading: { type: Boolean },
    selectedGrid: { type: String },
    selectedMarker: { type: String },
    waveformZoom: { type: Number },
    audioEngine: { type: Object },
  };

  static styles = css`
    :host {
      display: grid;
      grid-template-areas:
        "header header"
        "sidebar main";
      grid-template-rows: auto minmax(0, 1fr);
      grid-template-columns: 300px 1fr;
      height: 100%;
      overflow: hidden;
      background: var(--bg-primary);
    }

    header {
      grid-area: header;
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 0.75rem 1rem;
      background: var(--bg-secondary);
      border-bottom: 1px solid var(--bg-tertiary);
    }

    h1 {
      font-size: 1.25rem;
      font-weight: 600;
      margin: 0;
    }

    .sidebar {
      grid-area: sidebar;
      min-height: 0;
      overflow-y: auto;
      background: var(--bg-secondary);
      border-right: 1px solid var(--bg-tertiary);
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
      grid-area: main;
      display: flex;
      flex-direction: column;
      padding: 0.75rem;
      gap: 0.75rem;
      overflow: hidden;
    }

    .visualizer-row {
      display: flex;
      gap: 0.75rem;
      height: 120px;
      min-height: 120px;
      max-height: 120px;
    }

    .waveform-container {
      flex: 1;
      height: 100%;
      background: var(--waveform-bg);
      border-radius: 6px;
      overflow: hidden;
    }

    .overview-container {
      height: 40px;
      flex-shrink: 0;
      background: var(--waveform-bg);
      border-radius: 6px;
      overflow: hidden;
    }

    .realtime-container {
      height: 60px;
      flex-shrink: 0;
      background: var(--waveform-bg);
      border-radius: 6px;
      overflow: hidden;
    }

    .beat-indicator {
      width: 200px;
      height: 120px;
      overflow: hidden;
    }

    .controls-row {
      display: flex;
      gap: 2rem;
      align-items: center;
    }

    .control-group {
      display: flex;
      align-items: center;
      gap: 0.5rem;
    }

    .control-label {
      font-size: 0.7rem;
      color: var(--text-secondary);
      text-transform: uppercase;
      letter-spacing: 0.05em;
    }

    .analyzer-selector {
      display: flex;
      gap: 0.25rem;
    }

    .analyzer-btn {
      padding: 0.3rem 0.6rem;
      background: var(--bg-tertiary);
      border: none;
      border-radius: 4px;
      color: var(--text-primary);
      cursor: pointer;
      font-size: 0.75rem;
    }

    .analyzer-btn.active {
      background: var(--accent);
    }

    .analyzer-btn:disabled {
      opacity: 0.3;
      cursor: not-allowed;
    }

    .toggle-btn {
      padding: 0.3rem 0.6rem;
      background: var(--bg-tertiary);
      border: none;
      border-radius: 4px;
      color: var(--text-secondary);
      cursor: pointer;
      font-size: 0.75rem;
      transition: all 0.2s;
    }

    .toggle-btn.active {
      background: var(--accent-dim);
      color: var(--text-primary);
    }

    .toggle-btn:hover {
      background: var(--bg-tertiary);
      color: var(--text-primary);
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
    this.selectedGrid = 'beatthis';
    this.selectedMarker = 'mixx';
    this.waveformZoom = 1;
    this.audioEngine = null;
  }

  handleAudioReady(e) {
    this.audioEngine = e.detail.engine;
  }

  connectedCallback() {
    super.connectedCallback();
    this.fetchTracks();

    // Global keyboard shortcuts
    this.handleKeyDown = (e) => {
      if (e.code === 'Space' && e.target.tagName !== 'INPUT') {
        e.preventDefault();
        this.togglePlayPause();
      }
    };
    window.addEventListener('keydown', this.handleKeyDown);
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    window.removeEventListener('keydown', this.handleKeyDown);
  }

  togglePlayPause() {
    if (this.audioEngine) {
      if (this.audioEngine.paused) {
        this.audioEngine.play();
      } else {
        this.audioEngine.pause();
      }
    }
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
    this.waveformZoom = 1; // Reset zoom on track change

    if (track.has_json) {
      try {
        const response = await fetch(`/api/music/${track.json_path}`);
        this.analysis = await response.json();

        // Select first available grid
        if (this.analysis.grids) {
          const grids = Object.keys(this.analysis.grids);
          if (!grids.includes(this.selectedGrid)) {
            this.selectedGrid = grids[0];
          }
        }

        // Select first available marker
        if (this.analysis.markers) {
          const markers = Object.keys(this.analysis.markers);
          if (!markers.includes(this.selectedMarker)) {
            this.selectedMarker = markers[0];
          }
        }
      } catch (e) {
        console.error('Failed to fetch analysis:', e);
      }
    }
  }

  selectGrid(name) {
    this.selectedGrid = name;
  }

  selectMarker(name) {
    this.selectedMarker = name;
  }

  handleZoomChange(e) {
    this.waveformZoom = e.detail.zoom;
  }

  get currentBeats() {
    if (!this.analysis?.grids?.[this.selectedGrid]) return [];
    return this.analysis.grids[this.selectedGrid].beats || [];
  }

  get currentBPM() {
    if (!this.analysis?.grids?.[this.selectedGrid]) return 0;
    return this.analysis.grids[this.selectedGrid].bpm || 0;
  }

  get currentCuePoints() {
    if (!this.selectedMarker) return [];
    return this.analysis?.markers?.[this.selectedMarker]?.cue_points || [];
  }

  get currentPhrases() {
    if (!this.selectedMarker) return [];
    return this.analysis?.markers?.[this.selectedMarker]?.phrases || [];
  }

  get availableGrids() {
    if (!this.analysis?.grids) return [];
    return Object.keys(this.analysis.grids);
  }

  get availableMarkers() {
    if (!this.analysis?.markers) return [];
    return Object.keys(this.analysis.markers);
  }

  formatGridName(name) {
    const names = {
      'mixx': 'Mixx',
      'mixx-extended': 'Mixx+',
      'rekordbox-py': 'RekordboxPy',
      'rekordbox-go': 'RekordboxGo',
      'beatthis': 'BeatThis',
      'beatthis-full': 'BeatThis+',
    };
    return names[name] || name;
  }

  formatMarkerName(name) {
    const names = {
      'mixx': 'Mixx',
      'beats': 'Beats',
      'songformer': 'SongFormer',
      'rekordbox': 'Rekordbox',
    };
    return names[name] || name;
  }

  render() {
    if (this.loading) {
      return html`<div class="loading">Loading tracks...</div>`;
    }

    return html`
      <header>
        <h1>Beat Grid Visualizer</h1>
        ${this.analysis ? html`
          <div class="controls-row">
            <div class="control-group">
              <span class="control-label">Grid</span>
              <div class="analyzer-selector">
                ${this.availableGrids.map(name => {
                  const g = this.analysis.grids[name];
                  const hasError = g.error;
                  const hasDownbeats = g.downbeats?.length > 0;
                  return html`
                    <button
                      class="analyzer-btn ${name === this.selectedGrid ? 'active' : ''}"
                      ?disabled=${hasError}
                      @click=${() => this.selectGrid(name)}
                      title=${hasError ? g.error : `${g.bpm?.toFixed(1)} BPM, ${g.beats?.length} beats${hasDownbeats ? ', has downbeats' : ''}`}
                    >
                      ${this.formatGridName(name)}
                    </button>
                  `;
                })}
              </div>
            </div>
            ${this.availableMarkers.length > 0 ? html`
              <div class="control-group">
                <span class="control-label">Markers</span>
                <div class="analyzer-selector">
                  ${this.availableMarkers.map(name => {
                    const m = this.analysis.markers[name];
                    const hasError = m.error;
                    const hasCues = m.cue_points?.length > 0;
                    const hasPhrases = m.phrases?.length > 0;
                    const info = [];
                    if (hasCues) info.push(`${m.cue_points.length} cues`);
                    if (hasPhrases) info.push(`${m.phrases.length} phrases`);
                    return html`
                      <button
                        class="analyzer-btn ${name === this.selectedMarker ? 'active' : ''}"
                        ?disabled=${hasError}
                        @click=${() => this.selectMarker(name)}
                        title=${hasError ? m.error : info.join(', ')}
                      >
                        ${this.formatMarkerName(name)}
                      </button>
                    `;
                  })}
                </div>
              </div>
            ` : ''}
          </div>
        ` : ''}
      </header>
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
      <div class="overview-container">
        <mixx-waveform-overview
          .beats=${this.currentBeats}
          .cuePoints=${this.currentCuePoints}
          .phrases=${this.currentPhrases}
          .duration=${this.analysis?.duration || 0}
          .waveform=${this.analysis?.waveform || null}
          .zoom=${this.waveformZoom}
        ></mixx-waveform-overview>
      </div>

      <div class="visualizer-row">
        <div class="waveform-container">
          <mixx-waveform
            .beats=${this.currentBeats}
            .cuePoints=${this.currentCuePoints}
            .phrases=${this.currentPhrases}
            .duration=${this.analysis?.duration || 0}
            .waveform=${this.analysis?.waveform || null}
            @zoomchange=${this.handleZoomChange}
          ></mixx-waveform>
        </div>
        <div class="beat-indicator">
          <mixx-visualizer
            .beats=${this.currentBeats}
            .bpm=${this.currentBPM}
          ></mixx-visualizer>
        </div>
      </div>

      <div class="realtime-container">
        <mixx-realtime-visualizer
          .audioEngine=${this.audioEngine}
        ></mixx-realtime-visualizer>
      </div>

      <mixx-transport
        .track=${this.currentTrack}
        .duration=${this.analysis?.duration || 0}
        .bpm=${this.currentBPM}
        @audioready=${this.handleAudioReady}
      ></mixx-transport>
    `;
  }
}

customElements.define('mixx-app', MixxApp);

// Transport component - uses AudioEngine for sample-accurate playback
class MixxTransport extends LitElement {
  static properties = {
    track: { type: Object },
    duration: { type: Number },
    bpm: { type: Number },
    playing: { type: Boolean },
    currentTime: { type: Number },
    loading: { type: Boolean },
  };

  static styles = css`
    :host {
      display: block;
      background: var(--bg-secondary);
      border-radius: 8px;
      padding: 1rem;
      flex-shrink: 0;
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

    .play-btn:disabled {
      opacity: 0.5;
      cursor: not-allowed;
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

    .loading-indicator {
      font-size: 0.8rem;
      color: var(--text-secondary);
    }
  `;

  constructor() {
    super();
    this.playing = false;
    this.currentTime = 0;
    this.loading = false;
    this.engine = null;
    this.rafId = null;
  }

  connectedCallback() {
    super.connectedCallback();

    // Create engine instance
    this.engine = new AudioEngine();

    // Listen for engine events
    this.engine.addEventListener('play', () => {
      this.playing = true;
      this.startTimeLoop();
    });

    this.engine.addEventListener('pause', () => {
      this.playing = false;
      this.stopTimeLoop();
    });

    this.engine.addEventListener('ended', () => {
      this.playing = false;
      this.currentTime = 0;
      this.stopTimeLoop();
      this.broadcastTime();
    });

    this.engine.addEventListener('loadstart', () => {
      this.loading = true;
    });

    this.engine.addEventListener('canplay', () => {
      this.loading = false;
    });

    this.engine.addEventListener('error', (e) => {
      this.loading = false;
      console.error('Audio engine error:', e.detail?.error);
    });

    this.engine.addEventListener('seeked', () => {
      this.broadcastTime();
    });

    // Listen for seek events from waveform
    this._seekHandler = (e) => {
      if (e.detail?.time !== undefined && this.engine) {
        this.engine.seek(e.detail.time);
      }
    };
    window.addEventListener('seek', this._seekHandler);

    // Notify parent that engine is ready
    this.dispatchEvent(new CustomEvent('audioready', {
      detail: { engine: this.engine },
      bubbles: true,
      composed: true
    }));
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    this.stopTimeLoop();
    window.removeEventListener('seek', this._seekHandler);
    if (this.engine) {
      this.engine.dispose();
      this.engine = null;
    }
  }

  updated(changed) {
    if (changed.has('track') && this.track) {
      this.loadTrack();
    }
  }

  async loadTrack() {
    this.playing = false;
    this.currentTime = 0;

    if (this.engine && this.track) {
      try {
        await this.engine.load(`/api/music/${this.track.path}`);
      } catch (e) {
        console.error('Failed to load track:', e);
      }
    }
  }

  broadcastTime() {
    if (this.engine) {
      this.currentTime = this.engine.getCurrentTime();
    }
    window.dispatchEvent(new CustomEvent('timeupdate', {
      detail: { time: this.currentTime }
    }));
  }

  startTimeLoop() {
    const update = () => {
      if (this.playing) {
        this.broadcastTime();
        this.rafId = requestAnimationFrame(update);
      }
    };
    this.rafId = requestAnimationFrame(update);
  }

  stopTimeLoop() {
    if (this.rafId) {
      cancelAnimationFrame(this.rafId);
      this.rafId = null;
    }
  }

  togglePlay() {
    if (!this.engine) return;

    if (this.engine.paused) {
      this.engine.play();
    } else {
      this.engine.pause();
    }
  }

  formatTime(seconds) {
    const m = Math.floor(seconds / 60);
    const s = Math.floor(seconds % 60);
    return `${m}:${s.toString().padStart(2, '0')}`;
  }

  render() {
    return html`
      <div class="controls">
        <button class="play-btn" @click=${this.togglePlay} ?disabled=${this.loading}>
          ${this.loading ? '...' : (this.playing ? '⏸' : '▶')}
        </button>
        <span class="time">
          ${this.formatTime(this.currentTime)} / ${this.formatTime(this.duration)}
          ${this.loading ? html`<span class="loading-indicator">(loading...)</span>` : ''}
        </span>
        <div class="info">
          <div class="bpm">${this.bpm?.toFixed(1) || '—'} BPM</div>
        </div>
      </div>
    `;
  }
}

customElements.define('mixx-transport', MixxTransport);
