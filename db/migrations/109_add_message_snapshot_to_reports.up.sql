-- メッセージの通報で、通報を受けた時点の内容を残すための列。
--
-- JSON を messagecrypto で暗号化した文字列を入れる（messages.content と同じ扱い）。
-- 暗号化すると本文より長くなるので、TEXT の messages.content が上限まで使われていても
-- 収まるよう MEDIUMTEXT にしてある。
--
-- 番号は Develop の 108 に続く。
ALTER TABLE user_reports ADD COLUMN message_snapshot MEDIUMTEXT NULL AFTER content;
