#!/bin/bash
# ============================================================
# run_ofo.sh — ofo 博客启动脚本
# 用法: ./run_ofo.sh [start|stop|restart]
# 无参数默认 start，自动杀掉旧进程 + flock 防并发
# ============================================================

set -euo pipefail

# ---------------------------- 配置区 ----------------------------
APP_PATH="${APP_PATH:-./ofo}"
LOCK_FILE="/tmp/ofo.lock"
LOG_FILE="/tmp/ofo.log"

# ---------------------------- 工具函数 ----------------------------
log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" | tee -a "${LOG_FILE}"
}

find_ofo_pid() {
    pgrep -f "${APP_PATH}" 2>/dev/null || true
}

# ---------------------------- start ----------------------------
do_start() {
    log "========== ofo 启动 =========="

    # 检查二进制
    if [ ! -f "${APP_PATH}" ]; then
        log "ERROR: 找不到 ${APP_PATH}，请先编译。"
        exit 1
    fi

    # 杀掉旧进程
    local old_pid
    old_pid=$(find_ofo_pid)
    if [ -n "${old_pid}" ]; then
        log "发现旧进程 PID: ${old_pid}，正在停止..."
        kill "${old_pid}" 2>/dev/null || true
        sleep 1
        # 如果还没死就强杀
        if kill -0 "${old_pid}" 2>/dev/null; then
            kill -9 "${old_pid}" 2>/dev/null || true
        fi
        log "旧进程已停止"
    fi

    # 获取文件锁
    exec 200>"${LOCK_FILE}"
    if ! flock -n 200; then
        log "ERROR: 另一个 ofo 启动进程正在运行 (锁: ${LOCK_FILE})"
        exit 1
    fi
    log "锁文件: ${LOCK_FILE}"

    # 启动
    log "启动: ${APP_PATH}"
    nohup "${APP_PATH}" >> "${LOG_FILE}" 2>&1 &
    local new_pid=$!
    sleep 1

    if kill -0 "${new_pid}" 2>/dev/null; then
        log "ofo 启动成功，PID: ${new_pid}"
    else
        log "ERROR: ofo 启动失败，查看日志: ${LOG_FILE}"
        exit 1
    fi
}

# ---------------------------- stop ----------------------------
do_stop() {
    log "========== ofo 停止 =========="
    local pid
    pid=$(find_ofo_pid)
    if [ -z "${pid}" ]; then
        log "没有正在运行的 ofo 进程"
        return
    fi
    log "停止 PID: ${pid}"
    kill "${pid}" 2>/dev/null || true
    sleep 1
    if kill -0 "${pid}" 2>/dev/null; then
        kill -9 "${pid}" 2>/dev/null || true
    fi
    log "ofo 已停止"
    rm -f "${LOCK_FILE}"
}

# ---------------------------- restart ----------------------------
do_restart() {
    do_stop
    sleep 1
    do_start
}

# ---------------------------- 入口 ----------------------------
case "${1:-start}" in
    start)   do_start ;;
    stop)    do_stop ;;
    restart) do_restart ;;
    status)
        pid=$(find_ofo_pid)
        if [ -n "${pid}" ]; then
            log "ofo 运行中，PID: ${pid}"
        else
            log "ofo 未运行"
        fi
        ;;
    *)
        echo "用法: $0 {start|stop|restart|status}"
        exit 1
        ;;
esac
