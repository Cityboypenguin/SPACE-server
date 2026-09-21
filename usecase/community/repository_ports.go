package community

import "github.com/Cityboypenguin/SPACE-server/repository"

// communityMemberRepository はメンバー編集（昇格・降格・除名）に要る口。
//
// 除名は在籍を外す操作、昇格・降格は役割を変える操作なので、2つの役割に
// またがる。まとめて1つのトランザクションで適用するため、同じ口で受け取る。
type communityMemberRepository interface {
	repository.RoomMembershipWriter
	repository.RoomRoleRepository
}
