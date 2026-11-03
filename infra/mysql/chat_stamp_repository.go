package mysql

import (
	"context"
	"database/sql"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type ChatStampRepository struct{ DB *sql.DB }

const stampColumns = "id,label,image_key,display_size,status,allow_message,allow_reaction,message_order,reaction_order,version,created_at,updated_at,created_by,updated_by"

func scanStamp(s interface{ Scan(...any) error }) (*model.ChatStamp, error) {
	v := new(model.ChatStamp)
	err := s.Scan(&v.ID, &v.Label, &v.ImageKey, &v.DisplaySize, &v.Status, &v.AllowMessage, &v.AllowReaction, &v.MessageOrder, &v.ReactionOrder, &v.Version, &v.CreatedAt, &v.UpdatedAt, &v.CreatedBy, &v.UpdatedBy)
	return v, err
}
func (r *ChatStampRepository) List(ctx context.Context) ([]*model.ChatStamp, error) {
	rows, err := extractDB(ctx, r.DB).QueryContext(ctx, "SELECT "+stampColumns+" FROM chat_stamps ORDER BY message_order,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.ChatStamp{}
	for rows.Next() {
		v, e := scanStamp(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *ChatStampRepository) Get(ctx context.Context, id string) (*model.ChatStamp, error) {
	v, e := scanStamp(extractDB(ctx, r.DB).QueryRowContext(ctx, "SELECT "+stampColumns+" FROM chat_stamps WHERE id=?", id))
	if e == sql.ErrNoRows {
		return nil, nil
	}
	return v, e
}
func (r *ChatStampRepository) Create(ctx context.Context, v *model.ChatStamp) error {
	_, e := extractDB(ctx, r.DB).ExecContext(ctx, "INSERT INTO chat_stamps ("+stampColumns+") VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)", v.ID, v.Label, v.ImageKey, v.DisplaySize, v.Status, v.AllowMessage, v.AllowReaction, v.MessageOrder, v.ReactionOrder, v.Version, v.CreatedAt, v.UpdatedAt, v.CreatedBy, v.UpdatedBy)
	return e
}
func (r *ChatStampRepository) Update(ctx context.Context, v *model.ChatStamp) (bool, error) {
	res, e := extractDB(ctx, r.DB).ExecContext(ctx, `UPDATE chat_stamps SET label=?,image_key=?,display_size=?,status=?,allow_message=?,allow_reaction=?,message_order=?,reaction_order=?,updated_at=?,updated_by=?,version=version+1 WHERE id=? AND version=?`, v.Label, v.ImageKey, v.DisplaySize, v.Status, v.AllowMessage, v.AllowReaction, v.MessageOrder, v.ReactionOrder, v.UpdatedAt, v.UpdatedBy, v.ID, v.Version)
	if e != nil {
		return false, e
	}
	n, e := res.RowsAffected()
	return n == 1, e
}
