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
	tests := []struct {
		inURL string
		out   AnalyzeOut
	}{
		{"https://archive.org/download/The_WIRED_CD_Rip_Sample_Mash_Share-2769/Beastie_Boys_-_01_-_Now_Get_Busy.mp3", AnalyzeOut{BPM: 113.01, SampleRate: 44100, Duration: 141.86}},
		{"https://archive.org/download/The_WIRED_CD_Rip_Sample_Mash_Share-2769/David_Byrne_-_02_-_My_Fair_Lady.mp3", AnalyzeOut{BPM: 140.01, SampleRate: 44100, Duration: 208.34}},
		{"https://archive.org/download/The_WIRED_CD_Rip_Sample_Mash_Share-2769/Zap_Mama_-_03_-_Wadidyusay.mp3", AnalyzeOut{BPM: 92.99, SampleRate: 44100, Duration: 196.43}},
		{"https://archive.org/download/The_WIRED_CD_Rip_Sample_Mash_Share-2769/My_Morning_Jacket_-_04_-_One_Big_Holiday.mp3", AnalyzeOut{BPM: 137.13, SampleRate: 44100, Duration: 317.35}},
		{"https://archive.org/download/The_WIRED_CD_Rip_Sample_Mash_Share-2769/Spoon_-_05_-_Revenge.mp3", AnalyzeOut{BPM: 129.53, SampleRate: 44100, Duration: 144.81}},
		{"https://archive.org/download/The_WIRED_CD_Rip_Sample_Mash_Share-2769/Gilberto_Gil_-_06_-_Oslodum.mp3", AnalyzeOut{BPM: 91.65, SampleRate: 44100, Duration: 234.10}},
		{"https://archive.org/download/The_WIRED_CD_Rip_Sample_Mash_Share-2769/Dan_the_Automator_-_07_-_Relaxation_Spa_Treatment.mp3", AnalyzeOut{BPM: 85.00, SampleRate: 44100, Duration: 198.88}},
		{"https://archive.org/download/The_WIRED_CD_Rip_Sample_Mash_Share-2769/Thievery_Corporation_-_08_-_DC_3000.mp3", AnalyzeOut{BPM: 90.17, SampleRate: 44100, Duration: 261.60}},
		{"https://archive.org/download/The_WIRED_CD_Rip_Sample_Mash_Share-2769/Le_Tigre_-_09_-_Fake_French.mp3", AnalyzeOut{BPM: 90.01, SampleRate: 44100, Duration: 169.16}},
		{"https://archive.org/download/The_WIRED_CD_Rip_Sample_Mash_Share-2769/Paul_Westerberg_-_10_-_Looking_Up_in_Heaven.mp3", AnalyzeOut{BPM: 147.78, SampleRate: 44100, Duration: 188.33}},
		{"https://archive.org/download/The_WIRED_CD_Rip_Sample_Mash_Share-2769/Chuck_D_-_11_-_No_Meaning_No_feat_Fine_Arts_Militia.mp3", AnalyzeOut{BPM: 91.87, SampleRate: 44100, Duration: 189.98}},
		{"https://archive.org/download/The_WIRED_CD_Rip_Sample_Mash_Share-2769/The_Rapture_-_12_-_Sister_Saviour_Blackstrobe_Remix.mp3", AnalyzeOut{BPM: 116.81, SampleRate: 44100, Duration: 417.71}},
		{"https://archive.org/download/The_WIRED_CD_Rip_Sample_Mash_Share-2769/Cornelius_-_13_-_Wataridori_2.mp3", AnalyzeOut{BPM: 140.02, SampleRate: 44100, Duration: 422.28}},
		{"https://archive.org/download/The_WIRED_CD_Rip_Sample_Mash_Share-2769/DJ_Danger_Mouse_-_14_-_What_U_Sittin_On_feat_Jemini_Cee_Lo_And_Tha_Alkaholiks.mp3", AnalyzeOut{BPM: 91.64, SampleRate: 44100, Duration: 204.27}},
		{"https://archive.org/download/The_WIRED_CD_Rip_Sample_Mash_Share-2769/DJ_Dolores_-_15_-_Oslodum_2004.mp3", AnalyzeOut{BPM: 92.09, SampleRate: 44100, Duration: 235.40}},
		{"https://archive.org/download/The_WIRED_CD_Rip_Sample_Mash_Share-2769/Matmos_-_16_-_Action_at_a_Distance.mp3", AnalyzeOut{BPM: 164.18, SampleRate: 44100, Duration: 160.80}},
	}

	for _, tt := range tests {
		name := filepath.Base(tt.inURL)
		t.Run(name, func(t *testing.T) {
			path := get(t, tt.inURL)

			got, err := AnalyzeFile(path)
			require.NoError(t, err)

			assert.InDelta(t, tt.out.BPM, got.BPM, 1.0, "BPM")
			assert.Equal(t, tt.out.SampleRate, got.SampleRate, "SampleRate")
			assert.InDelta(t, tt.out.Duration, got.Duration, 0.1, "Duration")
			assert.NotEmpty(t, got.Beats, "Beats")
			assert.NotZero(t, got.TotalFrames, "TotalFrames")

			t.Logf("BPM: %.2f, Bars: %.1f, Beats: %d, Duration: %.2fs",
				got.BPM, got.Bars(), len(got.Beats), got.Duration)
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
