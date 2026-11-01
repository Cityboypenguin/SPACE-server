-- メッセージの通報で、通報を受けた時点の内容を残すための列。
--
-- JSON を messagecrypto で暗号化した文字列を入れる（messages.content と同じ扱い）。
-- 暗号化すると本文より長くなるので、TEXT の messages.content が上限まで使われていても
-- 収まるよう MEDIUMTEXT にしてある。
--
-- 番号が 074 ではなく 109 なのは、dev_new を合流させる Develop が 108 まで使っているため。
-- 074 だと Azure / Develop の 074 と重複し、合流後の起動時にマイグレーションが失敗する。
ALTER TABLE user_reports ADD COLUMN message_snapshot MEDIUMTEXT NULL AFTER content;
