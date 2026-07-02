CREATE TABLE IF NOT EXISTS user_session_summaries (
    id                     BIGINT NOT NULL AUTO_INCREMENT,
    user_id                BIGINT NOT NULL,
    date                   DATE   NOT NULL,
    session_count          INT    NOT NULL DEFAULT 0,
    total_duration_seconds BIGINT NOT NULL DEFAULT 0,
    created_at             BIGINT NOT NULL,
    updated_at             BIGINT NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY unique_user_date (user_id, date),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
