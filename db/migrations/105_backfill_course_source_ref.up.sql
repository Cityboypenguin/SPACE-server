-- 'senshu:{年度}:{学期}:{講義コード}:{曜日}:{時限}' の4番目（講義コード）を source_ref へ写す。
UPDATE courses
SET source_ref = SUBSTRING_INDEX(SUBSTRING_INDEX(dedup_key, ':', 4), ':', -1)
WHERE source = 'senshu';
