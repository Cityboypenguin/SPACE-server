-- シラバス同期（取り込みを「足すだけ」から「大学の登録内容に合わせる」へ）のための列。
--
-- source / source_ref は取り込み元で同じ授業を見つけるためだけに使う。アプリの中で
-- 授業を指すのは今まで通り courses.id で、時間割・ルーム・API はこちらを参照する。
--   source      : 'senshu'（シラバスから取り込んだ）/ 'manual'（管理者が手で作った）
--   source_ref  : 取り込み元での識別子（専修大学なら講義コード）。manual は NULL
--   source_name : 校舎サフィックス「（生田・…）」を付ける前の授業名。名前が変わったかの
--                 比較に使う（付けた後の名前で比べると、サフィックスの付け外しを
--                 名前の変更と取り違える）。既存行は不明なので NULL のまま始める
-- discontinued_at はシラバスから消えた日時。行は消さずに印だけ付ける（時間割・ルーム・
-- メッセージを残すため）。NULL なら現行の授業。
--
-- dedup_key は残す。新しい列で動くことを確かめてから、別のリリースで落とす。
ALTER TABLE courses
    ADD COLUMN source          VARCHAR(20)  NOT NULL DEFAULT 'senshu' AFTER dedup_key,
    ADD COLUMN source_ref      VARCHAR(64)  NULL AFTER source,
    ADD COLUMN source_name     VARCHAR(255) NULL AFTER source_ref,
    ADD COLUMN discontinued_at BIGINT       NULL AFTER source_name,
    ADD INDEX idx_courses_year_source_ref (year, source, source_ref);
