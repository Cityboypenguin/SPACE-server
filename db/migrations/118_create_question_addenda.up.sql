-- 質問への追記（Q-09）。追記できるのは質問者本人だけ。元の質問の本文は変えず、
-- 追記は別の行として足す（一度出した中身をしれっと変えられないようにするため）。
CREATE TABLE IF NOT EXISTS question_addenda (
    id          BIGINT NOT NULL AUTO_INCREMENT,
    question_id BIGINT NOT NULL,
    user_id     BIGINT NOT NULL,
    body        TEXT   NOT NULL,
    created_at  BIGINT NOT NULL,
    PRIMARY KEY (id),
    INDEX idx_question_addenda_question (question_id, id),
    CONSTRAINT fk_question_addenda_question FOREIGN KEY (question_id) REFERENCES questions(id) ON DELETE CASCADE,
    CONSTRAINT fk_question_addenda_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
)
