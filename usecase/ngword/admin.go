package ngword

import (
	"context"
	"errors"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// StatsTopN は集計に出す件数（言葉ごと・人ごと）。
const StatsTopN = 20

// AdminService は管理画面でのNGワードの登録・削除・集計。
type AdminService struct {
	repo  repository.NGWordRepository
	cache Invalidator
}

func NewAdminService(repo repository.NGWordRepository, cache Invalidator) *AdminService {
	return &AdminService{repo: repo, cache: cache}
}

func (s *AdminService) List(ctx context.Context) ([]*model.NGWord, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return nil, err
	}
	return s.repo.ListNGWords(ctx)
}

func (s *AdminService) Create(ctx context.Context, pattern string, matchType model.NGWordMatchType) (*model.NGWord, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return nil, err
	}
	w, err := model.NewNGWord(pattern, matchType, time.Now())
	if err != nil {
		return nil, err
	}
	if err := s.repo.CreateNGWord(ctx, w, model.MaxNGWords); err != nil {
		if errors.Is(err, repository.ErrDuplicateKey) {
			return nil, apperr.InvalidInput("このNGワードはすでに登録されています")
		}
		if errors.Is(err, repository.ErrNGWordLimit) {
			return nil, apperr.InvalidInput("登録できるNGワードの数の上限に達しています")
		}
		return nil, err
	}
	s.cache.Invalidate()
	return w, nil
}

func (s *AdminService) Delete(ctx context.Context, id int64) (bool, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return false, err
	}
	deleted, err := s.repo.DeleteNGWord(ctx, id)
	if err != nil {
		return false, err
	}
	s.cache.Invalidate()
	return deleted, nil
}

func (s *AdminService) Stats(ctx context.Context) (*model.NGWordHitStats, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return nil, err
	}
	return s.repo.NGWordHitStats(ctx, StatsTopN)
}
