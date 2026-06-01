CREATE TABLE IF NOT EXISTS announcements (
    id         BIGINT NOT NULL AUTO_INCREMENT,
    title      VARCHAR(255) NOT NULL,
    body       TEXT NOT NULL,
    admin_id   BIGINT NOT NULL,
    created_at BIGINT NOT NULL,
    PRIMARY KEY (id),
    FOREIGN KEY (admin_id) REFERENCES administrators(id) ON DELETE CASCADE,
    INDEX idx_announcements_created (created_at)
);
