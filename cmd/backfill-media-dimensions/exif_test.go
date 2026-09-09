package main

import (
	"encoding/binary"
	"testing"
)

// buildJPEGWithOrientation は Orientation タグだけを持つ EXIF APP1 を付けた
// 最小の JPEG バイト列を組み立てる。bigEndian で TIFF のバイトオーダーも切り替える。
func buildJPEGWithOrientation(orientation int, bigEndian bool) []byte {
	var order binary.ByteOrder = binary.LittleEndian
	head := []byte("II")
	if bigEndian {
		order = binary.BigEndian
		head = []byte("MM")
	}

	tiff := append([]byte{}, head...)
	tiff = append(tiff, make([]byte, 2)...)
	order.PutUint16(tiff[2:4], 42)
	tiff = append(tiff, make([]byte, 4)...)
	order.PutUint32(tiff[4:8], 8)

	entry := make([]byte, 12)
	order.PutUint16(entry[0:2], 0x0112) // Orientation
	order.PutUint16(entry[2:4], 3)      // SHORT
	order.PutUint32(entry[4:8], 1)      // count
	order.PutUint16(entry[8:10], uint16(orientation))

	ifd := make([]byte, 2)
	order.PutUint16(ifd, 1)
	ifd = append(ifd, entry...)
	ifd = append(ifd, make([]byte, 4)...) // 次の IFD なし

	payload := append([]byte("Exif\x00\x00"), append(tiff, ifd...)...)
	app1 := []byte{0xFF, 0xE1}
	length := make([]byte, 2)
	binary.BigEndian.PutUint16(length, uint16(len(payload)+2))
	app1 = append(app1, length...)
	app1 = append(app1, payload...)

	// SOI + APP1 + SOS（以降は読まれない）
	out := []byte{0xFF, 0xD8}
	out = append(out, app1...)
	out = append(out, 0xFF, 0xDA, 0x00, 0x02)
	return out
}

func TestJPEGExifOrientation(t *testing.T) {
	for _, tc := range []struct {
		name      string
		bigEndian bool
		want      int
	}{
		{"リトルエンディアン", false, 6},
		{"ビッグエンディアン", true, 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := jpegExifOrientation(buildJPEGWithOrientation(tc.want, tc.bigEndian))
			if got != tc.want {
				t.Errorf("orientation = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestJPEGExifOrientationMissing(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"SOI のみ", []byte{0xFF, 0xD8}},
		{"JPEG ではない", []byte("not a jpeg at all")},
		{"空", nil},
		{"APP1 が途中で切れている", []byte{0xFF, 0xD8, 0xFF, 0xE1, 0x00, 0x40, 'E', 'x'}},
		{"EXIF ではない APP1", append([]byte{0xFF, 0xD8, 0xFF, 0xE1, 0x00, 0x08}, []byte("XMP\x00\x00\x00")...)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := jpegExifOrientation(tc.data); got != 0 {
				t.Errorf("orientation = %d, want 0", got)
			}
		})
	}
}

func TestExifOrientationSwapsAxes(t *testing.T) {
	// 1〜4 は回転を伴わない（あるいは反転のみ）、5〜8 は 90 度系で縦横が入れ替わる。
	for orientation := 0; orientation <= 9; orientation++ {
		want := orientation >= 5 && orientation <= 8
		if got := exifOrientationSwapsAxes(orientation); got != want {
			t.Errorf("orientation %d: swaps = %v, want %v", orientation, got, want)
		}
	}
}
