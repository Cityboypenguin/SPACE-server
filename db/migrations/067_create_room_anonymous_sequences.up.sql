-- 授業内チャットの匿名表示名（匿名NNN）の採番カウンタ。ルームごとに1行。
--
-- 番号を room_anonymous_identities の COUNT(*)+1 で決めていたのをやめて、この表へ移した。
-- 理由は2つ。
--   1. COUNT(*) は行が減ると小さくなる。ユーザー削除で identity が CASCADE 削除されると
--      件数が戻り、既存の「匿名005」と同じ番号を再発行してしまう（同じ部屋に同名が2人）。
--   2. COUNT して INSERT する二段構えは同時投稿に弱く、MySQL の名前付きロック
--      (GET_LOCK) で守る必要があった。名前付きロックは接続単位なのに sql.DB は
--      文ごとに別接続へ振り分けうるため、そもそも正しく守れていなかった。
--
-- next_seq は「次に配る番号」で、単調増加させる（減らす経路を作らない）。
-- identity 行が消えても番号は再利用しないので、過去のラベルと衝突しない。
--
-- 採番は
--   INSERT ... VALUES (room_id, 2, ...) ON DUPLICATE KEY UPDATE next_seq = LAST_INSERT_ID(next_seq) + 1
-- の1文で原子的に進める。行が無ければ1番を配って next_seq=2 から始め、あれば現在値を
-- LAST_INSERT_ID() に載せてから +1 する。値はその文の OK パケット（Go の
-- Result.LastInsertId()）で受け取るので、別接続へ振り分けられても取り違えない。
CREATE TABLE IF NOT EXISTS room_anonymous_sequences (
    room_id    BIGINT NOT NULL,
    next_seq   BIGINT NOT NULL, /*次に配る番号。1番を配った直後は 2*/
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL,
    PRIMARY KEY (room_id),
    CONSTRAINT fk_room_anon_seq_room FOREIGN KEY (room_id) REFERENCES rooms(id) ON DELETE CASCADE
);
