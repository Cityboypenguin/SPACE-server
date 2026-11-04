package model

// ChatStamp is a shared image definition; message and reaction usage are independent.
type ChatStamp struct {
	ID            string
	Label         string
	ImageKey      string
	DisplaySize   string
	Status        string
	AllowMessage  bool
	AllowReaction bool
	MessageOrder  int
	ReactionOrder int
	Version       int64
	CreatedAt     int64
	UpdatedAt     int64
	CreatedBy     int64
	UpdatedBy     int64
}
