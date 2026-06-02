// infra/mysql/query_helper.go
package mysql

import (
	"context"
	"fmt"
	"strings"

	"github.com/Cityboypenguin/SPACE-server/internal/middleware"
)

// AppendBlockFilter は、対象のユーザーカラムにブロック除外 (NOT IN) 条件を安全に追加します。
// ※注意: baseQuery は WHERE 句の途中（ORDER BY や LIMIT の前）である必要があります。
func AppendBlockFilter(ctx context.Context, baseQuery string, args []interface{}, userColumn string) (string, []interface{}) {
	// 1. Contextからブロックリストを取得（ミドルウェアがセットしたもの）
	blockedIDs := middleware.GetBlockListFromContext(ctx)

	// ブロック対象がいない場合は、元のクエリと引数をそのまま返す
	if len(blockedIDs) == 0 {
		return baseQuery, args
	}

	// 2. プレースホルダー（?, ?, ...）と引数の追加
	placeholders := make([]string, len(blockedIDs))
	for i, id := range blockedIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}

	// 3. 元のクエリに結合する
	// 例: " AND user_id NOT IN (?, ?)"
	query := fmt.Sprintf("%s AND %s NOT IN (%s)", baseQuery, userColumn, strings.Join(placeholders, ","))

	return query, args
}
