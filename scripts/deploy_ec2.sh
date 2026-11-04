#!/bin/bash
set -euo pipefail
cd /opt/space-server

REGION="${AWS_REGION:-ap-northeast-1}"
ECR_IMAGE="${ECR_IMAGE:?ECR_IMAGE must be set}"
ECR_REGISTRY="${ECR_IMAGE%%/*}"

get_param() {
  aws ssm get-parameter --name "$1" --with-decryption --region "$REGION" --query 'Parameter.Value' --output text
}

DB_PASSWORD="$(get_param /space/db-password)"
JWT_SECRET="$(get_param /space/jwt-secret)"
MESSAGE_ENCRYPTION_KEY="$(get_param /space/message-encryption-key)"
ACTIVITY_ARCHIVE_HMAC_KEY="$(get_param /space/activity-archive-hmac-key)"
SMTP_PASSWORD="$(get_param /space/smtp-password)"
MINIO_ACCESS_KEY="$(get_param /space/aws-access-key)"
MINIO_SECRET_KEY="$(get_param /space/aws-secret-key)"
OPAQUE_ID_SECRET="$(get_param /space/opaque-id-secret)"

cat > .env.prod <<EOF
ECR_IMAGE=${ECR_IMAGE}
DB_USER=root
DB_PASSWORD=${DB_PASSWORD}
DB_NAME=space
JWT_SECRET=${JWT_SECRET}
JWT_EXPIRATION_MINUTES=60
JWT_REFRESH_EXPIRATION_MINUTES=43200
MESSAGE_ENCRYPTION_KEY=${MESSAGE_ENCRYPTION_KEY}
ACTIVITY_ARCHIVE_HMAC_KEY=${ACTIVITY_ARCHIVE_HMAC_KEY}
OPAQUE_ID_SECRET=${OPAQUE_ID_SECRET}
INIT_ADMIN_NAME=Initial_Admin
INIT_ADMIN_EMAIL=kosugi.pjt@gmail.com
MINIO_ACCESS_KEY=${MINIO_ACCESS_KEY}
MINIO_SECRET_KEY=${MINIO_SECRET_KEY}
MINIO_BUCKET=space-avatars
MINIO_PRIVATE_BUCKET=space-activity-archives
AWS_REGION=${REGION}
MINIO_PUBLIC_ENDPOINT=https://s3.${REGION}.amazonaws.com
ALLOWED_ORIGINS=https://d2ihoct532mxwc.cloudfront.net,https://senshu-universe.com,https://www.senshu-universe.com
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USERNAME=kosugi.pjt@gmail.com
SMTP_PASSWORD=${SMTP_PASSWORD}
SMTP_FROM=kosugi.pjt@gmail.com
EOF
chmod 600 .env.prod

# 起動時にマイグレーションが走るので、DB を戻す手段はこのダンプだけになる。取れなければ配信しない。
if [ "$(docker inspect -f '{{.State.Running}}' space-server-mysql-1 2>/dev/null)" = "true" ]; then
  install -d -m 700 backups
  BACKUP="backups/pre-deploy-$(date +%Y%m%d-%H%M%S).sql.gz"
  docker exec space-server-mysql-1 sh -c 'exec mysqldump --single-transaction --quick --default-character-set=utf8mb4 --set-gtid-purged=OFF --no-tablespaces --routines --triggers --hex-blob -uroot -p"$MYSQL_ROOT_PASSWORD" "$MYSQL_DATABASE"' | gzip > "$BACKUP.tmp"
  mv "$BACKUP.tmp" "$BACKUP"
  chmod 600 "$BACKUP"
  ls -1t backups/pre-deploy-*.sql.gz | tail -n +6 | xargs -r rm -f
fi

aws ecr get-login-password --region "$REGION" | docker login --username AWS --password-stdin "$ECR_REGISTRY"

docker compose -f compose.ec2.yaml --env-file .env.prod pull
docker compose -f compose.ec2.yaml --env-file .env.prod up -d
docker image prune -f
