package upload

import (
	"bytes"
	"encoding/xml"
	"net/http"
	"strings"
)

// sniffLimit は形式の判定に読む先頭のバイト数。
//
// http.DetectContentType が見るのは 512 バイトだが、SVG は根の要素が宣言や
// コメントの後ろに来るので、少し多めに読む。これより後ろに <svg> が現れる
// ファイルは受け付けない（判定できないものは通さない側に倒す）。
const sniffLimit = 4096

// contentTypeMatchesBytes は、申告された種別と実物のバイト列が一致するかを返す。
//
// # なぜ要るか
//
// 署名付き PUT では Content-Type も利用者が決める。保存されたメタデータは
// 「利用者がそう名乗った」という記録にすぎず、中身の保証ではない。中身が HTML の
// ファイルを image/png と名乗って置くことができる。そのまま配ると、ブラウザーが
// 中身を見て HTML として解釈する余地が残る（MIME スニッフィング）。
//
// # なぜ標準ライブラリで判定するか
//
// http.DetectContentType は MIME Sniffing Standard の実装で、ブラウザーが
// 使うのと同じ手順。これで image/png と判定されたものは、ブラウザーも
// image/png と判定する。自前のマジックナンバー表を持つと、この一致が崩れる。
func contentTypeMatchesBytes(declared string, head []byte) bool {
	if len(head) == 0 {
		return false
	}
	switch declared {
	case "image/svg+xml":
		// SVG はテキスト形式なのでバイト列では見分けられない
		// （http.DetectContentType は text/xml や text/plain を返す）。
		// 根の要素が svg であることを確かめる。
		return isSVGDocument(head)
	case "text/markdown":
		// マークダウンもテキストなので、同じく一致の確認ができない。
		// ここで見るのは「HTML として解釈されうる中身ではないこと」だけ。
		// 先頭が <div> や <!DOCTYPE html> のものは落ちる。管理者が書く規約で
		// 先頭を HTML で始める必要は無く、落とす側に倒して困らない。
		return strings.HasPrefix(http.DetectContentType(head), "text/plain")
	default:
		// 画像はバイト列で一致を確かめられる。ここは厳密に見る。
		return http.DetectContentType(head) == declared
	}
}

// isSVGDocument は先頭が「根の要素が svg の XML」かを見る。
//
// 中に script が入っていても true を返す。ここで見るのは「名乗った形式と
// 中身が一致しているか」だけで、SVG が持つ危険はこの層では落とせない
// （配信側の ServedContentDisposition と、別ホストからの配信で受ける）。
//
// XML として壊れているもの、根が svg でないもの（HTML を SVG と名乗る等）は
// 落ちる。
func isSVGDocument(head []byte) bool {
	decoder := xml.NewDecoder(bytes.NewReader(head))
	// 外部実体を読みに行かせない。XML の実体参照は、解析側が取りに行くと
	// サーバーから任意のURLへ要求を出させる手口（XXE）に使われる。
	decoder.Entity = map[string]string{}
	decoder.Strict = false

	for {
		token, err := decoder.Token()
		if err != nil {
			// 先頭だけを渡しているので、根の要素まで届かずに終わることがある。
			// 届かなければ判定できない＝受け付けない。
			return false
		}
		if start, ok := token.(xml.StartElement); ok {
			return strings.EqualFold(start.Name.Local, "svg")
		}
	}
}
