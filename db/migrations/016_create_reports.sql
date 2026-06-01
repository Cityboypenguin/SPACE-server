CREATE TABLE IF NOT EXISTS user_reports (
    id            VARCHAR(255) NOT NULL,
    target_type   VARCHAR(50)  NOT NULL,
    target_id     VARCHAR(255) NOT NULL,
    reporter_id   VARCHAR(255) NOT NULL,
    reason        VARCHAR(100) NOT NULL,
    custom_reason TEXT,
    status        VARCHAR(50)  NOT NULL DEFAULT 'pending',
    created_at    BIGINT(3)  NOT NULL,
    updated_at    BIGINT(3)  NOT NULL,
    PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;