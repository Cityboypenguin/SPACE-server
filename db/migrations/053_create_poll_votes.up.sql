CREATE TABLE IF NOT EXISTS poll_votes (
    id             BIGINT NOT NULL AUTO_INCREMENT,
    poll_option_id BIGINT NOT NULL,
    user_id        BIGINT NOT NULL,
    created_at     BIGINT NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY unique_poll_votes_option_user (poll_option_id, user_id),
    INDEX idx_poll_votes_user (user_id),
    CONSTRAINT fk_poll_votes_option FOREIGN KEY (poll_option_id) REFERENCES poll_options(id) ON DELETE CASCADE,
    CONSTRAINT fk_poll_votes_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
