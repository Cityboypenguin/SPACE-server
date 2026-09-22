-- 授業内チャットの既読位置。
--
-- 授業内チャットは room_users の membership を使わない（誰でも閲覧でき匿名で表示する）
-- ため、既読位置を room_users に持てずこの表が要る。匿名ID
-- (room_anonymous_identities) とも分けてある: 「匿名表示名の採番」は投稿した人だけ、
-- 「どこまで読んだか」は読んだ人だけが行を持つ、寿命も更新頻度も別の関心事のため。
--
-- 既読位置は当初 last_read_at（Unix秒）だけだった。メッセージIDで持つ列
-- (last_read_message_id) は 073 で足す。この表を作る側をいじらないのは、066 が既に
-- 適用された環境では CREATE TABLE が二度と走らず、ここへ列を書いても追加されない
-- ため（実際にそれで、列を使う既読更新と未読集計が稼働中の環境で落ちていた）。
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
