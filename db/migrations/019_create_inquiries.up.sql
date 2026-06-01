CREATE TABLE IF NOT EXISTS inquiries (
    id         VARCHAR(36)  NOT NULL,
    name       VARCHAR(255) NOT NULL,
    email      VARCHAR(255) NOT NULL,
    subject    VARCHAR(255) NOT NULL,
    content    TEXT         NOT NULL,
    created_at BIGINT       NOT NULL,
    PRIMARY KEY (id)
) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;