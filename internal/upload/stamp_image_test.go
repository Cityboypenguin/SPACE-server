package upload

import (
	"bytes"
	"context"
	"encoding/binary"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

func TestStampSVGRejectsExecutableAndExternalContent(t *testing.T) {
	for _, fragment := range []string{
		`<script>alert(1)</script>`, `<path fill="u\72l(https://example.com/a)"/>`, `<g onload="alert(1)"/>`, `<foreignObject/>`, `<animate/>`, `<style>path{fill:red}</style>`,
		`<use href="https://example.com/a.svg#x"/>`, `<path fill="url(https://example.com/a)"/>`, `<image href="data:image/svg+xml;base64,eA=="/>`,
		`<use xmlns:xlink="http://www.w3.org/1999/xlink" xlink:href="&#104;ttps://example.com/x"/>`,
	} {
		body := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64">` + fragment + `</svg>`)
		if ValidateStampImage("image/svg+xml", body) == nil {
			t.Errorf("accepted %s", fragment)
		}
	}
	for _, body := range []string{`<!DOCTYPE svg><svg viewBox="0 0 10 10"/>`, `<svg viewBox="0 0 10 10"/><svg viewBox="0 0 10 10"/>`, `<svg viewBox="0 0 0 10"/>`, `<svg/>`, `<svg viewBox="0 0 NaN 10"/>`, `<svg viewBox="0 0 10 10">`} {
		if ValidateStampImage("image/svg+xml", []byte(body)) == nil {
			t.Errorf("accepted %s", body)
		}
	}
}
func TestStampSVGAcceptsStaticVectorAndLocalGradient(t *testing.T) {
	body := []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 256 128"><defs><linearGradient id="a"><stop offset="0" stop-color="#fff"/></linearGradient></defs><path d="M0 0 L10 10" fill="url(#a)"/></svg>`)
	if e := ValidateStampImage("image/svg+xml", body); e != nil {
		t.Fatal(e)
	}
}
func TestStampPNGRejectsAnimationOversizeAndCorruption(t *testing.T) {
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 64, 64)))
	body := b.Bytes()
	if e := ValidateStampImage("image/png", body); e != nil {
		t.Fatal(e)
	}
	// Insert an APNG animation-control chunk with a correct CRC.
	chunk := make([]byte, 20)
	binary.BigEndian.PutUint32(chunk[:4], 8)
	copy(chunk[4:8], "acTL")
	binary.BigEndian.PutUint32(chunk[16:], crc32.ChecksumIEEE(chunk[4:16]))
	animated := append(append(append([]byte{}, body[:33]...), chunk...), body[33:]...)
	for _, v := range [][]byte{animated, body[:len(body)/2], []byte(strings.Repeat("x", maxStampSourceBytes+1))} {
		if ValidateStampImage("image/png", v) == nil {
			t.Error("invalid PNG accepted")
		}
	}
	b.Reset()
	_ = png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 32769, 1)))
	if ValidateStampImage("image/png", b.Bytes()) == nil {
		t.Error("oversize PNG accepted")
	}
}

// Storage round trip: accepted PNGs are canonicalized into a new immutable key.
type stampPNGStore struct {
	*fakeStore
	saved []byte
}

func (s *stampPNGStore) PutObject(_ context.Context, key, contentType string, body []byte) error {
	s.saved = append([]byte(nil), body...)
	s.objects[key] = repository.ObjectInfo{Size: int64(len(body)), ContentType: contentType, ETag: "accepted"}
	return nil
}
func TestAcceptStampPNGCanonicalizesAndChecksOwnership(t *testing.T) {
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 16, 16)))
	body := b.Bytes()
	// A valid ancillary text chunk is accepted but must not survive normalization.
	data := []byte("note\x00metadata")
	chunk := make([]byte, len(data)+12)
	binary.BigEndian.PutUint32(chunk[:4], uint32(len(data)))
	copy(chunk[4:8], "tEXt")
	copy(chunk[8:], data)
	binary.BigEndian.PutUint32(chunk[len(chunk)-4:], crc32.ChecksumIEEE(chunk[4:len(chunk)-4]))
	body = append(append(append([]byte{}, body[:33]...), chunk...), body[33:]...)
	key, e := ChatStamp.NewStagingKey("42", "image/png")
	if e != nil {
		t.Fatal(e)
	}
	fake := newFakeStore(repository.ObjectInfo{Size: int64(len(body)), ContentType: "image/png", ETag: "checked"}, key)
	fake.headBytes = body
	store := &stampPNGStore{fakeStore: fake}
	if _, e = Accept(context.Background(), store, key, ChatStamp, "43"); e == nil {
		t.Fatal("another administrator's upload accepted")
	}
	accepted, e := Accept(context.Background(), store, key, ChatStamp, "42")
	if e != nil {
		t.Fatal(e)
	}
	if accepted == key || strings.HasPrefix(accepted, "staging/") {
		t.Fatal("mutable staging key returned")
	}
	if bytes.Contains(store.saved, []byte("metadata")) {
		t.Fatal("ancillary data survived")
	}
	if _, e = png.Decode(bytes.NewReader(store.saved)); e != nil {
		t.Fatal(e)
	}
	if _, ok := store.objects[key]; ok {
		t.Fatal("staging not removed")
	}
}

func TestStampPNGShrinksWithoutStretchingOrEnlarging(t *testing.T) {
	for _, tc := range []struct{ w, h, wantW, wantH int }{{2048, 1024, 1024, 512}, {1024, 2048, 512, 1024}, {2048, 2048, 1024, 1024}, {80, 40, 80, 40}} {
		var buf bytes.Buffer
		src := image.NewNRGBA(image.Rect(0, 0, tc.w, tc.h))
		src.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 128})
		if err := png.Encode(&buf, src); err != nil {
			t.Fatal(err)
		}
		result, err := normalizeStampPNG(buf.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := png.DecodeConfig(bytes.NewReader(result))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Width != tc.wantW || cfg.Height != tc.wantH {
			t.Fatalf("%dx%d => %dx%d, want %dx%d", tc.w, tc.h, cfg.Width, cfg.Height, tc.wantW, tc.wantH)
		}
		if len(result) > maxStampStoredBytes {
			t.Fatal("stored image exceeds file size limit")
		}
		decoded, err := png.Decode(bytes.NewReader(result))
		if err != nil {
			t.Fatal(err)
		}
		r, g, b, a := decoded.At(0, 0).RGBA()
		if a == 0 || a == 65535 || r == 0 || g != 0 || b != 0 {
			t.Fatalf("transparency/color lost: %d,%d,%d,%d", r, g, b, a)
		}
	}
}
func TestStampSVGShrinksViewportAndKeepsGeometry(t *testing.T) {
	for _, tc := range []struct{ viewBox, w, h string }{{"0 0 4096 2048", "1024", "512"}, {"0 0 2048 4096", "512", "1024"}, {"0 0 80 40", "80", "40"}} {
		original := []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg" width="9999" height='9999' preserveAspectRatio="none" viewBox="` + tc.viewBox + `"><path d="M0 0 L4096 2048"/></svg>`)
		result, err := normalizeStampSVG(original)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(result, []byte(`viewBox="`+tc.viewBox+`"`)) || !bytes.Contains(result, []byte(`width="`+tc.w+`" height="`+tc.h+`"`)) || !bytes.Contains(result, []byte(`preserveAspectRatio="xMidYMid meet"`)) {
			t.Fatalf("wrong dimensions: %s", result)
		}
		if !bytes.Contains(result, []byte(`d="M0 0 L4096 2048"`)) {
			t.Fatal("vector geometry changed")
		}
		if err := ValidateStampImage("image/svg+xml", result); err != nil {
			t.Fatal(err)
		}
	}
}

func TestStampSVGResizeLeavesAttributeValuesIntact(t *testing.T) {
	source := []byte(`<svg xmlns="http://www.w3.org/2000/svg" id="has width='999' inside" viewBox="0 0 2048 1024" width="2048" height="1024"/>`)
	result, err := normalizeStampSVG(source)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(result, []byte(`id="has width='999' inside"`)) {
		t.Fatalf("attribute value changed: %s", result)
	}
	if err := ValidateStampImage("image/svg+xml", result); err != nil {
		t.Fatal(err)
	}
}
func TestAcceptLargeSVGStoresNormalizedViewport(t *testing.T) {
	body := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 4096 2048"><path d="M0 0 L4096 2048"/></svg>`)
	key, err := ChatStamp.NewStagingKey("42", "image/svg+xml")
	if err != nil {
		t.Fatal(err)
	}
	fake := newFakeStore(repository.ObjectInfo{Size: int64(len(body)), ContentType: "image/svg+xml", ETag: "checked"}, key)
	fake.headBytes = body
	store := &stampPNGStore{fakeStore: fake}
	accepted, err := Accept(context.Background(), store, key, ChatStamp, "42")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(store.saved, []byte(`width="1024" height="512"`)) {
		t.Fatalf("not resized: %s", store.saved)
	}
	if store.objects[accepted].ContentType != "image/svg+xml" {
		t.Fatal("SVG content type lost")
	}
}
