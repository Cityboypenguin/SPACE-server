package upload

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// テスト用の動画は scripts/gen_video_fixtures.sh で作る。

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "video", name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return b
}

func inspect(b []byte) (VideoInfo, error) {
	head := b
	if len(head) > videoHeadLimit {
		head = head[:videoHeadLimit]
	}
	return InspectMP4(head, int64(len(b)))
}

func TestInspectMP4_Accepts(t *testing.T) {
	cases := []struct {
		name          string
		width, height int
		audio         bool
	}{
		{"landscape.mp4", 320, 180, true},
		{"portrait.mp4", 180, 320, true},
		{"noaudio.mp4", 320, 180, false},
		{"hd720.mp4", 1280, 720, false},
		{"hd720_portrait.mp4", 720, 1280, false},
		{"d60.mp4", 64, 36, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			info, err := inspect(fixture(t, c.name))
			if err != nil {
				t.Fatalf("InspectMP4: %v", err)
			}
			if info.Width != c.width || info.Height != c.height {
				t.Errorf("size = %dx%d, want %dx%d", info.Width, info.Height, c.width, c.height)
			}
			if info.HasAudio != c.audio {
				t.Errorf("HasAudio = %v, want %v", info.HasAudio, c.audio)
			}
			if info.FPSMilli <= 0 || info.FPSMilli > videoMaxFPSMilli {
				t.Errorf("FPSMilli = %d", info.FPSMilli)
			}
		})
	}
}

// 60秒ちょうどの動画は、音声の先頭の無音で少し伸びても通る。
func TestInspectMP4_SixtySecondsExactlyIsAccepted(t *testing.T) {
	info, err := inspect(fixture(t, "d60.mp4"))
	if err != nil {
		t.Fatalf("InspectMP4: %v", err)
	}
	if info.DurationMs < 60_000 || info.DurationMs > videoMaxDurationMs {
		t.Errorf("DurationMs = %d, want 60000..%d", info.DurationMs, videoMaxDurationMs)
	}
}

func TestInspectMP4_Rejects(t *testing.T) {
	cases := []struct {
		name string
		want error
	}{
		{"d61.mp4", errVideoTooLong},
		{"fhd1080.mp4", errVideoTooLarge},
		{"fps60.mp4", errVideoFPS},
		{"hevc.mp4", errVideoCodec},
		{"mp3audio.mp4", errVideoAudioCodec},
		{"notfaststart.mp4", errVideoNotFast},
		{"fragmented.mp4", errVideoNotFast},
		{"quicktime.mov", errVideoNotMP4},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := inspect(fixture(t, c.name))
			if !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
			if !IsVideoRejection(err) {
				t.Errorf("IsVideoRejection(%v) = false", err)
			}
		})
	}
}

// 中身が画像や HTML のファイル（拡張子や Content-Type だけ mp4 を名乗る）は落ちる。
func TestInspectMP4_RejectsNonVideoBytes(t *testing.T) {
	for _, b := range [][]byte{
		[]byte("<!doctype html><script>alert(1)</script>"),
		{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0},
		{},
	} {
		if _, err := InspectMP4(b, int64(max(len(b), 1))); err == nil {
			t.Errorf("InspectMP4(%q) succeeded", b)
		}
	}
}

func TestInspectMP4_RejectsCorruption(t *testing.T) {
	good := fixture(t, "landscape.mp4")

	t.Run("truncated", func(t *testing.T) {
		b := good[:len(good)-100]
		if _, err := inspect(b); err == nil {
			t.Fatal("truncated file was accepted")
		}
	})
	t.Run("trailing garbage", func(t *testing.T) {
		b := append(bytes.Clone(good), []byte("garbage!")...)
		if _, err := inspect(b); err == nil {
			t.Fatal("file with trailing bytes was accepted")
		}
	})
	t.Run("chunk offset outside mdat", func(t *testing.T) {
		b := bytes.Clone(good)
		i := bytes.Index(b, []byte("stco"))
		if i < 0 {
			t.Fatal("no stco in fixture")
		}
		// version/flags(4) と entry_count(4) の後ろの最初のオフセットを壊す。
		binary.BigEndian.PutUint32(b[i+4+8:], uint32(len(b)+1000))
		if _, err := inspect(b); err == nil {
			t.Fatal("file with out-of-range chunk offset was accepted")
		}
	})
	t.Run("sample count mismatch", func(t *testing.T) {
		b := bytes.Clone(good)
		i := bytes.Index(b, []byte("stsz"))
		if i < 0 {
			t.Fatal("no stsz in fixture")
		}
		// sample_size が 0 のとき sample_count は version/flags(4)+sample_size(4) の後ろ。
		count := binary.BigEndian.Uint32(b[i+4+8:])
		binary.BigEndian.PutUint32(b[i+4+8:], count-1)
		if _, err := inspect(b); err == nil {
			t.Fatal("file with mismatching sample tables was accepted")
		}
	})
	t.Run("box size beyond parent", func(t *testing.T) {
		b := bytes.Clone(good)
		i := bytes.Index(b, []byte("mvhd"))
		binary.BigEndian.PutUint32(b[i-4:], 0x7fffffff)
		if _, err := inspect(b); err == nil {
			t.Fatal("file with oversized box was accepted")
		}
	})
	t.Run("missing avcC", func(t *testing.T) {
		b := bytes.Clone(good)
		i := bytes.Index(b, []byte("avcC"))
		copy(b[i:], "xxxx")
		if _, err := inspect(b); err == nil {
			t.Fatal("file without avcC was accepted")
		}
	})
	t.Run("moov beyond head", func(t *testing.T) {
		// 先頭に読んだ範囲に moov が収まっていない場合。
		_, err := InspectMP4(good[:64], int64(len(good)))
		if !errors.Is(err, errVideoNotFast) {
			t.Fatalf("err = %v, want errVideoNotFast", err)
		}
	})
}

// 表の水増しで検査の CPU とメモリを食わせられないこと。
func TestInspectMP4_RejectsInflatedTables(t *testing.T) {
	b := bytes.Clone(fixture(t, "landscape.mp4"))
	i := bytes.Index(b, []byte("stts"))
	binary.BigEndian.PutUint32(b[i+4+4:], 0x7fffffff) // entry_count
	if _, err := inspect(b); err == nil {
		t.Fatal("inflated stts was accepted")
	}
}

// PadMP4 は動画の mdat の後ろに 0 を足し、全体を size バイトにする（大きさの境界のテスト用）。
// mdat の大きさの欄も書き換えるので、構造としては正しいまま。
func padMP4(t *testing.T, b []byte, size int) []byte {
	t.Helper()
	i := bytes.Index(b, []byte("mdat"))
	if i < 4 {
		t.Fatal("no mdat")
	}
	start := i - 4
	if int(binary.BigEndian.Uint32(b[start:]))+start != len(b) {
		t.Fatal("mdat is not the last box")
	}
	out := make([]byte, size)
	copy(out, b)
	binary.BigEndian.PutUint32(out[start:], uint32(size-start))
	return out
}

func TestInspectMP4_ExactlyMaxBytesIsStructurallyValid(t *testing.T) {
	b := padMP4(t, fixture(t, "landscape.mp4"), int(VideoMaxBytes))
	if _, err := inspect(b); err != nil {
		t.Fatalf("InspectMP4: %v", err)
	}
}
