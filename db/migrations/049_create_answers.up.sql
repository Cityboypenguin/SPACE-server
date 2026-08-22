CREATE TABLE IF NOT EXISTS answers (
    id             BIGINT      NOT NULL AUTO_INCREMENT,
    question_id    BIGINT      NOT NULL,
    author_user_id BIGINT      NOT NULL,
    author_role    VARCHAR(20) NOT NULL DEFAULT 'STUDENT',
    body           TEXT        NOT NULL,
    created_at     BIGINT      NOT NULL,
    updated_at     BIGINT      NOT NULL,
    PRIMARY KEY (id),
    INDEX idx_answers_question (question_id),
    CONSTRAINT fk_answers_question FOREIGN KEY (question_id) REFERENCES questions(id) ON DELETE CASCADE,
    CONSTRAINT fk_answers_author FOREIGN KEY (author_user_id) REFERENCES users(id) ON DELETE CASCADE
);
