-- 授業内チャットの匿名表示名（匿名NNN）の採番だけを持つ表。
--
-- 行ができるのはそのルームで初めて投稿したときで、label はそのとき必ず割り当てる。
-- 読むだけの利用者には行を作らない（行があると「開いた人数」が推測できてしまう）。
--
-- 番号そのものは room_anonymous_sequences（067）が持つカウンタから配る。この表の
-- 行数からは採番しない（行が消えると番号が戻り、既存ラベルと衝突するため）。
--
-- 既読位置はこの表ではなく course_room_reads（066）が持つ。授業内チャットは
-- room_users の membership を使わない（誰でも閲覧でき匿名で表示する）ため
-- 既読位置を room_users に持てず、別表に分けてある。
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
