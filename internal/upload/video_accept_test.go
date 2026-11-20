package upload

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

func videoStore(t *testing.T, body []byte, declaredType string, size int64) (*fakeStore, string) {
	t.Helper()
	key, err := Video.NewStagingKey("42", "video/mp4")
	if err != nil {
		t.Fatal(err)
	}
	store := newFakeStore(repository.ObjectInfo{Size: size, ContentType: declaredType, ETag: "etag-v"}, key)
	store.headBytes = body
	return store, key
}

func TestAcceptVideo_AcceptsAndReportsProbe(t *testing.T) {
	body := fixture(t, "landscape.mp4")
	store, key := videoStore(t, body, "video/mp4", int64(len(body)))

	final, probe, err := AcceptVideo(context.Background(), store, key, "42")
	if err != nil {
		t.Fatalf("AcceptVideo: %v", err)
	}
	if !strings.HasPrefix(final, "videos/42/") || !strings.HasSuffix(final, ".mp4") {
		t.Errorf("final key = %q", final)
	}
	if probe.SizeBytes != int64(len(body)) || probe.Width != 320 || probe.Height != 180 || !probe.HasAudio {
		t.Errorf("probe = %+v", probe)
	}
	if store.copiedTypes[final] != "video/mp4" {
		t.Errorf("copied content type = %q", store.copiedTypes[final])
	}
	if _, ok := store.objects[key]; ok {
		t.Error("staging object was not removed")
	}
}

// 25,000,000 バイトちょうどは通り、1バイト超えると中身を読む前に落ちて実体も消える。
func TestAcceptVideo_SizeBoundary(t *testing.T) {
	exact := padMP4(t, fixture(t, "landscape.mp4"), int(VideoMaxBytes))
	store, key := videoStore(t, exact, "video/mp4", VideoMaxBytes)
	if _, _, err := AcceptVideo(context.Background(), store, key, "42"); err != nil {
		t.Fatalf("25,000,000 bytes: %v", err)
	}

	store, key = videoStore(t, exact, "video/mp4", VideoMaxBytes+1)
	_, _, err := AcceptVideo(context.Background(), store, key, "42")
	if err == nil || !strings.Contains(err.Error(), "25MB") {
		t.Fatalf("25,000,001 bytes: err = %v, want size error", err)
	}
	if len(store.copied) != 0 {
		t.Error("oversized video was copied")
	}
	if _, ok := store.objects[key]; ok {
		t.Error("oversized staging object was kept")
	}
}

// Content-Type を video/mp4 と名乗っても、中身が動画でなければ落ちる。
func TestAcceptVideo_RejectsSpoofedContent(t *testing.T) {
	body := []byte("<!doctype html><script>alert(1)</script>")
	store, key := videoStore(t, body, "video/mp4", int64(len(body)))
	if _, _, err := AcceptVideo(context.Background(), store, key, "42"); err == nil {
		t.Fatal("HTML disguised as mp4 was accepted")
	}
	if len(store.copied) != 0 {
		t.Error("spoofed file was copied")
	}
}

// 中身は正しい mp4 でも、名乗りが別の種別なら落ちる（許可リストの段）。
func TestAcceptVideo_RejectsWrongDeclaredType(t *testing.T) {
	body := fixture(t, "landscape.mp4")
	store, key := videoStore(t, body, "video/quicktime", int64(len(body)))
	if _, _, err := AcceptVideo(context.Background(), store, key, "42"); err == nil {
		t.Fatal("wrong declared type was accepted")
	}
}

func TestAcceptVideo_RejectsOutOfSpecVideos(t *testing.T) {
	for _, name := range []string{"d61.mp4", "fhd1080.mp4", "fps60.mp4", "hevc.mp4", "notfaststart.mp4"} {
		t.Run(name, func(t *testing.T) {
			body := fixture(t, name)
			store, key := videoStore(t, body, "video/mp4", int64(len(body)))
			_, _, err := AcceptVideo(context.Background(), store, key, "42")
			if !IsVideoRejection(err) {
				t.Fatalf("err = %v, want a user-facing rejection", err)
			}
			if _, ok := store.objects[key]; ok {
				t.Error("rejected staging object was kept")
			}
		})
	}
}

// 他人のキー・受け入れ済みのキー・別種別のキーはストレージに触る前に落ちる。
func TestAcceptVideo_RejectsForeignAndNonStagingKeys(t *testing.T) {
	body := fixture(t, "landscape.mp4")
	store, key := videoStore(t, body, "video/mp4", int64(len(body)))

	if _, _, err := AcceptVideo(context.Background(), store, key, "7"); err == nil {
		t.Error("someone else's key was accepted")
	}
	accepted := strings.TrimPrefix(key, StagingPrefix+"/")
	store.objects[accepted] = store.objects[key]
	if _, _, err := AcceptVideo(context.Background(), store, accepted, "42"); err == nil {
		t.Error("non-staging key was accepted")
	}
	img := mediaKey(t)
	if _, _, err := AcceptVideo(context.Background(), store, img, "42"); err == nil {
		t.Error("image key was accepted as a video")
	}
	if len(store.deleted) != 0 || len(store.copied) != 0 {
		t.Errorf("storage was touched: deleted=%v copied=%v", store.deleted, store.copied)
	}
}

// 測ってから写すまでの間に差し替えられたら公開しない。
func TestAcceptVideo_RejectsReplacedBetweenStatAndCopy(t *testing.T) {
	body := fixture(t, "landscape.mp4")
	store, key := videoStore(t, body, "video/mp4", int64(len(body)))
	store.copyErr = repository.ErrObjectChanged
	_, _, err := AcceptVideo(context.Background(), store, key, "42")
	if !errors.Is(err, errNotUploaded) {
		t.Fatalf("err = %v, want errNotUploaded", err)
	}
}

// 汎用の Accept からは動画を受け入れない（実測値を記録しそこなうため）。
func TestAccept_RefusesVideoKind(t *testing.T) {
	body := fixture(t, "landscape.mp4")
	store, key := videoStore(t, body, "video/mp4", int64(len(body)))
	if _, err := Accept(context.Background(), store, key, Video, "42"); err == nil {
		t.Fatal("Accept took a video")
	}
}

func TestAcceptor_RoutesVideosToTheVideoStore(t *testing.T) {
	public := newFakeStore(okImage())
	key, _ := VideoThumbnail.NewStagingKey("42", "image/jpeg")
	video := newFakeStore(repository.ObjectInfo{Size: 100, ContentType: "image/jpeg", ETag: "e"}, key)
	a := NewAcceptor(public, nil, video)
	ctx := ctxAs(42)
	final, err := a.Accept(ctx, "video-thumbnails", key)
	if err != nil {
		t.Fatalf("Accept thumbnail: %v", err)
	}
	if _, ok := video.objects[final]; !ok {
		t.Error("thumbnail was not stored in the video store")
	}
	if len(public.copied) != 0 {
		t.Error("public store was used for a video thumbnail")
	}

	noVideo := NewAcceptor(public, nil, nil)
	if _, err := noVideo.Accept(ctx, "video-thumbnails", key); err == nil {
		t.Error("thumbnail accepted without a video store")
	}
}
