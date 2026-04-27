package model

type Message struct {
	ID        int64  `json:"id"`
	RoomID    int64  `json:"room_id"`
	UserID    int64  `json:"user_id"`
	Content   string `json:"content"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

type CreateMessageParam struct {
	RoomID    int64  `json:"room_id"`
	UserID    int64  `json:"user_id"`
	Content   string `json:"content"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

type UpdateMessageParam struct {
	Content *string `json:"content,omitempty"`
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
}