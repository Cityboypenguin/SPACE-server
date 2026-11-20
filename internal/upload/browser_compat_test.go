package upload

import "testing"

// SPACE-client の圧縮（ブラウザーの WebCodecs + Mediabunny）が実際に書き出した動画が、
// サーバーの検査を通ることを確かめる。クライアント側の書き出し方（moov を先頭に置く、
// 箱の並び、音声の edit list）を変えたとき、ここで食い違いに気づける。
//
// ファイルは Google Chrome で SPACE-client/e2e/fixtures/video の動画を圧縮したもの
// （browser_360p_audio: 640×360・24fps・AAC、browser_4k_to_720p_noaudio: 4K から 1280×720・音声なし）。
func TestInspectMP4_AcceptsBrowserOutput(t *testing.T) {
	cases := []struct {
		name          string
		width, height int
		audio         bool
	}{
		{"browser_360p_audio.mp4", 640, 360, true},
		{"browser_4k_to_720p_noaudio.mp4", 1280, 720, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			info, err := inspect(fixture(t, c.name))
			if err != nil {
				t.Fatalf("InspectMP4: %v", err)
			}
			if info.Width != c.width || info.Height != c.height || info.HasAudio != c.audio {
				t.Errorf("info = %+v", info)
			}
		})
	}
}
