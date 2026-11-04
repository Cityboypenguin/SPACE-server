-- 通報を受け付けたときにお知らせメールを送る宛先。管理画面から追加・削除する。
CREATE TABLE IF NOT EXISTS report_notification_recipients (
    id         BIGINT       NOT NULL AUTO_INCREMENT,
    email      VARCHAR(255) NOT NULL,
    created_at BIGINT       NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uq_report_notification_recipients_email (email)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
