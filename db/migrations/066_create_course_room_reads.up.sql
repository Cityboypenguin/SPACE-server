-- 授業内チャットの既読位置。
--
-- 授業内チャットは room_users の membership を使わない（誰でも閲覧でき匿名で表示する）
-- ため、既読位置を room_users に持てずこの表が要る。匿名ID
-- (room_anonymous_identities) とも分けてある: 「匿名表示名の採番」は投稿した人だけ、
-- 「どこまで読んだか」は読んだ人だけが行を持つ、寿命も更新頻度も別の関心事のため。
CREATE TABLE IF NOT EXISTS course_room_reads (
    id           BIGINT NOT NULL AUTO_INCREMENT,
    room_id      BIGINT NOT NULL,
    user_id      BIGINT NOT NULL,
    last_read_at BIGINT NOT NULL,
    created_at   BIGINT NOT NULL,
    updated_at   BIGINT NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY unique_course_room_reads_room_user (room_id, user_id),
    CONSTRAINT fk_course_room_reads_room FOREIGN KEY (room_id) REFERENCES rooms(id) ON DELETE CASCADE,
    CONSTRAINT fk_course_room_reads_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
