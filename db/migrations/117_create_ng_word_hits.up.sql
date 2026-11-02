-- NGワードで止めた投稿の記録（止めた回数と、言った人ごとの回数の集計元）。
-- 本文は残さない（投稿は保存されないので、残すと個人情報の置き場になる）。
-- pattern はそのときのNGワードの写し。NGワードを消しても回数が消えないようにする。
CREATE TABLE IF NOT EXISTS ng_word_hits (
    id         BIGINT       NOT NULL AUTO_INCREMENT,
    ng_word_id BIGINT       NULL,
    pattern    VARCHAR(255) NOT NULL,
    user_id    BIGINT       NOT NULL,
    room_id    BIGINT       NULL,
    source     VARCHAR(16)  NOT NULL,
    created_at BIGINT       NOT NULL,
    PRIMARY KEY (id),
    INDEX idx_ng_word_hits_user (user_id),
    INDEX idx_ng_word_hits_pattern (pattern),
    CONSTRAINT fk_ng_word_hits_word FOREIGN KEY (ng_word_id) REFERENCES ng_words(id) ON DELETE SET NULL,
    CONSTRAINT fk_ng_word_hits_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
)
