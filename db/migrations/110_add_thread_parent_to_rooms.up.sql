ALTER TABLE rooms
    ADD COLUMN parent_room_id BIGINT NULL,
    ADD CONSTRAINT fk_rooms_parent FOREIGN KEY (parent_room_id) REFERENCES rooms(id) ON DELETE CASCADE;
