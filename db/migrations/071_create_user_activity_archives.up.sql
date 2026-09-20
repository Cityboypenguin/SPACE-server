CREATE TABLE user_activity_archives (
    archive_month DATE         NOT NULL,
    object_key    VARCHAR(512) NOT NULL,
    row_count     BIGINT       NOT NULL,
    sha256        CHAR(64)     NOT NULL,
    archived_at   BIGINT       NOT NULL,
    expires_at    BIGINT       NOT NULL,
    PRIMARY KEY (archive_month),
    INDEX idx_user_activity_archives_expires_at (expires_at)
) ENGINE=InnoDB;
