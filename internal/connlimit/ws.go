package connlimit

import (
	"fmt"
	"sync"
)

// MaxWSConnectionsPerUser は1ユーザーが同時に保持できるWebSocket接続数の上限。
// 複数タブを考慮して5としている。IPではなくユーザーID単位のため大学NATでも問題ない。
const MaxWSConnectionsPerUser = 5

// WSLimiter はユーザーIDごとのWebSocket同時接続数を管理する。
type WSLimiter struct {
	mu    sync.Mutex
	count map[int64]int
}

func NewWSLimiter() *WSLimiter {
	return &WSLimiter{count: make(map[int64]int)}
}

// Acquire は接続を1つ確保する。上限に達していた場合はエラーを返す。
func (l *WSLimiter) Acquire(userID int64) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.count[userID] >= MaxWSConnectionsPerUser {
		return fmt.Errorf("too many WebSocket connections for user %d (limit: %d)", userID, MaxWSConnectionsPerUser)
	}
	l.count[userID]++
	return nil
}

// Release は接続を1つ解放する。
func (l *WSLimiter) Release(userID int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.count[userID] > 0 {
		l.count[userID]--
	}
	if l.count[userID] == 0 {
		delete(l.count, userID)
	}
}
