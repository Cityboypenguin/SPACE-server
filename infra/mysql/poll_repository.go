package mysql

import (
	"context"
	"database/sql"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

var _ repository.PollRepository = &MySQLPollRepository{}

type MySQLPollRepository struct {
	DB *sql.DB
}

func NewMySQLPollRepository(db *sql.DB) repository.PollRepository {
	return &MySQLPollRepository{DB: db}
}

func (r *MySQLPollRepository) CreatePoll(ctx context.Context, param repository.CreatePollParam) (*model.Poll, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	now := time.Now()
	nowUnix := now.Unix()

	var deadlineUnix sql.NullInt64
	if param.Deadline != nil {
		deadlineUnix = sql.NullInt64{Int64: param.Deadline.Unix(), Valid: true}
	}

	result, err := tx.ExecContext(ctx,
		`INSERT INTO polls (room_id, author_user_id, author_role, question, allow_multiple_choice, deadline, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		param.RoomID, param.AuthorUserID, param.AuthorRole, param.Question, param.AllowMultipleChoice, deadlineUnix, nowUnix, nowUnix,
	)
	if err != nil {
		return nil, err
	}
	pollID, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	for i, label := range param.OptionLabels {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO poll_options (poll_id, label, display_order) VALUES (?, ?, ?)`,
			pollID, label, i,
		); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &model.Poll{
		ID:                  pollID,
		RoomID:              param.RoomID,
		AuthorUserID:        param.AuthorUserID,
		AuthorRole:          param.AuthorRole,
		Question:            param.Question,
		AllowMultipleChoice: param.AllowMultipleChoice,
		Deadline:            param.Deadline,
		CreatedAt:           now,
		UpdatedAt:           now,
	}, nil
}

func (r *MySQLPollRepository) GetPollByID(ctx context.Context, id int64) (*model.Poll, error) {
	row := r.DB.QueryRowContext(ctx,
		`SELECT id, room_id, author_user_id, author_role, question, allow_multiple_choice, deadline, created_at, updated_at
		 FROM polls WHERE id = ?`, id)
	return scanPoll(row)
}

func (r *MySQLPollRepository) ListPollsByRoomID(ctx context.Context, roomID int64, limit, offset int) ([]*model.Poll, int, error) {
	var total int
	if err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM polls WHERE room_id = ?`, roomID).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.DB.QueryContext(ctx,
		`SELECT id, room_id, author_user_id, author_role, question, allow_multiple_choice, deadline, created_at, updated_at
		 FROM polls WHERE room_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		roomID, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var list []*model.Poll
	for rows.Next() {
		p, err := scanPoll(rows)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, p)
	}
	return list, total, rows.Err()
}

func (r *MySQLPollRepository) CountUnvotedPollsByRoomID(ctx context.Context, roomID, viewerUserID int64) (int, error) {
	var total int
	err := r.DB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM polls p
		WHERE p.room_id = ?
		  AND NOT EXISTS (
		    SELECT 1
		    FROM poll_options po
		    JOIN poll_votes pv ON pv.poll_option_id = po.id
		    WHERE po.poll_id = p.id
		      AND pv.user_id = ?
		  )
	`, roomID, viewerUserID).Scan(&total)
	if err != nil {
		return 0, err
	}
	return total, nil
}

func (r *MySQLPollRepository) ListOptionsWithResults(ctx context.Context, pollID, viewerUserID int64) ([]*repository.PollOptionResult, error) {
	rows, err := r.DB.QueryContext(ctx, `
		SELECT po.id, po.poll_id, po.label, po.display_order,
		       COUNT(pv.id) AS vote_count,
		       SUM(CASE WHEN pv.user_id = ? THEN 1 ELSE 0 END) AS my_vote_count
		FROM poll_options po
		LEFT JOIN poll_votes pv ON pv.poll_option_id = po.id
		WHERE po.poll_id = ?
		GROUP BY po.id, po.poll_id, po.label, po.display_order
		ORDER BY po.display_order
	`, viewerUserID, pollID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*repository.PollOptionResult
	for rows.Next() {
		var opt model.PollOption
		var voteCount int
		var myVoteCount sql.NullInt64
		if err := rows.Scan(&opt.ID, &opt.PollID, &opt.Label, &opt.DisplayOrder, &voteCount, &myVoteCount); err != nil {
			return nil, err
		}
		list = append(list, &repository.PollOptionResult{
			Option:    &opt,
			VoteCount: voteCount,
			VotedByMe: myVoteCount.Valid && myVoteCount.Int64 > 0,
		})
	}
	return list, rows.Err()
}

func (r *MySQLPollRepository) ReplaceVotes(ctx context.Context, pollID, userID int64, optionIDs []int64) error {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		DELETE pv FROM poll_votes pv
		JOIN poll_options po ON pv.poll_option_id = po.id
		WHERE po.poll_id = ? AND pv.user_id = ?
	`, pollID, userID); err != nil {
		return err
	}

	now := time.Now().Unix()
	for _, optionID := range optionIDs {
		// INSERT ... SELECT confirms optionID actually belongs to pollID, so a caller
		// cannot vote for an option belonging to a different poll.
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO poll_votes (poll_option_id, user_id, created_at)
			SELECT id, ?, ? FROM poll_options WHERE id = ? AND poll_id = ?
		`, userID, now, optionID, pollID); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *MySQLPollRepository) DeletePoll(ctx context.Context, pollID int64) (bool, error) {
	result, err := r.DB.ExecContext(ctx, `DELETE FROM polls WHERE id = ?`, pollID)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

type pollScanner interface {
	Scan(dest ...any) error
}

func scanPoll(row pollScanner) (*model.Poll, error) {
	var p model.Poll
	var createdAt, updatedAt int64
	var deadline sql.NullInt64
	if err := row.Scan(&p.ID, &p.RoomID, &p.AuthorUserID, &p.AuthorRole, &p.Question, &p.AllowMultipleChoice, &deadline, &createdAt, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if deadline.Valid {
		t := time.Unix(deadline.Int64, 0)
		p.Deadline = &t
	}
	p.CreatedAt = time.Unix(createdAt, 0)
	p.UpdatedAt = time.Unix(updatedAt, 0)
	return &p, nil
}
