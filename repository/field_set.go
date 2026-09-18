package repository

import (
	"sort"
	"strings"
)

// FieldSet は「応答に出るフィールドの名前の集合」を下層へ運ぶ値。
//
// ■ なぜ要るか
//
// 管理画面のサマリーは35本以上の集計 SQL を撃つが、GraphQL のクエリが選んでいる
// 項目は普通そのうち数本しかない。時系列も6系列すべてを計算していた。
// 「要求されていない派生値は計算しない」は graph/helpers.go の fieldRequested で
// 揃えてあるが、判定を下層（リポジトリ）まで運ぶ必要がある場合だけは、bool を
// 引数に並べずに明示的な型1つへ載せる。PageQuery.WithTotal と同じ考え方で、
// 増やし方を型に固定しておくと呼び出しごとに引数の並びが食い違わない。
//
// ■ 判定できないときは「全部」
//
// GraphQL の実行文脈の外（内部呼び出し・テスト）では All を使う。ここで
// 「何も要求されていない」に倒すと、本来出るはずの値が欠けたまま返ってしまう。
// 余計に計算するぶんには従来と同じ費用が乗るだけで、外から見える挙動は変わらない。
// fieldRequested が判定不能時に true を返すのと同じ倒し方。
type FieldSet struct {
	// all が true なら、どの名前を聞かれても要求されたものとして扱う。
	all bool
	// names は all が false のときの選択済みフィールド名。
	names map[string]struct{}
}

// AllFields は「全部要求された」FieldSet を返す。
func AllFields() FieldSet { return FieldSet{all: true} }

// NewFieldSet は選択済みフィールド名から FieldSet を作る。
func NewFieldSet(names []string) FieldSet {
	set := make(map[string]struct{}, len(names))
	for _, name := range names {
		set[name] = struct{}{}
	}
	return FieldSet{names: set}
}

// Wants は names のどれか1つでも要求されているかを返す。
//
// 引数を可変長にしてあるのは、1本の集計が複数のフィールドを賄うことがあるため
// （TotalPosts は totalPosts だけでなく avgLikesPerPost の分母でもある）。
// 「この集計が要るか」を1回の呼び出しで聞けるようにしておかないと、
// 派生値だけを選んだクエリで分母が 0 のまま返る。
func (s FieldSet) Wants(names ...string) bool {
	if s.all {
		return true
	}
	for _, name := range names {
		if _, ok := s.names[name]; ok {
			return true
		}
	}
	return false
}

// CacheKey は「何を計算したか」を表す決定的な文字列。
//
// キャッシュのキーに必ず混ぜること。混ぜないと「一部だけ計算した結果」が
// 全部を要求した側へ返り、計算していない項目が 0 のまま画面に出る
// （PageQuery.WithTotal をキーに足したのと同じ理由）。
func (s FieldSet) CacheKey() string {
	if s.all {
		return "*"
	}
	if len(s.names) == 0 {
		return "-"
	}
	names := make([]string, 0, len(s.names))
	for name := range s.names {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}
