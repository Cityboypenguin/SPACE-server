-- 授業内チャットの既読位置。
--
-- 授業内チャットは room_users の membership を使わない（誰でも閲覧でき匿名で表示する）
-- ため、既読位置を room_users に持てずこの表が要る。匿名ID
-- (room_anonymous_identities) とも分けてある: 「匿名表示名の採番」は投稿した人だけ、
-- 「どこまで読んだか」は読んだ人だけが行を持つ、寿命も更新頻度も別の関心事のため。
--
-- 既読位置は last_read_message_id（メッセージID）が正で、last_read_at（Unix秒）は
-- 表示用に残してある。秒解像度の時刻を位置に使うと、既読更新と新着が同じ秒に起きた
-- ときに「既読にした後に保存されたメッセージ」が created_at > last_read_at を満たさず
-- 未読から漏れる（同じ秒の中での前後関係は時刻からは分からない）。AUTO_INCREMENT の
-- ID なら保存順に単調増加するので、この取りこぼしが原理的に起きない。
CREATE TABLE IF NOT EXISTS course_room_reads (
    id                   BIGINT NOT NULL AUTO_INCREMENT,
    room_id              BIGINT NOT NULL,
    user_id              BIGINT NOT NULL,
    last_read_message_id BIGINT NULL, /*既読位置の正。NULL は「まだ一度も読んでいない」*/
    last_read_at         BIGINT NOT NULL, /*既読にした時刻。表示用で、未読判定の起点としてはフォールバック*/
    created_at           BIGINT NOT NULL,
    updated_at           BIGINT NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY unique_course_room_reads_room_user (room_id, user_id),
    CONSTRAINT fk_course_room_reads_room FOREIGN KEY (room_id) REFERENCES rooms(id) ON DELETE CASCADE,
    CONSTRAINT fk_course_room_reads_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
