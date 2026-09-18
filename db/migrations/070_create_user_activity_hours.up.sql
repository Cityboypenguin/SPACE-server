-- ユーザーの活動を「JST の1時間」単位で記録する表。
--
-- 何のために要るか:
-- 管理画面の時間別グラフ（activeUsers）は users.last_active_at を時刻で
-- GROUP BY して数えていた。last_active_at はユーザーごとに1値（最後の活動時刻）
-- しか持たないので、10時と14時に活動した人は14時のスロットにしか現れず、
-- 過去の時間帯ほど人数が実際より少なく出ていた。時間解像度の活動履歴が無いと
-- この数字は原理的に直せないので、履歴を持つ表を足す。
--
-- なぜ user_activity_dates を時間解像度に拡張しなかったか（採らなかった案）:
-- あちらは migration 043 で本番に適用済み・データありなので、主キーを
-- (user_id, activity_date) から (user_id, activity_date, activity_hour) へ
-- 広げる ALTER（表の再構築）と、時刻を持たない既存行のための番兵値
-- （主キー列は NULL 不可なので -1 のような値）が要る。さらに down で主キーを
-- 戻すには「同じ日の複数行を1行に潰す DELETE」→「主キー付け替え」と最低でも
-- 2文が必要で、この repo の「1ファイル1ステートメント」（DSN に
-- multiStatements が無い）と両立しない。本番データを触らずに済み、up/down が
-- どちらも1文で書けるこちらを採った。
--
-- 表が増えること（活動情報の重複）について:
-- この表は user_activity_dates の情報を完全に含む（DATE(activity_hour) が活動日）。
-- 日次の集計をこの表から導出できるようになった時点で user_activity_dates は
-- 不要になる。切り替えの条件は「この表が日次グラフの最大範囲ぶんの履歴を
-- 持つこと」だけで、その後 043 の表を落とせば重複は解消する。
-- それまでは書き込み側（internal/middleware/user_activity.go）が両方へ書く。
--
-- 行数の増え方と保持期間:
-- 1ユーザー1日あたり最大24行（実際は「活動した時間帯の数」ぶんで、平均3〜5行）。
-- DAU 1,000 人・1人あたり4時間帯なら 4,000行/日 ≒ 146万行/年、
-- InnoDB の主キー+索引込みで年 100〜150MB 程度。DAU 10,000 人なら10倍。
-- 単調増加するので保持期間を決める必要があるが、何日残すかは運用要件なので
-- ここでは決めない（削除処理も入れていない）。user_activity_dates /
-- user_session_summaries / page_view_stats も同じく無期限なので、
-- まとめて決めて1箇所で消すのが筋。
--
-- activity_hour は JST の「時」の始まり（例 2026-09-18 14:00:00）を
-- そのまま入れる。user_activity_dates.activity_date が JST の日付を
-- そのまま入れているのと同じ流儀で、読み出し側で UTC→JST の補正
-- （jstOffsetSec）が要らない。補正が要るのは created_at / last_active_at の
-- ような Unix 秒の列だけ。
--
-- 外部キーは張らない（user_activity_dates と同じ）。書き込みはリクエストの
-- 応答経路の外（async.Runner）から INSERT IGNORE で撃つので、参照先の消滅で
-- 書き込みが失敗する形にしたくない。退会したユーザーの行が残るが、集計は
-- 人数を数えるだけなので影響しない。
CREATE TABLE user_activity_hours (
    user_id       BIGINT   NOT NULL,
    activity_hour DATETIME NOT NULL, /*JST の「時」の始まり。分・秒は常に 0*/
    PRIMARY KEY (user_id, activity_hour), /*同じ人・同じ時間帯は1行（INSERT IGNORE で重複を捨てる）*/
    /*時間別グラフは activity_hour の範囲で絞って人数を数える。InnoDB の
      二次索引は主キー列を含むので、この索引だけで user_id まで読める。*/
    INDEX idx_activity_hour (activity_hour)
) ENGINE=InnoDB;
