CREATE TABLE post_mentions (
    id                BIGINT       NOT NULL AUTO_INCREMENT,
    post_id           BIGINT       NOT NULL,
    mentioned_user_id BIGINT       NOT NULL, /*メンションされた側のユーザーID。投稿時に @accountID から解決して保存する*/
    /*本文に書かれた表記のスナップショット（投稿は accountID）。
      相手が accountID を変更しても本文の文字列は変わらないため、
      表示側の着色・リンク化はこの値と突き合わせて行う。*/
    mention_text      VARCHAR(255) NOT NULL,
    created_at        BIGINT       NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY unique_post_mention (post_id, mentioned_user_id), /*同じ投稿で同じ相手を二重にメンションしない*/
    INDEX idx_post_mentions_user (mentioned_user_id), /*「自分宛メンション」を引くための索引*/
    CONSTRAINT fk_post_mentions_post FOREIGN KEY (post_id)
        REFERENCES posts(id) ON DELETE CASCADE,
    CONSTRAINT fk_post_mentions_user FOREIGN KEY (mentioned_user_id)
        REFERENCES users(id) ON DELETE CASCADE
);
