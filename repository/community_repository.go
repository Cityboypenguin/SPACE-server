package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

// UpdateCommunityAvatarParam はコミュニティアバター更新時に渡すメディア情報。
// コミュニティ更新とメディア挿入を同一トランザクションで処理するために使用する。
type UpdateCommunityAvatarParam struct {
	StorageKey     string
	UploaderUserID int64
	ContentType    string
}

// CommunitySortField は管理画面のコミュニティ一覧の並び替えキー。
type CommunitySortField string

const (
	CommunitySortCreatedAt   CommunitySortField = "created_at"
	CommunitySortName        CommunitySortField = "name"
	CommunitySortMemberCount CommunitySortField = "member_count"
)

// AdminCommunityFilter は管理画面のコミュニティ一覧の検索・並び替え条件。
//
// 以前の管理画面は検索のたびに communities(limit: 1000) で全件を持ち帰り、
// ブラウザ側で絞り込み・ページ分割していた（さらに1件ごとにメンバー一覧を
// 引いていた）。件数が増えると1回の検索で数千リクエストになるうえ、1000件を
// 超えた分は検索にも出てこない。絞り込み・並び替え・ページングはすべて DB 側で行う。
type AdminCommunityFilter struct {
	// Keyword が空でなければ、名前・説明の部分一致で絞り込む。
	Keyword string
	// SortBy が空なら CommunitySortCreatedAt。
	SortBy CommunitySortField
	// Desc が true なら降順。
	Desc bool
}

type CommunityRepository interface {
	// SaveCommunityWithRoom は Room・RoomUser・Community を単一トランザクションで作成する。
	// いずれかのステップで失敗した場合はロールバックし、孤立レコードを残さない。
	SaveCommunityWithRoom(ctx context.Context, name, description string, avatar *UpdateCommunityAvatarParam, creatorUserID int64) (*model.Community, error)
	GetCommunityByID(ctx context.Context, id int64) (*model.Community, error)
	SearchCommunities(ctx context.Context, name string, userID int64, q PageQuery) ([]*model.Community, int, error)
	// UpdateCommunity はコミュニティ情報を更新する。avatar が nil でない場合はメディアレコードの
	// 作成も同一トランザクション内で行い、孤立レコードが生じないことを保証する。
	UpdateCommunity(ctx context.Context, c *model.Community, avatar *UpdateCommunityAvatarParam) error
	ListCommunitiesByUserID(ctx context.Context, userID int64, q PageQuery) ([]*model.Community, int, error)
	ListAllCommunities(ctx context.Context, q PageQuery) ([]*model.Community, int, error)
	// AdminListCommunities は管理画面の一覧。並び替えは全件に対して行ってから窓を切る。
	AdminListCommunities(ctx context.Context, f AdminCommunityFilter, q PageQuery) ([]*model.Community, int, error)
	FindRandom(ctx context.Context, userID int64, limit int) ([]*model.Community, error)
}
