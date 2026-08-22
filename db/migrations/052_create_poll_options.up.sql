CREATE TABLE IF NOT EXISTS poll_options (
    id            BIGINT       NOT NULL AUTO_INCREMENT,
    poll_id       BIGINT       NOT NULL,
    label         VARCHAR(255) NOT NULL,
    display_order INT          NOT NULL DEFAULT 0,
    PRIMARY KEY (id),
    INDEX idx_poll_options_poll (poll_id),
    CONSTRAINT fk_poll_options_poll FOREIGN KEY (poll_id) REFERENCES polls(id) ON DELETE CASCADE
);
