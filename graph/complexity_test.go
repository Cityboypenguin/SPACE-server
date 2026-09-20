package graph

import (
	"context"
	"testing"

	"github.com/99designs/gqlgen/complexity"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/validator"
)

// 複雑度の上限（cmd/server/main.go の FixedComplexityLimit）を、実際に流れている
// クエリと、防ぎたいクエリの両方から挟んで決める。
//
// 上限は1つの数字なので、片方だけ見て決めると必ずどちらかを壊す。高すぎれば
// 入れ子の展開が素通りし、低すぎれば普通の画面が「クエリが大きすぎます」で落ちる。
// 落ちる方は本番で初めて分かるので、ここで両側から確かめる。
// calcComplexity はクエリ文字列の複雑度を、本番と同じ設定で数える。
func calcComplexity(t *testing.T, query string, vars map[string]any) int {
	t.Helper()

	es := NewWeightedExecutableSchema(Config{})
	doc, errs := gqlparser.LoadQuery(es.Schema(), query)
	if errs != nil {
		t.Fatalf("クエリが解析できない: %v", errs)
	}
	op := doc.Operations[0]
	if vars == nil {
		vars = map[string]any{}
	}
	// 既定値つきの引数は、本番と同じく適用後の値で数える必要がある
	// （limit を省いたクエリの重みが 0 になってしまわないように）。
	raw, err := validator.VariableValues(es.Schema(), op, vars)
	if err != nil {
		t.Fatalf("変数が解決できない: %v", err)
	}
	return complexity.Calculate(context.Background(), es, op, raw)
}

// --- 防ぎたいクエリ -------------------------------------------------------

// TestComplexity_RejectsNestedReplyExpansion は項目8の本体。
//
// フィールド数は十数個しかないので、件数を見ない固定上限は素通りする。
// 実際に取りに行くのは 100 × 100 × 100 件。
func TestComplexity_RejectsNestedReplyExpansion(t *testing.T) {
	const query = `
	query {
	  getPostByID(id: "1") {
	    replies(limit: 100) {
	      replies(limit: 100) {
	        replies(limit: 100) {
	          ID
	          content
	        }
	      }
	    }
	  }
	}`

	got := calcComplexity(t, query, nil)
	if got <= ComplexityLimit {
		t.Fatalf("複雑度 = %d, want > %d（3階層の返信展開は弾くこと）", got, ComplexityLimit)
	}
}

// TestComplexity_RejectsTwoLevelReplyExpansion は、2階層でも弾けることを確かめる。
// 100 × 100 = 1万件で、画面から出ることはない。
func TestComplexity_RejectsTwoLevelReplyExpansion(t *testing.T) {
	const query = `
	query {
	  getPostByID(id: "1") {
	    replies(limit: 100) {
	      replies(limit: 100) {
	        ID
	        content
	      }
	    }
	  }
	}`

	got := calcComplexity(t, query, nil)
	if got <= ComplexityLimit {
		t.Fatalf("複雑度 = %d, want > %d（2階層の返信展開は弾くこと）", got, ComplexityLimit)
	}
}

// TestComplexity_RejectsNestedAnswerExpansion は Question.answers 側。
// questions(limit: 50) の下に answers(limit: 100) を置くと 5000 件になる。
func TestComplexity_RejectsNestedAnswerExpansion(t *testing.T) {
	const query = `
	query {
	  questions(roomID: "1", limit: 50) {
	    items {
	      answers(limit: 100) {
	        items {
	          ID
	          body
	          user { ID name accountID avatarUrl }
	        }
	      }
	    }
	  }
	}`

	got := calcComplexity(t, query, nil)
	if got <= ComplexityLimit {
		t.Fatalf("複雑度 = %d, want > %d（質問 × 回答の展開は弾くこと）", got, ComplexityLimit)
	}
}

// --- 通さないといけないクエリ ---------------------------------------------

// postFields はクライアントの PostFields フラグメント（SPACE-client の
// src/features/user/api/post.ts）と同じ選択。実際に流れている中で一番重い。
const postFields = `
  ID
  content
  createdAt
  replyCount
  deletedAt
  user { ID name accountID avatarUrl }
  favoriteCount
  isFavoritedByMe
  media { ID url contentType width height }
  mentions { user { ID name accountID avatarUrl } text }
`

// TestComplexity_AllowsTheRealPostDetailQuery は、投稿詳細（GetPostByID）が
// 通ることを確かめる。ここが通らないと投稿を開けない。
func TestComplexity_AllowsTheRealPostDetailQuery(t *testing.T) {
	query := `
	query {
	  getPostByID(id: "1") {
	    ` + postFields + `
	    rootPost { ` + postFields + ` }
	    replies(limit: 50) { ` + postFields + ` }
	  }
	}`

	got := calcComplexity(t, query, nil)
	if got > ComplexityLimit {
		t.Fatalf("複雑度 = %d, want <= %d（実際に流れている投稿詳細のクエリ）", got, ComplexityLimit)
	}
	t.Logf("投稿詳細の複雑度 = %d（上限 %d）", got, ComplexityLimit)
}

// TestComplexity_AllowsTheRealRepliesPagingQuery は返信のページ送り（GetPostReplies）。
func TestComplexity_AllowsTheRealRepliesPagingQuery(t *testing.T) {
	query := `
	query GetPostReplies($id: ID!, $limit: Int!, $offset: Int!) {
	  getPostByID(id: $id) {
	    replies(limit: $limit, offset: $offset) { ` + postFields + ` }
	  }
	}`

	got := calcComplexity(t, query, map[string]any{"id": "1", "limit": 50, "offset": 0})
	if got > ComplexityLimit {
		t.Fatalf("複雑度 = %d, want <= %d（返信のページ送り）", got, ComplexityLimit)
	}
	t.Logf("返信ページ送りの複雑度 = %d（上限 %d）", got, ComplexityLimit)
}

// TestComplexity_AllowsTheRealTimelineQuery はタイムライン（posts）。
// Query 直下の一覧には重みを付けていないので、ここは軽いままであるはず。
func TestComplexity_AllowsTheRealTimelineQuery(t *testing.T) {
	query := `
	query {
	  posts(limit: 20) {
	    items { ` + postFields + ` }
	    total
	  }
	}`

	got := calcComplexity(t, query, nil)
	if got > ComplexityLimit {
		t.Fatalf("複雑度 = %d, want <= %d（タイムライン）", got, ComplexityLimit)
	}
	t.Logf("タイムラインの複雑度 = %d（上限 %d）", got, ComplexityLimit)
}

// TestComplexity_AllowsTheRealQuestionListQuery は授業の質問一覧。
func TestComplexity_AllowsTheRealQuestionListQuery(t *testing.T) {
	query := `
	query {
	  questions(roomID: "1", limit: 50) {
	    items {
	      ID body isAnswered createdAt
	      user { ID name accountID avatarUrl }
	      bestAnswer { ID body }
	      media { ID url contentType }
	    }
	    total
	  }
	}`

	got := calcComplexity(t, query, nil)
	if got > ComplexityLimit {
		t.Fatalf("複雑度 = %d, want <= %d（質問一覧）", got, ComplexityLimit)
	}
	t.Logf("質問一覧の複雑度 = %d（上限 %d）", got, ComplexityLimit)
}

// messageFields はクライアントのチャット履歴クエリ（ListMessages、SPACE-client の
// src/features/user/api/message.ts）と同じ選択。実クエリの中で一番重い。
const messageFields = `
  ID
  roomID
  user { ID name accountID avatarUrl }
  content
  media { ID url contentType width height }
  createdAt
  updatedAt
  isMine
  replyToID
  replyTo {
    ID
    user { ID name accountID avatarUrl }
    content
    media { ID url contentType width height }
    isMine
  }
  mentions { user { ID name accountID avatarUrl } text }
`

// TestComplexity_AllowsTheHeaviestRealQuery は、実クエリの中で一番重いものが、
// 呼び出し側が渡しうる一番大きい件数（管理画面の 200）でも通ることを確かめる。
//
// 普段は 50 で呼ばれているが、上限は「普段」ではなく「渡しうる最大」で決めないと、
// 管理画面を開いた人だけがクエリを拒否される。
func TestComplexity_AllowsTheHeaviestRealQuery(t *testing.T) {
	query := `
	query ListMessages($roomID: ID!, $limit: Int) {
	  messages(roomID: $roomID, limit: $limit) {
	    items { ` + messageFields + ` }
	    hasMoreBefore
	    hasMoreAfter
	  }
	}`

	got := calcComplexity(t, query, map[string]any{"roomID": "1", "limit": 200})
	if got > ComplexityLimit {
		t.Fatalf("複雑度 = %d, want <= %d（実クエリ中で最重のチャット履歴）", got, ComplexityLimit)
	}
	t.Logf("チャット履歴(limit 200)の複雑度 = %d（上限 %d）", got, ComplexityLimit)
}

// TestComplexity_LimitHasHeadroomOverRealQueries は、上限が「いま実際に
// 投げられているクエリ」に対して余裕を持っていることを確かめる。
//
// ぴったりに寄せると、画面にフィールドを1つ足しただけで本番が落ちる。
// 上の TestComplexity_AllowsTheHeaviestRealQuery が「サーバーが許す最大でも
// 通る」を見るのに対し、こちらは「普段の使い方に余裕がある」を見る。
func TestComplexity_LimitHasHeadroomOverRealQueries(t *testing.T) {
	query := `
	query ListMessages($roomID: ID!, $limit: Int) {
	  messages(roomID: $roomID, limit: $limit) {
	    items { ` + messageFields + ` }
	    hasMoreBefore
	    hasMoreAfter
	  }
	}`

	// 50 はチャット画面と管理画面がいま渡している件数
	// （SPACE-client の listMessages / adminMessagePageSize）。
	got := calcComplexity(t, query, map[string]any{"roomID": "1", "limit": 50})
	if got*3 > ComplexityLimit {
		t.Fatalf("普段のクエリの複雑度 = %d、上限 = %d。3倍の余裕が無い", got, ComplexityLimit)
	}
	t.Logf("普段のチャット履歴(limit 50)の複雑度 = %d（上限 %d）", got, ComplexityLimit)
}

// TestComplexity_UsesTheEffectiveLimit は、複雑度がサーバーの実際の頭打ちに
// 合わせて数えられることを確かめる。
//
// 要求された値をそのまま使うと、丸められるはずの大きな limit が「クエリが
// 大きすぎます」で拒否される（説明のつかないエラーになる）。
func TestComplexity_UsesTheEffectiveLimit(t *testing.T) {
	query := `
	query Posts($limit: Int) {
	  posts(limit: $limit) { items { ID content } total }
	}`

	atCap := calcComplexity(t, query, map[string]any{"limit": maxMessagePageSize})
	absurd := calcComplexity(t, query, map[string]any{"limit": 1000000})
	if atCap != absurd {
		t.Fatalf("複雑度 = %d（上限ちょうど）と %d（桁違い）。サーバーは同じ件数しか返さないので同じ値になること", atCap, absurd)
	}
	if absurd > ComplexityLimit {
		t.Fatalf("丸められるはずのクエリが拒否される（複雑度 %d > 上限 %d）", absurd, ComplexityLimit)
	}
}

// TestComplexity_ChargesUncappedCollectionsAtTheirCeiling は、limit を省ける
// コレクション（Post.favorites）が、省かれたときも件数ぶん数えられることを
// 確かめる。
//
// ここが 0 に落ちると、引数を送らないだけで重み付けを回避できる。
// 実際には unpagedCollectionCap 件まで返るので、一覧の下に置かれると
// 「投稿の件数 × その上限」が1回の応答に乗る。
func TestComplexity_ChargesUncappedCollectionsAtTheirCeiling(t *testing.T) {
	withFavorites := `
	query {
	  topLevelPosts(limit: 100) { items { ID favorites { ID } } }
	}`
	withoutFavorites := `
	query {
	  topLevelPosts(limit: 100) { items { ID } }
	}`

	got := calcComplexity(t, withFavorites, nil)
	base := calcComplexity(t, withoutFavorites, nil)
	if got <= base*2 {
		t.Fatalf("favorites 付き = %d, 無し = %d。引数を省いた favorites に重みが付いていない", got, base)
	}
	if got <= ComplexityLimit {
		t.Fatalf("複雑度 = %d, want > %d（投稿100件 × いいね上限の展開は弾くこと）", got, ComplexityLimit)
	}
}
