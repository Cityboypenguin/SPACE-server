package notification

import "github.com/Cityboypenguin/SPACE-server/repository"

// notificationPublishRepository は通知の発行に要る口。
//
// 作るだけでなく読みもするのは、同じ対象への通知が既にあるかを見てから
// 作る経路（重複の抑制）があるため。
type notificationPublishRepository interface {
	repository.NotificationWriter
	repository.NotificationReader
}
