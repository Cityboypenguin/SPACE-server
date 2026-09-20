-- 068 で足した room_users.last_read_message_id を既存行に埋める（段階移行の後始末）。
--
-- 068 はバックフィルしていないので、既存ユーザーの行は last_read_message_id が NULL の
-- まま。その間は infra/mysql/unread_origin.go のフォールバックで last_read_at（Unix 秒）
-- を起点に数えるため、「既読にした直後の同じ秒に届いたメッセージが未読から漏れる」という
-- 秒解像度の競合が残り続ける。次に既読を打てば ID へ移るとはいえ、長く開いていない
-- ルームでは旧挙動が残ったままなので、ここで一括して ID へ移す。
--
-- 各行の位置は「そのルームで created_at <= last_read_at を満たす最大の message id」。
--
-- 同じ秒の扱いについて（< ではなく <= を使う理由）:
-- 移行前の未読判定は created_at > last_read_at だった。つまり既読時刻とちょうど同じ秒に
-- 作られたメッセージは「未読ではない」側に数えられていた。<= で埋めるとその行までが
-- 既読位置に含まれ、移行後の判定 id > last_read_message_id でもやはり未読にならない。
-- 逆に < で埋めると同じ秒の行が未読へ転じ、移行しただけでバッジの数字が増える。
-- <= にしておけば、このバックフィルによって未読が増えることも減ることもない
-- （＝利用者から見て何も起きない）。競合が解消されるのはここから先の新着だけ。
--
-- 論理削除されたメッセージも MAX の対象に含める。位置は「ここまで見た」というしおりで、
-- 削除済みを飛ばして小さい id を書くと、その行より後ろの既読が巻き戻る（既読の考え方は
-- repository/read_position.go、位置の決め方は MessageReadModel.GetLatestMessageID と同じ）。
--
-- last_read_at IS NULL の行（一度も読んでいない）は対象外。起点が無いのが正しい状態で、
-- ここに 0 や NULL 以外を書くと「全件未読」という現行の意味が壊れる。
-- last_read_message_id が既に入っている行も対象外（既に ID 移行済み）。
--
-- course_room_reads には同じ手当てをしない。あちらは 066 で新設する表（未リリース）で、
-- 最初から last_read_message_id を持って作られ、既読を打つたびに ID が入る。つまり
-- 「列が無かった頃の行」が本番に存在しない。埋める対象が無いので UPDATE は不要。
UPDATE room_users
SET last_read_message_id = (
        SELECT MAX(m.id)
        FROM messages m
        WHERE m.room_id = room_users.room_id
          AND m.created_at <= room_users.last_read_at
    )
WHERE room_users.last_read_message_id IS NULL
  AND room_users.last_read_at IS NOT NULL;
