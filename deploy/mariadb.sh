#!/usr/bin/env bash
# ============================================================
# MariaDB 部署脚本 — Ubuntu 线上环境
# 用法: sudo bash mariadb.sh
# ============================================================
set -euo pipefail

# ---- 可配置项（按需修改）----
DB_NAME="${DB_NAME:-ofo}"
DB_USER="${DB_USER:-ofo}"
DB_PASS="${DB_PASS:-$(openssl rand -base64 24 | tr -d '/+=')}"   # 空则自动生成随机密码
ROOT_PASS="${ROOT_PASS:-}"                                        # 空则交互式设置
BIND_ADDRESS="${BIND_ADDRESS:-127.0.0.1}"                         # 仅本地访问；需要远程则改为 0.0.0.0

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log()  { echo -e "${GREEN}[INFO]${NC}  $*"; }
warn() { echo -e "${YELLOW}[WARN]${NC}  $*"; }
err()  { echo -e "${RED}[ERROR]${NC} $*"; exit 1; }

# ---- 检查是否 root ----
if [ "$(id -u)" -ne 0 ]; then
    err "请用 sudo 运行此脚本"
fi

# ---- Step 1: 安装 MariaDB ----
log "Step 1/5 — 安装 MariaDB"
if ! command -v mariadb &>/dev/null; then
    export DEBIAN_FRONTEND=noninteractive
    log "正在 apt-get update ..."
    apt-get update -qq
    log "正在 apt-get install mariadb-server ..."
    apt-get install -y mariadb-server mariadb-client
    log "MariaDB 安装完成"
else
    log "MariaDB 已安装，跳过"
fi

# ---- Step 2: 启动并设置开机自启 ----
log "Step 2/5 — 启动服务"
systemctl enable mariadb
systemctl start mariadb
log "MariaDB 服务已启动"

# ---- Step 3: 安全初始化 ----
log "Step 3/5 — 安全配置"

# 如果 root 尚未设密码，尝试无密码登录并设置
if [[ -z "$ROOT_PASS" ]]; then
    if mariadb -u root -e "SELECT 1" &>/dev/null; then
        ROOT_PASS=$(openssl rand -base64 18 | tr -d '/+=')
        mariadb -u root -e "ALTER USER 'root'@'localhost' IDENTIFIED BY '${ROOT_PASS}'; FLUSH PRIVILEGES;"
        log "root 密码已自动生成: ${ROOT_PASS}  (请妥善保存！)"
    else
        warn "root 已有密码保护，跳过"
    fi
else
    # 用户提供了密码，直接设置
    mariadb -u root -e "ALTER USER 'root'@'localhost' IDENTIFIED BY '${ROOT_PASS}'; FLUSH PRIVILEGES;" 2>/dev/null || true
    log "root 密码已设置为指定值"
fi

# 移除匿名用户、禁用远程 root、删除 test 库
mariadb -u root -p"${ROOT_PASS}" <<'EOSQL' 2>/dev/null || mariadb -u root <<'EOSQL' 2>/dev/null || true
DELETE FROM mysql.user WHERE User='';
DELETE FROM mysql.user WHERE User='root' AND Host NOT IN ('localhost', '127.0.0.1', '::1');
DROP DATABASE IF EXISTS test;
DELETE FROM mysql.db WHERE Db='test' OR Db='test\\_%';
FLUSH PRIVILEGES;
EOSQL
log "安全加固完成"

# ---- Step 4: 创建数据库 & 应用用户 ----
log "Step 4/5 — 创建数据库与应用账户"

CRED_FILE="/root/.ofo_mysql_credentials"
cat > "$CRED_FILE" <<EOF
# ofo 博客 MariaDB 凭据 — $(date)
DB_NAME=${DB_NAME}
DB_USER=${DB_USER}
DB_PASS=${DB_PASS}
EOF
chmod 600 "$CRED_FILE"

# 需要 root 密码来创建
AUTH=""
if [[ -n "${ROOT_PASS:-}" ]]; then
    AUTH="-u root -p${ROOT_PASS}"
else
    AUTH="-u root"
fi

mariadb $AUTH <<EOSQL
CREATE DATABASE IF NOT EXISTS \`${DB_NAME}\`
    CHARACTER SET utf8mb4
    COLLATE utf8mb4_unicode_ci;

DROP USER IF EXISTS '${DB_USER}'@'localhost';
CREATE USER '${DB_USER}'@'localhost' IDENTIFIED WITH mysql_native_password BY '${DB_PASS}';

GRANT ALL PRIVILEGES ON \`${DB_NAME}\`.* TO '${DB_USER}'@'localhost';

FLUSH PRIVILEGES;
EOSQL

log "数据库 ${DB_NAME} 已创建，用户 ${DB_USER} 已授权"
log "凭据已保存至 ${CRED_FILE}"

# ---- Step 5: 优化配置 ----
log "Step 5/5 — 写入性能优化配置"

CNF="/etc/mysql/mariadb.conf.d/99-ofo.cnf"
cat > "$CNF" <<'EOF'
[mysqld]
# === ofo 博客优化 ===
character-set-server  = utf8mb4
collation-server      = utf8mb4_unicode_ci
max_allowed_packet    = 64M

# InnoDB 缓冲池（根据服务器内存调整，建议设为可用内存的 50-70%）
# 1 GB 内存的 VPS 建议 256M
innodb_buffer_pool_size = 256M
innodb_log_file_size    = 64M
innodb_flush_log_at_trx_commit = 2
innodb_file_per_table   = 1

# 连接数
max_connections = 50

# 慢查询日志（调试用，生产环境可注释掉）
slow_query_log      = 1
slow_query_log_file = /var/log/mysql/mariadb-slow.log
long_query_time     = 2

[client]
default-character-set = utf8mb4
EOF

# 如果绑定地址需要修改
if [[ "$BIND_ADDRESS" != "127.0.0.1" ]]; then
    sed -i "s/^bind-address.*/bind-address = ${BIND_ADDRESS}/" /etc/mysql/mariadb.conf.d/50-server.cnf 2>/dev/null || true
    log "bind-address 已设为 ${BIND_ADDRESS}"
fi

systemctl restart mariadb
log "MariaDB 重启完成，配置已生效"

# ---- 输出结果 ----
echo ""
echo "============================================"
echo -e "  ${GREEN}MariaDB 部署完成${NC}"
echo "============================================"
echo "  数据库名 : ${DB_NAME}"
echo "  应用用户 : ${DB_USER}"
echo "  应用密码 : ${DB_PASS}"
echo "  root 密码: ${ROOT_PASS:-已有保护}"
echo "  凭据文件 : ${CRED_FILE}"
echo ""
echo "  .env 配置参考:"
echo "  ┌─────────────────────────────────────┐"
echo "  │ DB_HOST=127.0.0.1                   │"
echo "  │ DB_PORT=3306                        │"
echo "  │ DB_USER=${DB_USER}                │"
echo "  │ DB_PASSWORD=${DB_PASS}  │"
echo "  │ DB_NAME=${DB_NAME}                   │"
echo "  └─────────────────────────────────────┘"
echo ""
