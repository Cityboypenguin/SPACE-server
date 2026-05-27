CREATE TABLE IF NOT EXISTS communities (
    id          BIGINT       NOT NULL AUTO_INCREMENT,
    room_id     BIGINT       NOT NULL,
    name        VARCHAR(255) NOT NULL,
    description TEXT         NOT NULL,
    icon_url VARCHAR(255) NOT NULL DEFAULT '',
    created_at  BIGINT       NOT NULL,
    updated_at  BIGINT       NOT NULL,
    PRIMARY KEY (id),
    FOREIGN KEY (room_id) REFERENCES rooms(id) ON DELETE CASCADE
);
