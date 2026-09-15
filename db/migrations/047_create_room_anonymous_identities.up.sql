-- 授業内チャットの匿名表示（匿名NNN）と既読位置を持つ表。
--
-- label は初めて投稿したときに割り当てる。読むだけの利用者にも既読位置のために
-- 行ができるので、それまでは NULL のままにして番号を消費させない
-- （番号から「開いた人数」が推測できてしまうのを防ぐ）。
--
-- last_read_at は授業内チャットの既読位置。授業内チャットは room_users の
-- membership を使わない（誰でも閲覧でき匿名で表示する）ため既読位置を
-- room_users に持てず、部屋×ユーザーで一意なこの表に持たせている。
CREATE TABLE IF NOT EXISTS room_anonymous_identities (
    id           BIGINT      NOT NULL AUTO_INCREMENT,
    room_id      BIGINT      NOT NULL,
    user_id      BIGINT      NOT NULL,
    label        VARCHAR(50) NULL,
    last_read_at BIGINT      NULL,
    created_at   BIGINT      NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY unique_room_anon_room_user (room_id, user_id),
    CONSTRAINT fk_room_anon_room FOREIGN KEY (room_id) REFERENCES rooms(id) ON DELETE CASCADE,
    CONSTRAINT fk_room_anon_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
