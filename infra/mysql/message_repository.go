package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/messagecrypto"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// メッセージ周りの実装は関心ごとに分けてある（合成インターフェースを満たすのは
// 同じ struct のまま。暗号鍵を持つ口を増やしたくないため実体は1つ）:
//   - message_store.go        … 1件単位の読み書き（MessageReader / MessageWriter）
//   - message_read_model.go   … 一覧取得（MessageReadModel）
//   - message_mention_store.go… メンション行（MessageMentionStore）
//   - message_unread_counter.go … 未読集計（MessageUnreadCounter）
//
// このファイルには生成と、上のどれからも使う暗号・scan のヘルパだけを置く。
var _ repository.MessageRepository = &MySQLMessageRepository{}

type MySQLMessageRepository struct {
	DB     *sql.DB
	cipher *messagecrypto.Cipher
}

func NewMySQLMessageRepository(db *sql.DB) (*MySQLMessageRepository, error) {
	cipher, err := messagecrypto.New(os.Getenv("MESSAGE_ENCRYPTION_KEY"))
	if err != nil {
		return nil, err
	}
	return &MySQLMessageRepository{DB: db, cipher: cipher}, nil
}

// messageColumns は messages を読むときの共通の列並び。scanMessage と対になっており、
// 片方だけ直すと取り違えるので必ず一緒に変更する。
const messageColumns = "id, room_id, user_id, author_role, content, reply_to_message_id, created_at, updated_at"

// qualifiedMessageColumns は messageColumns にテーブル別名を付けた並びを返す。
// JOIN を伴う SELECT でも列並びを1箇所に保つためのヘルパ。
func qualifiedMessageColumns(alias string) string {
	parts := strings.Split(messageColumns, ", ")
	for i, p := range parts {
		parts[i] = alias + "." + p
	}
	return strings.Join(parts, ", ")
}

// rowScanner は *sql.Row と *sql.Rows の両方を同じ scan ヘルパで扱うための最小の口。
type rowScanner interface {
	Scan(dest ...interface{}) error
}

// scanMessage は messageColumns の並びで1行読み、本文を復号して返す。
func (r *MySQLMessageRepository) scanMessage(scanner rowScanner) (*model.Message, error) {
	var m model.Message
	var createdAt, updatedAt int64
	if err := scanner.Scan(&m.ID, &m.RoomID, &m.UserID, &m.AuthorRole, &m.Content, &m.ReplyToID, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	m.CreatedAt = time.Unix(createdAt, 0)
	m.UpdatedAt = time.Unix(updatedAt, 0)
	if err := r.decryptMessage(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *MySQLMessageRepository) encryptContent(content string) (string, error) {
	encrypted, err := r.cipher.Encrypt(content)
	if err != nil {
		return "", fmt.Errorf("encrypt message content: %w", err)
	}
	return encrypted, nil
}

func (r *MySQLMessageRepository) decryptMessage(m *model.Message) error {
	content, err := r.cipher.Decrypt(m.Content)
	if err != nil {
		return fmt.Errorf("decrypt message content: %w", err)
	}
	m.Content = content
	return nil
}

// EncryptPlaintextMessages は暗号化導入前に平文で保存された本文を、起動時に
// まとめて暗号化し直す運用バッチ。repository.MessageRepository には含めず
// （アプリの通常動作では呼ばない）、main.go から実体型に対して直接呼ぶ。
// 暗号鍵を持つのがこの struct なので、置き場所としてはここが最も近い。
func (r *MySQLMessageRepository) EncryptPlaintextMessages(ctx context.Context, batchSize int) (int, error) {
	if batchSize <= 0 {
		batchSize = 500
	}

	total := 0
	for {
		rows, err := r.DB.QueryContext(ctx, `
			SELECT id, content
			FROM messages
			WHERE content NOT LIKE ?
			ORDER BY id ASC
			LIMIT ?
		`, messagecrypto.Prefix+"%", batchSize)
		if err != nil {
			return total, err
		}

		type messageContent struct {
			id      int64
			content string
		}
		var batch []messageContent
		for rows.Next() {
			var item messageContent
			if err := rows.Scan(&item.id, &item.content); err != nil {
				rows.Close()
				return total, err
			}
			batch = append(batch, item)
		}
		if err := rows.Close(); err != nil {
			return total, err
		}
		if err := rows.Err(); err != nil {
			return total, err
		}
		if len(batch) == 0 {
			return total, nil
		}

		for _, item := range batch {
			encrypted, err := r.encryptContent(item.content)
			if err != nil {
				return total, err
			}
			result, err := r.DB.ExecContext(ctx, `
				UPDATE messages
				SET content = ?
				WHERE id = ? AND content = ?
			`, encrypted, item.id, item.content)
			if err != nil {
				return total, err
			}
			rowsAffected, err := result.RowsAffected()
			if err != nil {
				return total, err
			}
			total += int(rowsAffected)
		}
	}
}
