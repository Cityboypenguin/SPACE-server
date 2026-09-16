-- チャットのリプライ（引用返信）。返信先メッセージを1階層だけ指す。
--
-- 返信先が物理削除された場合に返信側まで消えないよう ON DELETE SET NULL にしている。
-- （通常の削除はソフトデリートなので、その場合は行が残り reply_to_message_id も保持される。
--  「削除されたメッセージへの返信」はクライアント側で返信先を取得できないことで表現する。）
ALTER TABLE messages
    ADD COLUMN reply_to_message_id BIGINT NULL AFTER content,
    ADD INDEX idx_messages_reply_to (reply_to_message_id),
    ADD CONSTRAINT fk_messages_reply_to FOREIGN KEY (reply_to_message_id) REFERENCES messages(id) ON DELETE SET NULL;
