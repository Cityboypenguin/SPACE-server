-- 授業内チャットの匿名表示（匿名NNN）をやめたので、ラベルの表を落とす。
--
-- 投稿者は messages / questions / answers / polls の側が実ユーザーIDで持っているので、
-- この表を消しても「誰が書いたか」は失われない。採番カウンタ（067）は 075 で落とす。
-- マイグレーションは1ファイル1文なので表ごとに分けてある。
DROP TABLE IF EXISTS room_anonymous_identities;
