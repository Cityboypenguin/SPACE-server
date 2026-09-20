package repository

import "errors"

// ErrDuplicateKey は「一意制約に引っかかった」ことを表すリポジトリ共通のエラー。
//
// 重複の検出は DB の UNIQUE 制約に任せる（先に SELECT して存在確認してから INSERT
// する形は、確認と挿入の間に別リクエストが割り込めるので競合を防げない）。
// 制約違反をどう表現するかは DB ドライバ固有（MySQL なら errno 1062）なので、
// infra 層でこのエラーに包み替え、usecase 層は errors.Is(err, repository.ErrDuplicateKey)
// だけを見る。こうすると usecase がドライバに依存しない。
var ErrDuplicateKey = errors.New("duplicate key")
