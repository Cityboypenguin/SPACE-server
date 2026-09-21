package chat

import (
	"context"
	"errors"
	"fmt"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/usecase/block"
	courseusecase "github.com/Cityboypenguin/SPACE-server/usecase/course"
	roomusecase "github.com/Cityboypenguin/SPACE-server/usecase/room"
)

// AccessPolicy はチャットの権限判定。閲覧・書き込み・編集削除の可否をここだけで決める。
//
// 送信・編集・削除・一覧・既読の各サービスはこれを依存として受け取る。判定の実体が
// 1つであることが要で、サービスごとに同じ判定を書き直すと必ずどこかで食い違う
// （統合前は実際にそうなっていた。各メソッドのコメント参照）。
//
// 非公開メソッドを混ぜてあるのは、実装を chat パッケージの外へ出せなくするため。
// 「判定つきの入口」を外から差し替えられると、この型が権限判定の単一の出どころで
// あるという前提が崩れる。
type AccessPolicy interface {
	// EnsureReadAccess は roomID を閲覧してよいかを判定し、ルームを返す。
	// メッセージだけでなく質問・回答・投票のクエリ／サブスクリプションも
	// この1つを通す（判定の重複を無くすため）。
	EnsureReadAccess(ctx context.Context, roomID int64) (*model.Room, error)
	// EnsureWriteAccess は roomID へ書き込んでよいかを判定し、ルームを返す。
	EnsureWriteAccess(ctx context.Context, roomID int64) (*model.Room, error)

	// ensureWriteAccessFor は書き込み判定の本体。判定のついでに引いたメンバー一覧を
	// 送信サービスへ持ち回すため、EnsureWriteAccess とは別に用意している。
	ensureWriteAccessFor(ctx context.Context, claims *auth.Claims, roomID int64) (*writeAccess, error)
	// ensureMutateAccess は既存メッセージの編集・削除を許してよいルームかの判定。
	ensureMutateAccess(ctx context.Context, claims *auth.Claims, room *model.Room) error
}

// AccessPolicyDeps は権限判定に要るものだけ。ルーム・メンバー・授業の学期履修・
// ブロック関係の4つで判定は閉じている。
type AccessPolicyDeps struct {
	GetRoom roomusecase.GetRoomUseCase
	// GetRoomMemberIDs は新規送信の判定で使う。判定そのものに加えて、送信後の
	// 配信の宛先としてメンバー一覧をそのまま持ち回すため、ここだけは全員ぶんが要る。
	GetRoomMemberIDs roomusecase.GetUserIDsByRoomIDUseCase
	// IsRoomMember は閲覧・編集・削除の判定で使う。在籍の有無しか要らない経路を
	// メンバー一覧で代用すると、判定のたびにルームの人数ぶんの行が戻ってくる。
	IsRoomMember roomusecase.IsRoomMemberUseCase

	// CheckRoomWritable は授業ルームの学期・履修判定（非授業ルームは素通し）。
	CheckRoomWritable  courseusecase.CheckRoomWritableUseCase
	CheckBlockRelation block.CheckBlockRelationUseCase
}

var _ AccessPolicy = &accessPolicy{}

type accessPolicy struct {
	deps AccessPolicyDeps
}

func NewAccessPolicy(deps AccessPolicyDeps) AccessPolicy {
	return &accessPolicy{deps: deps}
}

// EnsureReadAccess は roomID を閲覧してよいかを判定する。
//
// 判定規則:
//   - 授業内チャット: 認証済みなら誰でも閲覧可（F-04 全授業公開。投稿者は匿名表示）。
//   - それ以外: room_users の membership が要る。
//   - 管理者: DM 以外なら非メンバーでも閲覧可。
//
// 管理者の扱いについて。統合前は messages クエリだけが isAdminRole を見ていて、
// messageAdded などのサブスクリプションは見ていなかった。同じ部屋なのに
// 「一覧は出るが購読は繋がらない」という食い違いが出るので、緩い側（管理者は
// 閲覧可）へ揃えた。ただし DM は例外で、統合前に管理者向けの DM 閲覧機能が
// 無かった以上ここで広げる理由が無く、通報対応は通報時のスナップショットで
// 足りるため、管理者でも membership を要求する（統合前の messages クエリより
// 厳しくなるのはこの1点だけ）。
func (p *accessPolicy) EnsureReadAccess(ctx context.Context, roomID int64) (*model.Room, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	room, err := p.deps.GetRoom.Execute(ctx, roomID)
	if err != nil {
		// GetRoomUseCase は存在しないルームに "room not found: <id>" を返す。
		// クライアントの not-found 判定がそのまま効くよう、包まずに返す。
		return nil, err
	}
	if room.Type == model.RoomTypeCourse {
		return room, nil
	}

	if err := p.ensureRoomParticipation(ctx, claims, room); err != nil {
		return nil, err
	}
	return room, nil
}

// ensureRoomParticipation は非授業ルームの「その部屋に関わってよい人か」の判定。
//
// 閲覧（EnsureReadAccess）と既存メッセージの編集・削除（ensureMutateAccess）で
// 同じ関数を使う。別々に書くと「閲覧は禁じているのに編集はできる」という食い違いが
// 生まれ、実際そうなっていた（退出後も自分のメッセージを編集・削除できた）。
//
// 管理者の DM 除外はここ1箇所で守る。非参加の DM は管理者でも読めず、書き換えも
// 削除もできない。法務上の要件なので、呼び出し側の都合で緩めないこと。
func (p *accessPolicy) ensureRoomParticipation(ctx context.Context, claims *auth.Claims, room *model.Room) error {
	// 管理者判定を先に見る。DM 以外なら在籍を問わず通るので、そこまで確かめてから
	// DB を引くのは無駄になる。判定の結果は入れ替えても変わらない。
	if room.Type != model.RoomTypeDM && authz.IsAdminRole(claims.Role) {
		return nil
	}
	// ここで要るのは「自分が入っているか」だけ。メンバー一覧を引いて探すと、
	// 閲覧・購読・編集・削除のたびにルームの人数ぶんの行が戻る。
	isMember, err := p.deps.IsRoomMember.Execute(ctx, room.ID, claims.ID)
	if err != nil {
		return fmt.Errorf("failed to verify room membership")
	}
	if isMember {
		return nil
	}
	return errors.New("forbidden: not a member of this room")
}

// ensureMutateAccess は「既にあるメッセージを、いま、このルームで操作してよいか」の判定。
//
// 所有権（本人か／管理者か／コミュニティのオーナーか）とは別の軸で、編集・削除は
// 両方を通って初めて許される。所有権しか見ていなかったため、コミュニティを退出・
// キックされた利用者が既知の message ID で編集・削除を続けられていた。
//
// 経路ごとにどの判定が効くか:
//
//	ルーム種別   | 一般利用者                        | 管理者
//	------------|----------------------------------|------------------------------
//	授業        | CheckRoomWritable（現学期＋履修）  | 素通し（通報対応のため従来どおり）
//	コミュニティ | membership 必須                   | 非メンバーでも可
//	DM          | membership 必須                   | membership 必須（非参加のDMは不可）
//
// 授業ルームで membership を見ないのは、授業内チャットが room_users を使わない
// 設計（誰でも閲覧でき匿名で表示する）だから。代わりに「現学期かつ履修中か」を
// CheckRoomWritable が見る。送信時 (ensureWriteAccessFor) と同じ判定なので、
// 送れる状態でなければ直せもしない、で揃う。
//
// 送信 (ensureWriteAccessFor) と違ってブロック判定は入れない。ブロックは「相手に
// 新しく話しかけさせない」ための設定であって、既に送ってしまった自分の発言を消す
// 手段まで奪う理由が無いため。
func (p *accessPolicy) ensureMutateAccess(ctx context.Context, claims *auth.Claims, room *model.Room) error {
	if room.Type == model.RoomTypeCourse {
		if authz.IsAdminRole(claims.Role) {
			return nil
		}
		return p.deps.CheckRoomWritable.Execute(ctx, room.ID)
	}
	return p.ensureRoomParticipation(ctx, claims, room)
}

// EnsureWriteAccess は roomID へ書き込んでよいかを判定する。
// 判定の中身は ensureWriteAccessFor と同じで、こちらはルームだけを返す薄い口。
func (p *accessPolicy) EnsureWriteAccess(ctx context.Context, roomID int64) (*model.Room, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	access, err := p.ensureWriteAccessFor(ctx, claims, roomID)
	if err != nil {
		return nil, err
	}
	return access.Room, nil
}

// writeAccess は書き込み判定の結果。
type writeAccess struct {
	Room *model.Room
	// MemberIDs は非授業ルームのメンバー。判定のついでに引いた結果を、送信後の配信
	// （room_changed の宛先・DM 通知）でもう一度引き直さずに済むよう持ち回す。
	// 授業ルームでは nil。
	MemberIDs []int64
}

// ensureWriteAccessFor は書き込み権限の唯一の判定。
//
//   - 授業内チャット: room_users を使わず、現在の学期と一致するか（アーカイブ
//     されていないか）と時間割に登録済みかを CheckRoomWritableUseCase が見る。
//   - それ以外: membership が必須。DM だけはブロック関係があれば拒否。
func (p *accessPolicy) ensureWriteAccessFor(ctx context.Context, claims *auth.Claims, roomID int64) (*writeAccess, error) {
	room, err := p.deps.GetRoom.Execute(ctx, roomID)
	if err != nil {
		return nil, fmt.Errorf("failed to get room")
	}

	if room.Type == model.RoomTypeCourse {
		if err := p.deps.CheckRoomWritable.Execute(ctx, roomID); err != nil {
			return nil, err
		}
		return &writeAccess{Room: room}, nil
	}

	// ここは ensureRoomParticipation を使わない。新規送信だけは管理者にも
	// membership を要求する（非メンバーのコミュニティへ管理者名義で書き込む機能は
	// 元から無く、閲覧・モデレーションのために広げた管理者権限を「発言」まで
	// 広げる理由が無いため）。閲覧・編集・削除とは意図的に別の規則。
	memberIDs, err := p.deps.GetRoomMemberIDs.Execute(ctx, roomID)
	if err != nil {
		return nil, fmt.Errorf("failed to verify room membership")
	}
	if !containsInt64(memberIDs, claims.ID) {
		return nil, errors.New("forbidden: not a member of this room")
	}

	// ブロックで送信を止めるのは DM だけ。以前はここが「メンバーが2人なら DM」と
	// 人数から推測していたが、model.RoomTypeDM という型がある以上その推測は要らず、
	// たまたま2人しか居ないコミュニティでも DM と見なされてしまう。コミュニティは
	// 参加者同士にブロック関係があっても場そのものは使えるのが正しい（ブロックは
	// 1対1の会話を止める機能であって、共同の場から締め出す機能ではない）ので、
	// 人数ではなくルーム種別で判定する。
	if room.Type == model.RoomTypeDM {
		if partnerID, ok := soleOtherMember(memberIDs, claims.ID); ok {
			isBlocked, err := p.deps.CheckBlockRelation.Execute(ctx, partnerID)
			if err != nil {
				return nil, fmt.Errorf("failed to check block status")
			}
			if isBlocked {
				return nil, errors.New("ブロック設定によりメッセージを送信できません")
			}
		}
		// 相手が1人に決まらない DM（相手が退会して room_users の行が消え、自分しか
		// 残っていない等）はブロック判定を飛ばす。ブロック関係は「相手が誰か」が
		// 決まらないと引けず、ここで拒否側に倒すと「ブロックしていないのに送れない」
		// 説明のつかない状態になるため。送り先が居ないだけなので、通しても害はない。
	}

	return &writeAccess{Room: room, MemberIDs: memberIDs}, nil
}
