#!/bin/bash
# =============================================================================
# certbot-renew.sh — Let's Encrypt 证书自动续期
# 用法: certbot renew + nginx reload
# cron: 0 3 * * * /usr/local/bin/certbot-renew.sh >> /var/log/certbot/renew.log 2>&1
# =============================================================================

set -e

LOG_FILE="/var/log/certbot/renew.log"
LOCK_FILE="/var/run/certbot-renew.lock"

# 防并发
exec 200>"$LOCK_FILE"
if ! flock -n 200; then
    echo "[$(date '+%F %T')] 另一个续期进程正在运行，跳过"
    exit 0
fi

log() {
    echo "[$(date '+%F %T')] $1" | tee -a "$LOG_FILE"
}

mkdir -p "$(dirname "$LOG_FILE")"

log "开始证书续期..."

# certbot renew 只会续期距过期 <30 天的证书，不会每次跑都签发
# --webroot 方式：利用 nginx 已有的 .well-known/acme-challenge 目录验证，不用停 nginx
certbot renew \
    --webroot \
    --webroot-path /var/www/certbot \
    --quiet \
    --post-hook "nginx -s reload" 2>&1 | tee -a "$LOG_FILE"

if [ $? -eq 0 ]; then
    log "续期检查完成（如有续期会自动 reload nginx）"
else
    log "续期失败，请检查"
    exit 1
fi
