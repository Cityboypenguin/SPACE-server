CREATE TABLE IF NOT EXISTS page_view_stats (
    id                      BIGINT       NOT NULL AUTO_INCREMENT,
    user_id                 BIGINT       NOT NULL,
    date                    DATE         NOT NULL,
    page_path               VARCHAR(500) NOT NULL,
    view_count              INT          NOT NULL DEFAULT 0,
    total_duration_seconds  BIGINT       NOT NULL DEFAULT 0,
    total_max_scroll_depth  INT          NOT NULL DEFAULT 0,
    created_at              BIGINT       NOT NULL,
    updated_at              BIGINT       NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY unique_user_date_page (user_id, date, page_path),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
