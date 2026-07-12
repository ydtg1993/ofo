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

# 等待旧进程被 kill 的最大秒数
KILL_TIMEOUT=5

# ---------------------------- 功能区 ----------------------------
log() {
    if [ -n "${LOG_FILE}" ]; then
        echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" | tee -a "${LOG_FILE}"
    else
        echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"
    fi
}

# 根据进程名杀掉所有匹配的进程
kill_by_name() {
    local pids
    # 查找进程名包含 PROC_NAME 的进程，排除 grep 自身和当前脚本
    pids=$(pgrep -f "${PROC_NAME}" 2>/dev/null || true)

    if [ -z "${pids}" ]; then
        log "没有找到正在运行的 ${PROC_NAME} 进程。"
        return 0
    fi

    log "找到 ${PROC_NAME} 进程，PID: ${pids}"

    for pid in ${pids}; do
        log "正在终止进程 ${pid}..."
        kill "${pid}" 2>/dev/null || true
    done

    # 等待所有进程退出
    local waited=0
    while [ ${waited} -lt ${KILL_TIMEOUT} ]; do
        local still_alive
        still_alive=$(pgrep -f "${PROC_NAME}" 2>/dev/null || true)
        if [ -z "${still_alive}" ]; then
            log "所有 ${PROC_NAME} 进程已退出。"
            return 0
        fi
        sleep 1
        waited=$((waited + 1))
        log "等待进程退出... (${waited}/${KILL_TIMEOUT})"
    done

    # 还没死的强杀
    local remaining
    remaining=$(pgrep -f "${PROC_NAME}" 2>/dev/null || true)
    if [ -n "${remaining}" ]; then
        log "部分进程未响应 SIGTERM，强制 kill -9..."
        for pid in ${remaining}; do
            kill -9 "${pid}" 2>/dev/null || true
        done
        sleep 1
    fi

    # 最终确认
    remaining=$(pgrep -f "${PROC_NAME}" 2>/dev/null || true)
    if [ -n "${remaining}" ]; then
        log "ERROR: 无法杀掉进程 ${remaining}，请手动处理。"
        return 1
    fi

    log "所有 ${PROC_NAME} 进程已强制终止。"
}

# 根据锁文件杀掉旧实例（兜底方案）
kill_by_lock() {
    if [ ! -f "${LOCK_FILE}" ]; then
        return 0
    fi

    local old_pid
    old_pid=$(cat "${LOCK_FILE}" 2>/dev/null || true)

    if [ -z "${old_pid}" ]; then
        rm -f "${LOCK_FILE}"
        return 0
    fi

    if kill -0 "${old_pid}" 2>/dev/null; then
        log "锁文件中发现旧 PID ${old_pid}，终止它..."
        kill "${old_pid}" 2>/dev/null || true
        sleep 1
        kill -9 "${old_pid}" 2>/dev/null || true
    fi

    rm -f "${LOCK_FILE}"
}

cleanup() {
    log "脚本退出，清理锁文件。"
    rm -f "${LOCK_FILE}"
    exit 0
}

# ---------------------------- 主流程 ----------------------------
main() {
    log "========== ofo 单实例启动 =========="

    # 1. 先通过进程名杀掉所有旧 ofo 进程
    kill_by_name

    # 2. 再用锁文件兜底清理
    kill_by_lock

    # 3. 写入当前 shell 的 PID
    echo $$ > "${LOCK_FILE}"
    log "脚本 PID: $$，锁文件: ${LOCK_FILE}"

    # 4. 注册退出清理
    trap cleanup EXIT INT TERM

    # 5. 检查程序是否存在
    if [ ! -f "${APP_PATH}" ]; then
        log "ERROR: 找不到程序 ${APP_PATH}，请先编译或修改 APP_PATH。"
        rm -f "${LOCK_FILE}"
        exit 1
    fi

    # 6. 启动 ofo
    log "启动: ${APP_PATH}"
    "${APP_PATH}" &
    local app_pid=$!
    log "ofo 进程 PID: ${app_pid}"

    # 7. 等待程序退出
    wait "${app_pid}"
    local exit_code=$?
    log "ofo 退出，exit code: ${exit_code}"

    # 清理
    rm -f "${LOCK_FILE}"
    exit ${exit_code}
}

main "$@"
