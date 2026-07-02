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

// Percentile returns the p-th percentile (0-100) of recorded response times.
func (m *Metrics) Percentile(p float64) float64 {
	m.mu.Lock()

	size := ringBufferSize
	if !m.rtFull {
		size = m.rtHead
	}
	if size == 0 {
		m.mu.Unlock()
		return 0
	}

	data := make([]float64, size)
	copy(data, m.responseTimes[:size])
	m.mu.Unlock()

	sort.Float64s(data)
	idx := int(p/100*float64(size-1) + 0.5)
	if idx >= size {
		idx = size - 1
	}
	return data[idx]
}
