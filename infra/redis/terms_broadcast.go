package redis

import (
	"context"
	"fmt"

	"github.com/Cityboypenguin/SPACE-server/usecase/terms"
	"github.com/redis/go-redis/v9"
)

// termsBroadcastKeyPrefix は「この版は配信済み」の印を置くキーの頭。
const termsBroadcastKeyPrefix = "terms:broadcast:"

// TermsBroadcastClaim は規約の配信権を台をまたいで1つに絞る terms.BroadcastOnce。
//
// SET NX で取る。最初に置けた台だけが true を受け取り、残りは false になる。
//
// 印に寿命を付けていない。付けると、寿命が切れた後の起動で同じ版をもう一度
// 配ってしまう。版は年に数回しか増えず、1件あたり数十バイトなので放置して困る
// 大きさにはならない。
//
// 注意: Redis の maxmemory-policy が allkeys-* だと、寿命の無い印も追い出され
// うる。その場合は次の起動で同じ版が1回だけ配り直される（クライアントは同意
// 状態を取り直すだけなので実害は無い）。
type TermsBroadcastClaim struct {
	client *redis.Client
	prefix string
}

var _ terms.BroadcastOnce = (*TermsBroadcastClaim)(nil)

func NewTermsBroadcastClaim(client *redis.Client, prefix string) *TermsBroadcastClaim {
	return &TermsBroadcastClaim{client: client, prefix: prefix}
}

func (c *TermsBroadcastClaim) Claim(ctx context.Context, version string) (bool, error) {
	ok, err := c.client.SetNX(ctx, c.prefix+termsBroadcastKeyPrefix+version, "1", 0).Result()
	if err != nil {
		return false, fmt.Errorf("failed to claim the terms broadcast for %q: %w", version, err)
	}
	return ok, nil
}
