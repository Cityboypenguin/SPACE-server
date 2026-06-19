WITH RECURSIVE visible_descendants AS (
    SELECT parent_id AS ancestor_id, id AS descendant_id
    FROM posts
    WHERE parent_id IS NOT NULL AND deleted_at IS NULL

    UNION ALL

    SELECT vd.ancestor_id, p.id
    FROM visible_descendants vd
    JOIN posts p ON p.parent_id = vd.descendant_id
    WHERE p.deleted_at IS NULL
),
reply_counts AS (
    SELECT ancestor_id, COUNT(*) AS reply_count
    FROM visible_descendants
    GROUP BY ancestor_id
)
UPDATE posts p
LEFT JOIN reply_counts rc ON rc.ancestor_id = p.id
SET p.reply_count = COALESCE(rc.reply_count, 0);
