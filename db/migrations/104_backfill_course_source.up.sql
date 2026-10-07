-- 既存行の取り込み元を dedup_key の接頭辞から埋める。
-- どちらでもない形の dedup_key は同期の対象にしない（'unknown' は照合にも廃止にも使わない）。
UPDATE courses
SET source = CASE
    WHEN dedup_key LIKE 'senshu:%' THEN 'senshu'
    WHEN dedup_key LIKE 'manual:%' THEN 'manual'
    ELSE 'unknown'
END;
