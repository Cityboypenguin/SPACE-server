CREATE TABLE IF NOT EXISTS profiles (
    user_id BIGINT PRIMARY KEY,
    bio TEXT,
    avatar_key VARCHAR(255) DEFAULT NULL,
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
