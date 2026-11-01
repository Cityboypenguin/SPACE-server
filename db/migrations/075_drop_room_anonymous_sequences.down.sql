CREATE TABLE IF NOT EXISTS room_anonymous_sequences (
    room_id    BIGINT NOT NULL,
    next_seq   BIGINT NOT NULL, /*次に配る番号。1番を配った直後は 2*/
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL,
    PRIMARY KEY (room_id),
    CONSTRAINT fk_room_anon_seq_room FOREIGN KEY (room_id) REFERENCES rooms(id) ON DELETE CASCADE
);
