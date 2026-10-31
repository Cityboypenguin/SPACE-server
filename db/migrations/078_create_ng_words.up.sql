-- NGワード。授業内チャットの投稿（メッセージ・質問・回答）に含まれていたら、
-- 投稿の前に止める。match_type は 'word'（含む。大小・全半角は区別しない）か
-- 'regex'（正規表現。例: 9桁の数字）。
CREATE TABLE IF NOT EXISTS ng_words (
    id         BIGINT       NOT NULL AUTO_INCREMENT,
    pattern    VARCHAR(255) NOT NULL,
    match_type VARCHAR(16)  NOT NULL DEFAULT 'word',
    created_at BIGINT       NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY unique_ng_words_pattern (match_type, pattern)
)
