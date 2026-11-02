-- 124 で個人情報に結び付け直す前に、すでに個人情報が消えた人の記録を消す。
DELETE h FROM ng_word_hits h
LEFT JOIN user_accounts a ON a.user_id = h.user_id
WHERE a.user_id IS NULL;
