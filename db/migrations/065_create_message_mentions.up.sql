CREATE TABLE message_mentions (
    id                BIGINT NOT NULL AUTO_INCREMENT,
    message_id        BIGINT NOT NULL,
    mentioned_user_id BIGINT NOT NULL, /*メンションされた側のユーザーID。送信時にクライアントの選択を検証して保存する*/
    /*本文に書かれた表記のスナップショット（コミュニティは表示名）。
      表示名は空白や記号を含みうるため本文からは終端を決められない。
      着色・リンク化はこの値との最長一致で行う。
      messages.content と同じ鍵で暗号化して保存するため TEXT（暗号文は元より長くなる）。*/
    mention_text      TEXT   NOT NULL,
    created_at        BIGINT NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY unique_message_mention (message_id, mentioned_user_id),
    INDEX idx_message_mentions_user (mentioned_user_id),
    CONSTRAINT fk_message_mentions_message FOREIGN KEY (message_id)
        REFERENCES messages(id) ON DELETE CASCADE,
    CONSTRAINT fk_message_mentions_user FOREIGN KEY (mentioned_user_id)
        REFERENCES users(id) ON DELETE CASCADE
);
