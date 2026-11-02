// Package reportedmedia は通報の写しが指している添付を、利用者の届かない置き場へ残す。
//
// 通報された時点の内容は、投稿者が退会したあとも対応する側が確認できる必要がある
// （model.ReportedContent 参照）。添付のファイルも同じ扱いにする。
package reportedmedia

import (
	"context"
	"errors"
	"strconv"

	"github.com/Cityboypenguin/SPACE-server/internal/upload"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// Retainer は通報の写しが指している添付を残す。
type Retainer struct {
	reports repository.ReportedMediaRepository
	public  repository.StorageRepository
	// private はメッセージの添付の置き場（非公開）。残すファイルもここへ置く。
	private repository.StorageRepository
}

func New(reports repository.ReportedMediaRepository, public, private repository.StorageRepository) *Retainer {
	return &Retainer{reports: reports, public: public, private: private}
}

// Retain は通報の写しが指している添付を非公開の置き場へ写し、写しをそちらへ付け替える。
// 通報されていない添付には何もしない。元のファイルは消さない（呼び出し側の仕事）。
func (r *Retainer) Retain(ctx context.Context, media []repository.ReportableMedia) error {
	for _, m := range media {
		targetID := strconv.FormatInt(m.TargetID, 10)
		reported, err := r.reports.HasReportedMedia(ctx, m.TargetType, targetID, m.StorageKey)
		if err != nil {
			return err
		}
		if !reported {
			continue
		}
		removedKey := upload.RemovedKeyFor(m.MediaID, m.StorageKey)
		err = CopyObject(ctx, r.storeFor(m), m.StorageKey, r.private, removedKey, m.ContentType)
		if errors.Is(err, repository.ErrObjectNotFound) {
			// 実体がもう無ければ残せない。写しの参照はそのままにして先へ進める。
			continue
		}
		if err != nil {
			return err
		}
		if err := r.reports.RepointReportedMedia(ctx, m.TargetType, targetID, m.StorageKey, removedKey); err != nil {
			return err
		}
	}
	return nil
}

// storeFor はファイルのある置き場を返す。メッセージの添付は非公開の置き場にあり、分離前の
// 添付だけが公開側に残っている（graph の messageMediaURL と同じ見分け方）。
func (r *Retainer) storeFor(m repository.ReportableMedia) repository.StorageRepository {
	if m.TargetType != model.TargetMessage {
		return r.public
	}
	if kind, ok := upload.KindForObjectKey(m.StorageKey); ok && !kind.Private {
		return r.public
	}
	return r.private
}

// CopyObject は置き場をまたいで写す。添付は上限が小さいのでメモリに載せて写す。
// 元が無ければ repository.ErrObjectNotFound を返す。
func CopyObject(ctx context.Context, from repository.StorageRepository, fromKey string, to repository.StorageRepository, toKey, contentType string) error {
	info, err := from.StatObject(ctx, fromKey)
	if err != nil {
		return err
	}
	body, _, err := from.ObjectHead(ctx, fromKey, max(info.Size, 1))
	if err != nil {
		return err
	}
	return to.PutObject(ctx, toKey, contentType, body)
}
