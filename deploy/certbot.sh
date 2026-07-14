#!/usr/bin/env bash
# ============================================================
# Certbot SSL 证书部署脚本 — Ubuntu 线上环境
# 用法: sudo bash certbot.sh
# 前提: nginx 已安装且域名 DNS 已指向本服务器
# ============================================================
set -euo pipefail

# ---- 可配置项 ----
DOMAIN="${DOMAIN:-qiofo.com}"                                     # 主域名
EMAIL="${EMAIL:-admin@qiofo.com}"                                 # 证书过期通知邮箱
WEBROOT="${WEBROOT:-/var/www/certbot}"                            # ACME challenge 目录
NGINX_CONF="${NGINX_CONF:-/etc/nginx/sites-enabled/qiofo.com.conf}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log()  { echo -e "${GREEN}[INFO]${NC}  $*"; }
warn() { echo -e "${YELLOW}[WARN]${NC}  $*"; }
err()  { echo -e "${RED}[ERROR]${NC} $*"; exit 1; }

# ---- 检查 root ----
if [ "$(id -u)" -ne 0 ]; then
    err "请用 sudo 运行此脚本"
fi

# ============================================================
# Step 1: 安装 Certbot
# ============================================================
log "Step 1/6 — 安装 Certbot"

if command -v certbot &>/dev/null; then
    log "Certbot 已安装: $(certbot --version | head -1)"
else
    # Ubuntu 24.04+ 推荐 snap；旧版用 apt
    if command -v snap &>/dev/null && snap list core &>/dev/null 2>&1; then
        log "通过 snap 安装 ..."
        snap install --classic certbot
        ln -sf /snap/bin/certbot /usr/bin/certbot
    else
        log "通过 apt 安装 ..."
        apt-get update -qq
        apt-get install -y certbot
    fi
    log "Certbot 安装完成"
fi

# ============================================================
# Step 2: 准备 ACME challenge 目录
# ============================================================
log "Step 2/6 — 准备验证目录"
mkdir -p "$WEBROOT"
chmod 755 "$WEBROOT"
log "ACME 验证目录: $WEBROOT"

# ============================================================
# Step 3: 确保 nginx 已配置 ACME challenge（未配置则补写）
# ============================================================
log "Step 3/6 — 检查 nginx ACME 配置"

# 检查 nginx 80 端口是否已配 /.well-known/acme-challenge/
if [ -f "$NGINX_CONF" ]; then
    if ! grep -q "acme-challenge" "$NGINX_CONF"; then
        warn "nginx 配置中未找到 ACME challenge 路径，请手动添加："
        warn ""
        warn "  location ^~ /.well-known/acme-challenge/ {"
        warn "      root $WEBROOT;"
        warn "  }"
        warn ""
        warn "放在 80 端口的 server {} 块内"
    else
        log "ACME challenge 配置已存在"
    fi
else
    warn "未找到 nginx 配置文件: $NGINX_CONF"
    warn "请确保 80 端口已配置 ACME challenge 路径"
fi

# 确保 nginx 在运行
if systemctl is-active --quiet nginx; then
    nginx -t && nginx -s reload
else
    warn "nginx 未运行，请先启动 nginx"
    exit 1
fi

# ============================================================
# Step 4: 申请证书（webroot 模式）
# ============================================================
log "Step 4/6 — 申请 SSL 证书"

# webroot 模式不需要暂停 nginx，最适合在线服务器
certbot certonly \
    --webroot \
    --webroot-path "$WEBROOT" \
    --non-interactive \
    --agree-tos \
    --email "$EMAIL" \
    -d "$DOMAIN" \
    -d "www.$DOMAIN" \
    --keep-until-expiring \
    --expand

log "证书申请成功"

# ============================================================
# Step 5: 自动续期 + nginx reload 钩子
# ============================================================
log "Step 5/6 — 配置自动续期"

# 写一个 reload-hook（certbot 续期成功后自动重载 nginx）
HOOK_SCRIPT="/etc/letsencrypt/renewal-hooks/deploy/nginx-reload.sh"
mkdir -p "$(dirname "$HOOK_SCRIPT")"
cat > "$HOOK_SCRIPT" <<'HOOKEOF'
#!/usr/bin/env bash
# certbot renew 成功后自动重载 nginx
set -e
echo "[certbot-hook] $(date -Iseconds) — reloading nginx"
systemctl reload nginx
HOOKEOF
chmod +x "$HOOK_SCRIPT"

# 续期 crontab（每天凌晨 3 点检查，实际仅过期前 30 天内才会续）
CRON_JOB="0 3 * * * /usr/bin/certbot renew --quiet --deploy-hook $HOOK_SCRIPT"
if crontab -l 2>/dev/null | grep -qF "certbot renew"; then
    log "续期 crontab 已存在，跳过"
else
    (crontab -l 2>/dev/null || true; echo "$CRON_JOB") | crontab -
    log "续期 crontab 已添加（每日 3:00 检查）"
fi

# 也试试 systemd timer（新版 certbot 自带）
if command -v systemctl &>/dev/null && systemctl list-timers certbot.timer &>/dev/null 2>&1; then
    systemctl enable certbot.timer --now
    log "certbot systemd timer 已启用"
fi

# ============================================================
# Step 6: 生成 DH 参数（如果缺失）
# ============================================================
log "Step 6/6 — 生成 DH 参数"

DH_FILE="/etc/nginx/ssl/dhparam.pem"
if [ -f "$DH_FILE" ]; then
    log "DH 参数已存在，跳过"
else
    mkdir -p "$(dirname "$DH_FILE")"
    log "正在生成 dhparam.pem（2048 位，低配约需 30 秒~1 分钟）..."
    openssl dhparam -out "$DH_FILE" 2048
    log "DH 参数生成完毕"
fi

# ============================================================
# 收尾验证
# ============================================================
echo ""
echo "============================================"
echo -e "  ${GREEN}SSL 证书部署完成${NC}"
echo "============================================"
echo "  域名      : $DOMAIN, www.$DOMAIN"
echo "  证书目录  : /etc/letsencrypt/live/$DOMAIN/"
echo "  验证模式  : webroot ($WEBROOT)"
echo "  续期方式  : crontab (每日 3:00) + deploy-hook"
echo "  通知邮箱  : $EMAIL"
echo ""

# 测试续期
log "运行 dry-run 续期测试..."
if certbot renew --dry-run --quiet; then
    log "续期测试通过 ✓"
else
    warn "续期测试失败，请检查 nginx 80 端口是否可从外网访问"
fi

# 检查证书状态
echo ""
certbot certificates 2>/dev/null || true
