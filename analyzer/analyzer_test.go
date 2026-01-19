package analyzer

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTracks(t *testing.T) {
	// WIRED CD: Rip. Sample. Mash. Share. - Creative Commons compilation (2004)
	// https://archive.org/details/The_WIRED_CD_Rip_Sample_Mash_Share-2769
	const baseURL = "https://archive.org/download/The_WIRED_CD_Rip_Sample_Mash_Share-2769/"

	// Expected outputs for each analyzer:
	// - outMixxx: Mixxx qm-dsp beat detection (C++)
	// - outPyTF:  Python TensorFlow (soundfile decoder)
	// - outGoTF:  Go TensorFlow (go-mp3 decoder)
	tests := []struct {
		inFile   string
		outMixxx AnalyzeOut
		outPyTF  MLAnalyzeOut
		outGoTF  MLAnalyzeOut
	}{
		{
			"Beastie_Boys_-_01_-_Now_Get_Busy.mp3",
			AnalyzeOut{BPM: 113.01, SampleRate: 44100, Duration: 141.86},
			MLAnalyzeOut{BPM: 113.21, Duration: 141.86},
			MLAnalyzeOut{BPM: 113.21, Duration: 145.89},
		},
		{
			"David_Byrne_-_02_-_My_Fair_Lady.mp3",
			AnalyzeOut{BPM: 140.01, SampleRate: 44100, Duration: 208.34},
			MLAnalyzeOut{BPM: 139.53, Duration: 208.34},
			MLAnalyzeOut{BPM: 139.53, Duration: 211.96},
		},
		{
			"Zap_Mama_-_03_-_Wadidyusay.mp3",
			AnalyzeOut{BPM: 92.99, SampleRate: 44100, Duration: 196.43},
			MLAnalyzeOut{BPM: 93.75, Duration: 196.43},
			MLAnalyzeOut{BPM: 93.75, Duration: 200.83},
		},
		{
			"My_Morning_Jacket_-_04_-_One_Big_Holiday.mp3",
			AnalyzeOut{BPM: 137.13, SampleRate: 44100, Duration: 317.35},
			MLAnalyzeOut{BPM: 136.36, Duration: 317.35},
			MLAnalyzeOut{BPM: 136.36, Duration: 321.67},
		},
		{
			"Spoon_-_05_-_Revenge.mp3",
			AnalyzeOut{BPM: 129.53, SampleRate: 44100, Duration: 144.81},
			MLAnalyzeOut{BPM: 122.45, Duration: 144.81},
			MLAnalyzeOut{BPM: 122.45, Duration: 147.91},
		},
		{
			"Gilberto_Gil_-_06_-_Oslodum.mp3",
			AnalyzeOut{BPM: 91.65, SampleRate: 44100, Duration: 234.10},
			MLAnalyzeOut{BPM: 90.91, Duration: 234.10},
			MLAnalyzeOut{BPM: 90.91, Duration: 238.16},
		},
		{
			"Dan_the_Automator_-_07_-_Relaxation_Spa_Treatment.mp3",
			AnalyzeOut{BPM: 85.00, SampleRate: 44100, Duration: 198.88},
			MLAnalyzeOut{BPM: 84.51, Duration: 198.88},
			MLAnalyzeOut{BPM: 84.51, Duration: 205.11},
		},
		{
			"Thievery_Corporation_-_08_-_DC_3000.mp3",
			AnalyzeOut{BPM: 90.17, SampleRate: 44100, Duration: 261.60},
			MLAnalyzeOut{BPM: 89.55, Duration: 261.60},
			MLAnalyzeOut{BPM: 89.55, Duration: 267.57},
		},
		{
			"Le_Tigre_-_09_-_Fake_French.mp3",
			AnalyzeOut{BPM: 90.01, SampleRate: 44100, Duration: 169.16},
			MLAnalyzeOut{BPM: 90.91, Duration: 169.16},
			MLAnalyzeOut{BPM: 89.55, Duration: 172.64},
		},
		{
			"Paul_Westerberg_-_10_-_Looking_Up_in_Heaven.mp3",
			AnalyzeOut{BPM: 147.78, SampleRate: 44100, Duration: 188.33},
			MLAnalyzeOut{BPM: 139.53, Duration: 188.33},
			MLAnalyzeOut{BPM: 142.86, Duration: 192.05},
		},
		{
			"Chuck_D_-_11_-_No_Meaning_No_feat_Fine_Arts_Militia.mp3",
			AnalyzeOut{BPM: 91.87, SampleRate: 44100, Duration: 189.98},
			MLAnalyzeOut{BPM: 92.31, Duration: 189.98},
			MLAnalyzeOut{BPM: 92.31, Duration: 192.71},
		},
		{
			"The_Rapture_-_12_-_Sister_Saviour_Blackstrobe_Remix.mp3",
			AnalyzeOut{BPM: 116.81, SampleRate: 44100, Duration: 417.71},
			MLAnalyzeOut{BPM: 117.65, Duration: 417.71},
			MLAnalyzeOut{BPM: 115.38, Duration: 425.22},
		},
		{
			"Cornelius_-_13_-_Wataridori_2.mp3",
			AnalyzeOut{BPM: 140.02, SampleRate: 44100, Duration: 422.28},
			MLAnalyzeOut{BPM: 133.33, Duration: 422.28},
			MLAnalyzeOut{BPM: 133.33, Duration: 429.69},
		},
		{
			"DJ_Danger_Mouse_-_14_-_What_U_Sittin_On_feat_Jemini_Cee_Lo_And_Tha_Alkaholiks.mp3",
			AnalyzeOut{BPM: 91.64, SampleRate: 44100, Duration: 204.27},
			MLAnalyzeOut{BPM: 90.91, Duration: 204.27},
			MLAnalyzeOut{BPM: 90.91, Duration: 207.97},
		},
		{
			"DJ_Dolores_-_15_-_Oslodum_2004.mp3",
			AnalyzeOut{BPM: 92.09, SampleRate: 44100, Duration: 235.40},
			MLAnalyzeOut{BPM: 92.31, Duration: 235.40},
			MLAnalyzeOut{BPM: 92.31, Duration: 240.22},
		},
		{
			"Matmos_-_16_-_Action_at_a_Distance.mp3",
			AnalyzeOut{BPM: 164.18, SampleRate: 44100, Duration: 160.80},
			MLAnalyzeOut{BPM: 98.36, Duration: 160.80},
			MLAnalyzeOut{BPM: 86.33, Duration: 164.44}, // go-mp3 handles this corrupted MP3 differently
		},
	}

	// Create analyzers once for all tests
	pyAnalyzer, pyErr := NewMLAnalyzer()
	if pyErr != nil {
		t.Logf("Python TF Analyzer not available: %v", pyErr)
	}

	goAnalyzer, goErr := NewTFAnalyzer()
	if goErr != nil {
		t.Logf("Go TF Analyzer not available: %v", goErr)
	}
	if goAnalyzer != nil {
		defer goAnalyzer.Close()
	}

	for _, tt := range tests {
		t.Run(tt.inFile, func(t *testing.T) {
			path := get(t, baseURL+tt.inFile)

			// Test 1: Mixxx (qm-dsp) analyzer
			mixxx, err := AnalyzeFile(path)
			require.NoError(t, err, "Mixxx analysis failed")

			assert.InDelta(t, tt.outMixxx.BPM, mixxx.BPM, 1.0, "Mixxx BPM")
			assert.Equal(t, tt.outMixxx.SampleRate, mixxx.SampleRate, "Mixxx SampleRate")
			assert.InDelta(t, tt.outMixxx.Duration, mixxx.Duration, 0.1, "Mixxx Duration")
			assert.NotEmpty(t, mixxx.Beats, "Mixxx Beats")

			t.Logf("Mixxx: BPM=%.2f, Bars=%.1f, Beats=%d, Duration=%.2fs",
				mixxx.BPM, mixxx.Bars(), len(mixxx.Beats), mixxx.Duration)

			// Test 2: Python TensorFlow analyzer
			if pyAnalyzer != nil {
				pyTF, err := pyAnalyzer.AnalyzeFile(path)
				require.NoError(t, err, "Python TF analysis failed")

				assert.InDelta(t, tt.outPyTF.BPM, pyTF.BPM, 2.0, "Python TF BPM")
				assert.InDelta(t, tt.outPyTF.Duration, pyTF.Duration, 0.5, "Python TF Duration")
				assert.NotEmpty(t, pyTF.Beats, "Python TF Beats")

				t.Logf("PyTF:  BPM=%.2f, Bars=%.1f, Beats=%d, Duration=%.2fs",
					pyTF.BPM, pyTF.Bars, pyTF.NumBeats, pyTF.Duration)
			}

			// Test 3: Go TensorFlow analyzer
			if goAnalyzer != nil {
				goTF, err := goAnalyzer.AnalyzeFile(path)
				require.NoError(t, err, "Go TF analysis failed")

				assert.InDelta(t, tt.outGoTF.BPM, goTF.BPM, 2.0, "Go TF BPM")
				assert.InDelta(t, tt.outGoTF.Duration, goTF.Duration, 0.5, "Go TF Duration")
				assert.NotEmpty(t, goTF.Beats, "Go TF Beats")

				t.Logf("GoTF:  BPM=%.2f, Bars=%.1f, Beats=%d, Duration=%.2fs",
					goTF.BPM, goTF.Bars, goTF.NumBeats, goTF.Duration)
			}
		})
	}
}

func get(t *testing.T, url string) string {
	t.Helper()

	dir := "fixtures"
	require.NoError(t, os.MkdirAll(dir, 0755))

	name := filepath.Base(url)
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); err == nil {
		return path
	}

	t.Logf("Downloading %s...", name)

	resp, err := http.Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	require.NoError(t, err)

	return path
}
