package graph

import (
	"context"
	"fmt"

	"github.com/Cityboypenguin/SPACE-server/internal/audit"
	"github.com/Cityboypenguin/SPACE-server/internal/opaqueid"
)

func encodeGraphID(kind string, id int64) string {
	return opaqueid.Encode(kind, id)
}

func decodeGraphID(ctx context.Context, kind string, value string) (int64, error) {
	id, err := opaqueid.Decode(kind, value)
	if err != nil {
		audit.LogProbe(ctx, "decode_id", kind, value, err.Error())
		return 0, err
	}
	return id, nil
}

// decodeMentionUserIDs はクライアントから届いたメンション先の GraphQL ID を
// 数値IDへ変換する。
//
// リゾルバが担うのはこのデコードだけ。「どのルームでメンションが成立するか」
// （messageusecase.MentionsSupported: コミュニティのみ）「誰をメンションできるか」の
// 判断は全て messageusecase.ResolveMentionsUseCase 側にあり、それを呼ぶのは
// ChatCommands だけ。リゾルバから直接呼ぶ経路を残さないことで、別経路から
// 授業内チャットに実名メンションが通ることを防いでいる。
func decodeMentionUserIDs(ctx context.Context, mentionUserIDs []string) ([]int64, error) {
	if len(mentionUserIDs) == 0 {
		return nil, nil
	}

	ids := make([]int64, 0, len(mentionUserIDs))
	for _, encoded := range mentionUserIDs {
		id, err := decodeGraphID(ctx, "user", encoded)
		if err != nil {
			return nil, fmt.Errorf("invalid mention user id")
		}
		ids = append(ids, id)
	}
	return ids, nil
}
