CREATE TABLE IF NOT EXISTS room_anonymous_identities (
    id         BIGINT      NOT NULL AUTO_INCREMENT,
    room_id    BIGINT      NOT NULL,
    user_id    BIGINT      NOT NULL,
    label      VARCHAR(50) NOT NULL,
    created_at BIGINT      NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY unique_room_anon_room_user (room_id, user_id),
    /*同じ部屋に同じ表示名が2人現れないことを DB 側でも止める安全網。
      採番はカウンタで原子的に進めるので通常ここには当たらないが、
      当たったときは重複ラベルを書くより INSERT を失敗させる方が安全*/
    UNIQUE KEY unique_room_anon_room_label (room_id, label),
    CONSTRAINT fk_room_anon_room FOREIGN KEY (room_id) REFERENCES rooms(id) ON DELETE CASCADE,
    CONSTRAINT fk_room_anon_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
