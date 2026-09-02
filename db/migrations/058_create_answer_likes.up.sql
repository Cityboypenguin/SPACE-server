CREATE TABLE IF NOT EXISTS answer_likes (
    id         BIGINT NOT NULL AUTO_INCREMENT,
    answer_id  BIGINT NOT NULL,
    user_id    BIGINT NOT NULL,
    created_at BIGINT NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY unique_answer_likes_answer_user (answer_id, user_id),
    INDEX idx_answer_likes_user (user_id),
    CONSTRAINT fk_answer_likes_answer FOREIGN KEY (answer_id) REFERENCES answers(id) ON DELETE CASCADE,
    CONSTRAINT fk_answer_likes_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
