package chatstamp

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/audit"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	uploads "github.com/Cityboypenguin/SPACE-server/usecase/upload"
	"github.com/google/uuid"
)

type Service struct {
	Repo    repository.ChatStampRepository
	Uploads uploads.Acceptor
	Tx      repository.TxManager
}

func (s *Service) List(ctx context.Context, admin bool) ([]*model.ChatStamp, error) {
	if admin {
		if _, e := authz.RequireAdmin(ctx); e != nil {
			return nil, e
		}
	} else {
		if _, e := authz.RequireAuth(ctx); e != nil {
			return nil, e
		}
	}
	rows, e := s.Repo.List(ctx)
	if e != nil {
		return nil, e
	}
	out := []*model.ChatStamp{}
	for _, v := range rows {
		if admin || v.Status != "draft" {
			out = append(out, v)
		}
	}
	return out, nil
}
func (s *Service) RequireAllowed(ctx context.Context, id string, reaction bool) error {
	v, e := s.Repo.Get(ctx, id)
	if e != nil {
		return e
	}
	if v == nil || v.Status != "active" || (!reaction && !v.AllowMessage) || (reaction && !v.AllowReaction) {
		return apperr.InvalidInput("このスタンプは現在使えません")
	}
	return nil
}

// Save never accepts an arbitrary stored image key. Only a fresh, owned upload can replace it.
func (s *Service) Save(ctx context.Context, v model.ChatStamp, uploadKey *string) (_ *model.ChatStamp, err error) {
	actor, e := authz.RequireAdmin(ctx)
	if e != nil {
		return nil, e
	}
	v.Label = strings.TrimSpace(v.Label)
	if v.Label == "" || utf8.RuneCountInString(v.Label) > 50 {
		return nil, apperr.InvalidInput("名前は1〜50文字で入力してください")
	}
	if v.DisplaySize != "small" && v.DisplaySize != "standard" && v.DisplaySize != "large" {
		return nil, apperr.InvalidInput("表示サイズが不正です")
	}
	if v.Status != "draft" && v.Status != "active" && v.Status != "inactive" {
		return nil, apperr.InvalidInput("公開状態が不正です")
	}
	if v.MessageOrder < 0 || v.ReactionOrder < 0 {
		return nil, apperr.InvalidInput("表示順が不正です")
	}
	now := time.Now().Unix()
	v.UpdatedAt = now
	v.UpdatedBy = actor.ID
	var before map[string]any
	create := v.ID == ""
	if create {
		v.ImageKey = ""
		v.ID = "stamp:" + uuid.NewString()
		v.Version = 1
		v.CreatedAt = now
		v.CreatedBy = actor.ID
		if v.Status != "draft" {
			return nil, apperr.InvalidInput("まず下書きとして保存してください")
		}
	} else {
		old, e := s.Repo.Get(ctx, v.ID)
		if e != nil {
			return nil, e
		}
		if old == nil {
			return nil, apperr.InvalidInput("スタンプが見つかりません")
		}
		before = stampAuditValues(old)
		if old.Version != v.Version {
			return nil, apperr.InvalidInput("他の管理者が更新しました。一覧を再取得してください")
		}
		if old.Status != "draft" && v.Status == "draft" {
			return nil, apperr.InvalidInput("公開済みのものは利用停止にしてください")
		}
		v.ImageKey = old.ImageKey
		v.CreatedAt = old.CreatedAt
		v.CreatedBy = old.CreatedBy
	}
	session := uploads.Begin(s.Uploads)
	defer session.DiscardOnError(ctx, &err)
	if uploadKey != nil {
		if !strings.HasPrefix(*uploadKey, "staging/chat-stamps/") {
			return nil, apperr.InvalidInput("新しい画像をアップロードしてください")
		}
		v.ImageKey, err = session.Accept(ctx, uploads.ChatStamp, *uploadKey)
		if err != nil {
			return nil, err
		}
	}
	if v.Status == "active" && (v.ImageKey == "" || (!v.AllowMessage && !v.AllowReaction)) {
		return nil, apperr.InvalidInput("公開には画像と利用用途を設定してください")
	}
	if create {
		err = s.Repo.Create(ctx, &v)
	} else {
		var ok bool
		ok, err = s.Repo.Update(ctx, &v)
		if err == nil && !ok {
			err = apperr.InvalidInput("他の管理者が更新しました。一覧を再取得してください")
		}
		if err == nil {
			v.Version++
		}
	}
	if err != nil {
		return nil, err
	}
	audit.LogStampSaved(ctx, v.ID, v.Version, before, stampAuditValues(&v))
	return &v, nil
}

type OrderItem struct {
	ID      string
	Version int64
}

func (s *Service) Reorder(ctx context.Context, items []OrderItem, reaction bool) error {
	actor, e := authz.RequireAdmin(ctx)
	if e != nil {
		return e
	}
	if len(items) > 1000 {
		return apperr.InvalidInput("一度に並べ替えられる件数を超えています")
	}
	e = s.Tx.RunInTx(ctx, func(ctx context.Context) error {
		seen := map[string]bool{}
		for i, item := range items {
			if seen[item.ID] {
				return apperr.InvalidInput("IDが重複しています")
			}
			seen[item.ID] = true
			v, e := s.Repo.Get(ctx, item.ID)
			if e != nil {
				return e
			}
			if v == nil || v.Version != item.Version {
				return apperr.InvalidInput("一覧が更新されています。再取得してください")
			}
			if reaction {
				v.ReactionOrder = i
			} else {
				v.MessageOrder = i
			}
			v.UpdatedBy = actor.ID
			v.UpdatedAt = time.Now().Unix()
			ok, e := s.Repo.Update(ctx, v)
			if e != nil {
				return e
			}
			if !ok {
				return apperr.InvalidInput("一覧が更新されています。再取得してください")
			}
		}
		return nil
	})
	if e == nil {
		ids := make([]string, len(items))
		for i, item := range items {
			ids[i] = item.ID
		}
		audit.LogStampsReordered(ctx, reaction, ids)
	}
	return e
}

func stampAuditValues(v *model.ChatStamp) map[string]any {
	return map[string]any{"label": v.Label, "image_key": v.ImageKey, "status": v.Status, "display_size": v.DisplaySize, "allow_message": v.AllowMessage, "allow_reaction": v.AllowReaction, "message_order": v.MessageOrder, "reaction_order": v.ReactionOrder}
}
