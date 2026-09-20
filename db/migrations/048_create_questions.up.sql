CREATE TABLE IF NOT EXISTS questions (
    id             BIGINT      NOT NULL AUTO_INCREMENT,
    room_id        BIGINT      NOT NULL,
    asker_user_id  BIGINT      NOT NULL,
    author_role    VARCHAR(20) NOT NULL DEFAULT 'STUDENT',
    body           TEXT        NOT NULL,
    is_answered    BOOLEAN     NOT NULL DEFAULT FALSE,
    best_answer_id BIGINT      DEFAULT NULL,
    created_at     BIGINT      NOT NULL,
    updated_at     BIGINT      NOT NULL,
    PRIMARY KEY (id),
    INDEX idx_questions_room (room_id),
    CONSTRAINT fk_questions_room FOREIGN KEY (room_id) REFERENCES rooms(id) ON DELETE CASCADE,
    CONSTRAINT fk_questions_asker FOREIGN KEY (asker_user_id) REFERENCES users(id) ON DELETE CASCADE
);
