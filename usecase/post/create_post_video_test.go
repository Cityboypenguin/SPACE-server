package post

import (
	"context"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
)

func asUser(id int64) context.Context {
	return auth.WithClaims(context.Background(), &auth.Claims{ID: id})
}

// 動画と画像は同じ投稿に付けられない。ファイルに触る前に断る。
func TestCreatePost_RejectsVideoWithImages(t *testing.T) {
	uc := &CreatePostInteractor{videos: nil}
	_, err := uc.Execute(asUser(1), model.CreatePostParam{
		Video: &model.VideoInput{StorageKey: "staging/videos/1/a.mp4", ThumbnailKey: "staging/video-thumbnails/1/a.jpg"},
	}, []model.MediaInput{{StorageKey: "staging/media/1/a.png", ContentType: "image/png"}})
	if apperr.CodeOf(err) != apperr.CodeInvalidInput {
		t.Fatalf("err = %v, want INVALID_INPUT", err)
	}
}

// 動画の機能を配線していなければ、動画付きの投稿は受け付けない（黙って動画を捨てない）。
func TestCreatePost_RejectsVideoWhenNotConfigured(t *testing.T) {
	uc := &CreatePostInteractor{}
	_, err := uc.Execute(asUser(1), model.CreatePostParam{
		Video: &model.VideoInput{StorageKey: "staging/videos/1/a.mp4", ThumbnailKey: "staging/video-thumbnails/1/a.jpg"},
	}, nil)
	if apperr.CodeOf(err) != apperr.CodeInvalidInput {
		t.Fatalf("err = %v, want INVALID_INPUT", err)
	}
}
