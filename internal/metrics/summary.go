package metrics

import "github.com/Cityboypenguin/SPACE-server/model"

// ApplyToSummary は「SQL を伴わない」実行時メトリクスを管理画面サマリーへ写す。
//
// この6項目はプロセスのメモリ上の値なので、集計キャッシュの有無に関わらず常に
// いまの値でなければならない。書く場所が2つ（集計本体と、キャッシュヒット時の
// 上書き）あり、片方だけ項目を足すと「キャッシュが効いている間だけ 0 が出る」
// という気づきにくい壊れ方をするので、写す口を1つにまとめてある。
//
// パーセンタイルは3値まとめて取る。個別に Percentile を3回呼ぶと、同じリング
// バッファ（最大1万件）のコピーとソートが3回走る。
func ApplyToSummary(s *model.AnalyticsSummary) {
	p := Global.ResponseTimePercentiles()
	s.WebSocketConnections = Global.WSConnections()
	s.SSEConnections = Global.SSEConnections()
	s.ErrorRate5xx = Global.ErrorRate()
	s.P50ResponseTimeMs = p.P50
	s.P95ResponseTimeMs = p.P95
	s.P99ResponseTimeMs = p.P99
}
