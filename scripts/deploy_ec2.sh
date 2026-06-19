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
SMTP_PASSWORD="$(get_param /space/smtp-password)"
MINIO_ACCESS_KEY="$(get_param /space/aws-access-key)"
MINIO_SECRET_KEY="$(get_param /space/aws-secret-key)"

cat > .env.prod <<EOF
ECR_IMAGE=${ECR_IMAGE}
DB_USER=root
DB_PASSWORD=${DB_PASSWORD}
DB_NAME=space
JWT_SECRET=${JWT_SECRET}
JWT_EXPIRATION_MINUTES=60
JWT_REFRESH_EXPIRATION_MINUTES=43200
MESSAGE_ENCRYPTION_KEY=${MESSAGE_ENCRYPTION_KEY}
INIT_ADMIN_NAME=Initial_Admin
INIT_ADMIN_EMAIL=kosugi.pjt@gmail.com
MINIO_ACCESS_KEY=${MINIO_ACCESS_KEY}
MINIO_SECRET_KEY=${MINIO_SECRET_KEY}
MINIO_BUCKET=space-avatars
AWS_REGION=${REGION}
MINIO_PUBLIC_ENDPOINT=https://s3.${REGION}.amazonaws.com
ALLOWED_ORIGINS=https://d2ihoct532mxwc.cloudfront.net
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USERNAME=kosugi.pjt@gmail.com
SMTP_PASSWORD=${SMTP_PASSWORD}
SMTP_FROM=kosugi.pjt@gmail.com
EOF
chmod 600 .env.prod

aws ecr get-login-password --region "$REGION" | docker login --username AWS --password-stdin "$ECR_REGISTRY"

docker compose -f compose.ec2.yaml --env-file .env.prod pull
docker compose -f compose.ec2.yaml --env-file .env.prod up -d
docker image prune -f
