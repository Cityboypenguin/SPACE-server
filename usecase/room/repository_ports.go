package room

import "github.com/Cityboypenguin/SPACE-server/repository"

// leaveRoomRepository は退出に要る口。
//
// 在籍を外す前に「最後のオーナーが抜けようとしていないか」を役割で確かめるので、
// 在籍と役割の両方に触る。
type leaveRoomRepository interface {
	repository.RoomMembershipWriter
	repository.RoomRoleRepository
}
