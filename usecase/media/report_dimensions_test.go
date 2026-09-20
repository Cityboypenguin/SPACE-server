package media

import (
	"context"
	"errors"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

// stubMediaRepo は SetMediaDimensionsIfUnset の呼ばれ方だけを記録する。
type stubMediaRepo struct {
	repository.MediaRepository
	called bool
	width  int
	height int
	err    error
}

func (s *stubMediaRepo) SetMediaDimensionsIfUnset(_ context.Context, _ int64, width, height int) error {
	s.called = true
	s.width, s.height = width, height
	return s.err
}

func TestReportDimensionsRejectsInvalidValues(t *testing.T) {
	for _, tc := range []struct {
		name          string
		width, height int
	}{
		{"幅がゼロ", 0, 100},
		{"高さがゼロ", 100, 0},
		{"幅が負", -1, 100},
		{"高さが負", 100, -1},
		{"幅が上限超え", 65536, 100},
		{"高さが上限超え", 100, 65536},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &stubMediaRepo{}
			err := NewReportDimensionsUseCase(repo).Execute(context.Background(), 1, tc.width, tc.height)
			if !errors.Is(err, ErrInvalidDimensions) {
				t.Fatalf("err = %v, want ErrInvalidDimensions", err)
			}
			if repo.called {
				t.Error("不正な値なのにリポジトリへ書き込もうとした")
			}
		})
	}
}

func TestReportDimensionsAcceptsValidValues(t *testing.T) {
	for _, tc := range []struct {
		name          string
		width, height int
	}{
		{"通常の値", 1224, 1600},
		{"最小", 1, 1},
		{"上限ちょうど", 65535, 65535},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &stubMediaRepo{}
			if err := NewReportDimensionsUseCase(repo).Execute(context.Background(), 1, tc.width, tc.height); err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if !repo.called || repo.width != tc.width || repo.height != tc.height {
				t.Errorf("リポジトリへ %dx%d が渡らなかった (called=%v, %dx%d)",
					tc.width, tc.height, repo.called, repo.width, repo.height)
			}
		})
	}
}

func TestReportDimensionsPropagatesRepositoryError(t *testing.T) {
	wantErr := errors.New("db down")
	repo := &stubMediaRepo{err: wantErr}
	if err := NewReportDimensionsUseCase(repo).Execute(context.Background(), 1, 100, 200); !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}
