-- 画像の表示領域をロード前に確保するための寸法。既存行は NULL のまま作成し、
-- cmd/backfill-media-dimensions で埋め戻す。動画・PDF など寸法を持たない
-- メディアは NULL のままになるため NOT NULL にはできない。
--
-- 列の位置を指定しないのは、末尾への追加なら MySQL 8.0.12 以降で
-- ALGORITHM=INSTANT が使えるため。AFTER で位置を指定すると 8.0.29 未満では
-- テーブル再構築に落ちる。
ALTER TABLE media
    ADD COLUMN width  INT NULL,
    ADD COLUMN height INT NULL;
