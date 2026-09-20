package metrics

import (
	"sort"
	"sync"
	"sync/atomic"
)

const ringBufferSize = 10000

// Global singleton
var Global = &Metrics{}

type Metrics struct {
	wsConnections  atomic.Int64
	sseConnections atomic.Int64

	mu            sync.Mutex
	totalRequests int64
	errorCount    int64
	responseTimes []float64 // ring buffer
	rtHead        int
	rtFull        bool
}

func (m *Metrics) IncWSConnections()  { m.wsConnections.Add(1) }
func (m *Metrics) DecWSConnections()  { m.wsConnections.Add(-1) }
func (m *Metrics) WSConnections() int { return int(m.wsConnections.Load()) }

func (m *Metrics) IncSSEConnections()  { m.sseConnections.Add(1) }
func (m *Metrics) DecSSEConnections()  { m.sseConnections.Add(-1) }
func (m *Metrics) SSEConnections() int { return int(m.sseConnections.Load()) }

func (m *Metrics) RecordRequest(durationMs float64, isError bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.totalRequests++
	if isError {
		m.errorCount++
	}

	if m.responseTimes == nil {
		m.responseTimes = make([]float64, ringBufferSize)
	}
	m.responseTimes[m.rtHead] = durationMs
	m.rtHead = (m.rtHead + 1) % ringBufferSize
	if m.rtHead == 0 {
		m.rtFull = true
	}
}

func (m *Metrics) ErrorRate() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.totalRequests == 0 {
		return 0
	}
	return float64(m.errorCount) / float64(m.totalRequests) * 100
}

// Percentiles は応答時間のパーセンタイル3値。
type Percentiles struct {
	P50 float64
	P95 float64
	P99 float64
}

// ResponseTimePercentiles は p50 / p95 / p99 を1回のコピー＆ソートで返す。
//
// 以前は Percentile(p) を p50/p95/p99 の3回呼んでおり、呼び出しのたびに
// 最大 ringBufferSize（1万）件のコピーとソートが走っていた（同じ入力に対して
// 3回）。読む側は必ず3値そろえて欲しがる（管理画面のサマリー1行）ので、
// 3値を返す口にしてコピーとソートを1回に畳んだ。
//
// 値は従来の Percentile と完全に一致する（同じ添字計算を共有している）。
func (m *Metrics) ResponseTimePercentiles() Percentiles {
	data := m.sortedResponseTimes()
	return Percentiles{
		P50: percentileOfSorted(data, 50),
		P95: percentileOfSorted(data, 95),
		P99: percentileOfSorted(data, 99),
	}
}

// Percentile returns the p-th percentile (0-100) of recorded response times.
//
// 3値まとめて要るときは ResponseTimePercentiles を使うこと（ソートが1回で済む）。
func (m *Metrics) Percentile(p float64) float64 {
	return percentileOfSorted(m.sortedResponseTimes(), p)
}

// sortedResponseTimes はリングバッファの有効部分だけをコピーして昇順に並べる。
// ロックを握るのはコピーの間だけ（ソートはロックの外）。
func (m *Metrics) sortedResponseTimes() []float64 {
	m.mu.Lock()
	size := ringBufferSize
	if !m.rtFull {
		size = m.rtHead
	}
	if size == 0 {
		m.mu.Unlock()
		return nil
	}
	data := make([]float64, size)
	copy(data, m.responseTimes[:size])
	m.mu.Unlock()

	sort.Float64s(data)
	return data
}

// percentileOfSorted は昇順済みのデータから p 番目（0-100）を取る。
// 添字の丸め方は元の Percentile のままなので、値は変わらない。
func percentileOfSorted(data []float64, p float64) float64 {
	size := len(data)
	if size == 0 {
		return 0
	}
	idx := int(p/100*float64(size-1) + 0.5)
	if idx >= size {
		idx = size - 1
	}
	return data[idx]
}
