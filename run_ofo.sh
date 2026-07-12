#!/bin/bash
# ============================================================
# run_singleton.sh — 单实例启动脚本（ofo 专用）
# 重复启动时会自动杀掉之前正在运行的 ofo 进程
# ============================================================

set -e

# ---------------------------- 配置区 ----------------------------
# 进程名称（用于查找和匹配）
PROC_NAME="ofo"

# 程序路径（根据实际部署路径修改）
APP_PATH="./ofo"
# APP_PATH="/opt/ofo/ofo"           # 生产环境示例

# 锁文件路径
LOCK_FILE="/tmp/ofo.lock"

# 日志文件（注释掉则不写日志）
LOG_FILE="/tmp/ofo.log"

# ---------------------------- 功能区 ----------------------------
log() {
    if [ -n "${LOG_FILE}" ]; then
        echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" | tee -a "${LOG_FILE}"
    else
        echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"
    fi
}

cleanup() {
    log "脚本退出，清理锁文件。"
    rm -f "${LOCK_FILE}"
    exit 0
}

# ---------------------------- 主流程 ----------------------------
main() {
    log "========== ofo 单实例启动 =========="

    # 写入当前 shell 的 PID
    echo $$ > "${LOCK_FILE}"
    log "脚本 PID: $$，锁文件: ${LOCK_FILE}"

    # 注册退出清理
    trap cleanup EXIT INT TERM

    # 检查程序是否存在
    if [ ! -f "${APP_PATH}" ]; then
        log "ERROR: 找不到程序 ${APP_PATH}，请先编译或修改 APP_PATH。"
        rm -f "${LOCK_FILE}"
        exit 1
    fi

    # 启动 ofo
    log "启动: ${APP_PATH}"
    exec "${APP_PATH}"
}

main "$@"
