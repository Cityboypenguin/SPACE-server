CREATE TABLE IF NOT EXISTS polls (
    id                    BIGINT       NOT NULL AUTO_INCREMENT,
    room_id               BIGINT       NOT NULL,
    author_user_id        BIGINT       NOT NULL,
    author_role           VARCHAR(20)  NOT NULL DEFAULT 'STUDENT',
    question              VARCHAR(255) NOT NULL,
    allow_multiple_choice BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at            BIGINT       NOT NULL,
    updated_at            BIGINT       NOT NULL,
    PRIMARY KEY (id),
    INDEX idx_polls_room (room_id),
    CONSTRAINT fk_polls_room FOREIGN KEY (room_id) REFERENCES rooms(id) ON DELETE CASCADE,
    CONSTRAINT fk_polls_author FOREIGN KEY (author_user_id) REFERENCES users(id) ON DELETE CASCADE
);
