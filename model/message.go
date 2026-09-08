package model

import "time"

type Message struct {
	ID        int64
	RoomID    int64
	UserID    int64
	Content   string
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
	DeletedBy *int64
}

// IsDeleted reports whether the message has been soft-deleted.
func (m *Message) IsDeleted() bool {
	return m.DeletedAt != nil
}

type CreateMessageParam struct {
	RoomID    int64
	UserID    int64
	Content   string
	MediaKeys []string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type UpdateMessageParam struct {
	Content *string
}

func (m *Message) CreateMessage(param CreateMessageParam) {
	m.RoomID = param.RoomID
	m.UserID = param.UserID
	m.Content = param.Content
	m.CreatedAt = param.CreatedAt
	m.UpdatedAt = param.UpdatedAt
}

func (m *Message) UpdateMessage(param UpdateMessageParam) {
	if param.Content != nil {
		m.Content = *param.Content
	}
	m.UpdatedAt = time.Now()
}
