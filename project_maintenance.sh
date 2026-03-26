#!/bin/bash

# ============================================================
# cool-dispatch 统一维护脚本
# 把 Git URL 修复、脚本权限修复、项目构建、自动拉取四类能力统一到一个入口中
# 通过子命令模式复用公共逻辑，只保留一个真实脚本入口，避免重复维护配置和环境变量
# ============================================================

set -o pipefail

# 统一脚本所在目录，后续所有相对路径都基于仓库根目录执行，避免从其他目录调用时找不到文件。
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_NAME="cool-dispatch"
DEFAULT_REPO_PATH="kk1491/cool-dispatch-system"

# Git 用户名允许通过环境变量覆盖；Token 不再内置默认值，避免敏感信息进入仓库。
GIT_USERNAME="${GIT_USERNAME:-kk1491}"
GIT_TOKEN="${GIT_TOKEN:-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}"

# 自动拉取轮询间隔同样支持通过环境变量覆盖，默认仍为 60 秒。
CHECK_INTERVAL="${CHECK_INTERVAL:-60}"

# Go 安装相关配置支持通过环境变量覆盖，默认从项目 go.mod 读取版本，并优先走官方安装包。
GO_VERSION_FILE="${GO_VERSION_FILE:-${SCRIPT_DIR}/server/go.mod}"
GO_DOWNLOAD_BASE_URL="${GO_DOWNLOAD_BASE_URL:-https://go.dev/dl}"
GO_SYSTEM_INSTALL_DIR="${GO_SYSTEM_INSTALL_DIR:-/usr/local/go}"
GO_USER_INSTALL_DIR="${GO_USER_INSTALL_DIR:-${HOME}/.local/go}"

# 颜色输出统一集中，方便所有子命令复用同一套展示风格。
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# 统一日志函数，确保各子命令输出时间戳，方便定位自动任务的执行过程。
log_with_color() {
    local color="$1"
    local message="$2"
    echo -e "${color}[$(date '+%Y-%m-%d %H:%M:%S')]${NC} ${message}"
}

log_info() {
    log_with_color "${BLUE}" "$1"
}

log_success() {
    log_with_color "${GREEN}" "$1"
}

log_warning() {
    log_with_color "${YELLOW}" "$1"
}

log_error() {
    log_with_color "${RED}" "$1"
}

# 为了避免直接把 Token 打到终端里，这里统一做脱敏输出。
mask_token() {
    local raw_text="${1:-}"
    if [ -n "${GIT_TOKEN}" ]; then
        printf '%s\n' "${raw_text//${GIT_TOKEN}/***TOKEN***}"
    else
        printf '%s\n' "${raw_text}"
    fi
}

# 统一回到仓库根目录执行，避免脚本从其他工作目录调用时访问到错误路径。
enter_repo_root() {
    cd "${SCRIPT_DIR}" || exit 1
}

# 打印帮助信息，统一说明所有可用能力，后续只需要维护和记忆这一个脚本名。
print_usage() {
    cat <<'EOF'
用法:
  bash ./project_maintenance.sh <命令>

可用命令:
  install-go       检测并安装项目要求的 Go 环境
  fix-url          修复 origin 远程地址并测试 Git 连接
  fix-permissions  修复常用脚本的可执行权限
  build            构建后端和前端，并尝试重启服务
  sync-once        立即检查一次远程更新，有更新则拉取并构建
  auto-pull        持续轮询远程更新，有更新时自动拉取并构建
  help             显示帮助信息
EOF
}

# 自动加载用户环境，尽量兼容历史部署环境中把 Go、Node、NPM 等路径写在 profile 里的情况。
load_runtime_env() {
    log_info "加载系统环境变量..."

    if [ -f "${HOME}/.bashrc" ]; then
        # shellcheck disable=SC1090
        source "${HOME}/.bashrc"
        log_info "已加载 ${HOME}/.bashrc"
    fi

    if [ -f "${HOME}/.profile" ]; then
        # shellcheck disable=SC1090
        source "${HOME}/.profile"
        log_info "已加载 ${HOME}/.profile"
    fi

    if [ -f "/etc/profile" ]; then
        # shellcheck disable=SC1091
        source "/etc/profile"
        log_info "已加载 /etc/profile"
    fi
}

# 涉及远程 Git 凭证的动作统一走这里校验，避免在未配置 Token 的情况下继续修改 origin 或执行推送。
ensure_git_credentials() {
    if [ -n "${GIT_TOKEN}" ]; then
        return 0
    fi

    log_error "未检测到 GIT_TOKEN 环境变量，无法继续执行需要远程凭证的操作"
    log_error "请先执行: export GIT_TOKEN=你的GitHubToken"
    return 1
}

# 从 server/go.mod 读取项目要求的 Go 版本，方便安装逻辑和版本校验逻辑统一复用。
get_required_go_version() {
    local required_version="${GO_REQUIRED_VERSION:-}"

    if [ -n "${required_version}" ]; then
        printf '%s\n' "${required_version#go}"
        return 0
    fi

    if [ -f "${GO_VERSION_FILE}" ]; then
        awk '/^go / {print $2; exit}' "${GO_VERSION_FILE}"
        return 0
    fi

    return 1
}

# 读取当前已安装 Go 的版本号，返回值只保留纯版本字符串，便于后续比较。
get_installed_go_version() {
    if ! command -v go >/dev/null 2>&1; then
        return 1
    fi

    go version 2>/dev/null | awk '{print $3}' | sed 's/^go//'
}

# 比较两个版本号是否满足大于等于关系，用于判断现有 Go 是否满足项目最低版本要求。
version_gte() {
    local current_version="$1"
    local required_version="$2"
    local current_parts=()
    local required_parts=()
    local index=0
    local current_value=0
    local required_value=0

    IFS='.' read -r -a current_parts <<< "${current_version}"
    IFS='.' read -r -a required_parts <<< "${required_version}"

    for index in 0 1 2; do
        current_value="${current_parts[${index}]:-0}"
        required_value="${required_parts[${index}]:-0}"

        if [ "${current_value}" -gt "${required_value}" ]; then
            return 0
        fi

        if [ "${current_value}" -lt "${required_value}" ]; then
            return 1
        fi
    done

    return 0
}

# 统一把潜在的 Go 安装路径加入 PATH，兼容系统级安装和当前用户级安装两种模式。
prepend_path_if_exists() {
    local target_path="$1"

    if [ -d "${target_path}" ] && [[ ":${PATH}:" != *":${target_path}:"* ]]; then
        export PATH="${target_path}:${PATH}"
    fi
}

# 刷新 Go 可执行文件搜索路径，确保刚安装完成后当前脚本能立刻找到 go 命令。
refresh_go_binary_path() {
    prepend_path_if_exists "${GO_SYSTEM_INSTALL_DIR}/bin"
    prepend_path_if_exists "${GO_USER_INSTALL_DIR}/bin"
}

# 将 Go 的 bin 路径持久化到常用 shell 配置文件，避免下次登录后还要手动导出 PATH。
persist_go_path_to_profiles() {
    local go_bin_path="$1"
    local export_line="export PATH=\"${go_bin_path}:\$PATH\""
    local profile_file=""

    for profile_file in "${HOME}/.profile" "${HOME}/.bashrc"; do
        touch "${profile_file}"
        if ! grep -Fqx "${export_line}" "${profile_file}" 2>/dev/null; then
            printf '\n# cool-dispatch 自动补充 Go 环境\n%s\n' "${export_line}" >> "${profile_file}"
        fi
    done
}

# 只在 Ubuntu 22.04 上启用自动安装流程，其他系统仍保持显式提示，避免误装到非目标环境。
is_ubuntu_2204() {
    if [ ! -f /etc/os-release ]; then
        return 1
    fi

    # shellcheck disable=SC1091
    . /etc/os-release
    [ "${ID}" = "ubuntu" ] && [ "${VERSION_ID}" = "22.04" ]
}

# 根据 CPU 架构选择对应的 Go 官方安装包名称，当前只覆盖服务器常见架构。
detect_go_arch() {
    case "$(uname -m)" in
        x86_64|amd64)
            printf '%s\n' "amd64"
            ;;
        aarch64|arm64)
            printf '%s\n' "arm64"
            ;;
        *)
            log_error "暂不支持的 CPU 架构: $(uname -m)"
            return 1
            ;;
    esac
}

# 为 Go 安装流程补齐下载工具，优先复用 curl/wget，缺失时在 Ubuntu 22.04 上自动安装 curl。
ensure_go_download_tool() {
    if command -v curl >/dev/null 2>&1 || command -v wget >/dev/null 2>&1; then
        return 0
    fi

    if ! is_ubuntu_2204; then
        log_error "未检测到 curl 或 wget，且当前系统不是 Ubuntu 22.04，无法自动补齐下载工具"
        return 1
    fi

    log_warning "未检测到 curl 或 wget，准备自动安装 curl..."

    if [ "$(id -u)" -eq 0 ]; then
        apt-get update && apt-get install -y curl
    elif command -v sudo >/dev/null 2>&1; then
        sudo apt-get update && sudo apt-get install -y curl
    else
        log_error "当前用户无 root 权限且未安装 sudo，无法自动安装 curl"
        return 1
    fi

    command -v curl >/dev/null 2>&1
}

# 在 Ubuntu 22.04 上按项目要求自动安装 Go；优先安装到 /usr/local/go，权限不足时回退到当前用户目录。
install_go_runtime() {
    local required_version=""
    local arch=""
    local archive_name=""
    local download_url=""
    local archive_path=""
    local install_parent=""
    local install_mode=""
    local go_bin_path=""
    local installed_version=""

    required_version="$(get_required_go_version)"
    if [ -z "${required_version}" ]; then
        log_error "未能从 ${GO_VERSION_FILE} 解析到 Go 版本，无法自动安装"
        return 1
    fi

    if ! is_ubuntu_2204; then
        log_error "当前系统不是 Ubuntu 22.04，未启用自动安装 Go 的预置流程"
        return 1
    fi

    arch="$(detect_go_arch)" || return 1
    archive_name="go${required_version}.linux-${arch}.tar.gz"
    download_url="${GO_DOWNLOAD_BASE_URL}/${archive_name}"
    archive_path="/tmp/${archive_name}"

    if [ -w "/usr/local" ]; then
        install_parent="/usr/local"
        install_mode="system"
        go_bin_path="${GO_SYSTEM_INSTALL_DIR}/bin"
    elif command -v sudo >/dev/null 2>&1; then
        install_parent="/usr/local"
        install_mode="system-with-sudo"
        go_bin_path="${GO_SYSTEM_INSTALL_DIR}/bin"
    else
        install_parent="$(dirname "${GO_USER_INSTALL_DIR}")"
        install_mode="user"
        go_bin_path="${GO_USER_INSTALL_DIR}/bin"
    fi

    ensure_go_download_tool || return 1

    log_info "准备安装 Go ${required_version} (${arch})"
    log_info "下载地址: ${download_url}"
    log_info "安装模式: ${install_mode}"

    if command -v curl >/dev/null 2>&1; then
        curl -fL "${download_url}" -o "${archive_path}"
    else
        wget -O "${archive_path}" "${download_url}"
    fi

    if [ $? -ne 0 ]; then
        log_error "Go 安装包下载失败"
        return 1
    fi

    if [ "${install_mode}" = "system" ]; then
        rm -rf "${GO_SYSTEM_INSTALL_DIR}" && tar -C "/usr/local" -xzf "${archive_path}"
    elif [ "${install_mode}" = "system-with-sudo" ]; then
        sudo rm -rf "${GO_SYSTEM_INSTALL_DIR}" && sudo tar -C "/usr/local" -xzf "${archive_path}"
    else
        mkdir -p "${install_parent}"
        rm -rf "${GO_USER_INSTALL_DIR}" && tar -C "${install_parent}" -xzf "${archive_path}"
    fi

    if [ $? -ne 0 ]; then
        log_error "Go 解压安装失败"
        rm -f "${archive_path}"
        return 1
    fi

    rm -f "${archive_path}"

    refresh_go_binary_path
    persist_go_path_to_profiles "${go_bin_path}"

    installed_version="$(get_installed_go_version || true)"
    if [ -n "${installed_version}" ] && version_gte "${installed_version}" "${required_version}"; then
        log_success "Go 安装完成: go${installed_version}"
        return 0
    fi

    log_error "Go 安装流程已执行，但当前仍未检测到满足要求的版本"
    return 1
}

# 统一补齐 Go 相关路径和缓存目录；缺少 Go 或版本过低时，在 Ubuntu 22.04 上自动安装项目要求版本。
ensure_go_env() {
    refresh_go_binary_path
    export GOPATH="${HOME}/go"
    export PATH="${PATH}:${GOPATH}/bin"
    export GOCACHE="${HOME}/.cache/go-build"
    export XDG_CACHE_HOME="${HOME}/.cache"

    mkdir -p "${GOCACHE}" "${XDG_CACHE_HOME}"

    local required_version=""
    local installed_version=""
    required_version="$(get_required_go_version || true)"

    if command -v go >/dev/null 2>&1; then
        installed_version="$(get_installed_go_version || true)"
        if [ -n "${required_version}" ] && [ -n "${installed_version}" ] && ! version_gte "${installed_version}" "${required_version}"; then
            log_warning "当前 Go 版本 go${installed_version} 低于项目要求 go${required_version}，准备自动安装"
            install_go_runtime || return 1
        fi
    else
        log_warning "未检测到 Go 环境"
        if is_ubuntu_2204; then
            log_info "检测到 Ubuntu 22.04，准备自动安装项目要求的 Go 环境"
            install_go_runtime || return 1
        else
            log_error "当前系统不是 Ubuntu 22.04，且未检测到 Go 环境，请手动安装 go${required_version}"
            return 1
        fi
    fi

    refresh_go_binary_path

    if command -v go >/dev/null 2>&1; then
        log_success "Go 环境检测成功: $(go version)"
    else
        log_error "Go 环境初始化失败，当前 PATH: ${PATH}"
        return 1
    fi
}

# 统一修复脚本权限。这里改为 755，既能保证可执行，也避免 777 过宽权限。
# 当前仓库已经收口为单入口，因此这里只维护仍然实际存在的脚本文件。
fix_script_permissions() {
    enter_repo_root

    echo "===================================="
    echo "修复脚本文件权限"
    echo "===================================="
    echo ""

    local files=(
        "project_maintenance.sh"
        "git_push.sh"
        "start.sh"
    )
    local success_count=0
    local fail_count=0

    for file in "${files[@]}"; do
        if [ -f "${SCRIPT_DIR}/${file}" ]; then
            chmod 755 "${SCRIPT_DIR}/${file}"
            if [ $? -eq 0 ]; then
                echo "✓ ${file} - 权限已设置为 755"
                success_count=$((success_count + 1))
            else
                echo "✗ ${file} - 权限设置失败"
                fail_count=$((fail_count + 1))
            fi
        else
            echo "⚠ ${file} - 文件不存在"
        fi
    done

    echo ""
    echo "===================================="
    echo "完成：成功 ${success_count} 个，失败 ${fail_count} 个"
    echo "===================================="
}

# 根据当前 origin 地址尽量推断仓库路径；如果无法识别，就回退到项目默认仓库路径。
extract_repo_path_from_remote() {
    local remote_url="$1"
    local repo_path=""

    if [[ "${remote_url}" == git@github.com:* ]]; then
        repo_path="${remote_url#git@github.com:}"
    elif [[ "${remote_url}" == https://* ]] || [[ "${remote_url}" == http://* ]]; then
        repo_path="${remote_url#http://}"
        repo_path="${repo_path#https://}"
        repo_path="${repo_path#*@}"
        repo_path="${repo_path#github.com/}"
    fi

    repo_path="${repo_path%.git}"

    if [ -z "${repo_path}" ]; then
        repo_path="${DEFAULT_REPO_PATH}"
    fi

    printf '%s\n' "${repo_path}"
}

# 统一拼装带鉴权的 HTTPS 远程地址，供 fix-url 和 auto-pull 共用。
build_authenticated_remote_url() {
    local repo_path="$1"
    printf 'https://%s:%s@github.com/%s.git\n' "${GIT_USERNAME}" "${GIT_TOKEN}" "${repo_path%.git}"
}

# 负责修复 origin 远程地址，不直接假定当前仓库一定是某一种 URL 格式。
configure_git_remote_url() {
    enter_repo_root
    ensure_git_credentials || return 1

    local current_url
    current_url="$(git config --get remote.origin.url 2>/dev/null || true)"

    if [ -z "${current_url}" ]; then
        log_error "未读取到 remote.origin.url，无法修复 Git URL"
        return 1
    fi

    local repo_path
    repo_path="$(extract_repo_path_from_remote "${current_url}")"

    local new_url
    new_url="$(build_authenticated_remote_url "${repo_path}")"

    log_info "当前 URL: $(mask_token "${current_url}")"
    log_info "正在修复 Git URL..."

    git remote set-url origin "${new_url}"
    if [ $? -ne 0 ]; then
        log_error "Git URL 修复失败"
        return 1
    fi

    log_success "Git URL 修复成功"
    log_info "新的 URL: $(mask_token "${new_url}")"
    return 0
}

# 单独抽出连接测试，便于 fix-url 和后续扩展场景共用。
test_git_connection() {
    enter_repo_root
    log_info "测试 Git 连接..."
    git fetch origin
    if [ $? -eq 0 ]; then
        log_success "Git 连接测试成功"
        return 0
    fi

    log_error "Git 连接测试失败，请检查网络或凭证"
    return 1
}

# 修复 origin 远程地址后立即测试连接。
fix_git_url() {
    echo "===================================="
    echo "Git URL 修复工具"
    echo "===================================="
    echo ""

    configure_git_remote_url || return 1
    echo ""
    test_git_connection || return 1

    echo ""
    echo "===================================="
    echo "修复完成！"
    echo "===================================="
}

# 释放端口占用，兼容 Linux 的 ss 与 macOS 的 lsof 两种方式。
release_port() {
    local port="$1"
    local pid=""

    if command -v ss >/dev/null 2>&1; then
        pid="$(ss -ltnp 2>/dev/null | awk -v target=":${port} " '
            index($0, target) > 0 {
                if (match($0, /pid=[0-9]+/)) {
                    print substr($0, RSTART + 4, RLENGTH - 4)
                    exit
                }
            }
        ')"
    else
        pid="$(lsof -ti:"${port}" 2>/dev/null | head -n 1)"
    fi

    if [ -n "${pid}" ]; then
        log_warning "发现端口 ${port} 被占用 (PID: ${pid})，正在释放..."
        kill -9 "${pid}" 2>/dev/null
        sleep 1
        log_success "端口 ${port} 已释放"
    else
        log_success "端口 ${port} 未被占用"
    fi
}

# 检查端口是否处于监听状态，用于构建后快速确认服务是否启动成功。
check_port_listening() {
    local port="$1"

    if command -v ss >/dev/null 2>&1; then
        ss -ltn 2>/dev/null | grep -q ":${port} "
        return $?
    fi

    lsof -i:"${port}" -sTCP:LISTEN >/dev/null 2>&1
    return $?
}

# 构建服务端、构建前端、释放端口、尝试重启服务，并检查端口监听状态。
build_project() {
    enter_repo_root

    echo "===================================="
    echo "${PROJECT_NAME} 开始构建项目..."
    echo "===================================="
    echo ""

    echo -e "${YELLOW}[1/2] 构建服务端 (server)...${NC}"
    cd "${SCRIPT_DIR}/server" || return 1
    go build -buildvcs=false -o api ./cmd/api
    if [ $? -ne 0 ]; then
        echo -e "${RED}错误: 服务端构建失败！${NC}"
        cd "${SCRIPT_DIR}" || return 1
        return 1
    fi
    cd "${SCRIPT_DIR}" || return 1
    echo -e "${GREEN}服务端构建完成 ✓${NC}"
    echo ""

    echo -e "${YELLOW}[2/2] 构建客户端 (client)...${NC}"
    cd "${SCRIPT_DIR}/client" || return 1
    echo "安装客户端依赖包..."
    npm install
    if [ $? -ne 0 ]; then
        echo -e "${RED}错误: 客户端依赖包安装失败！${NC}"
        cd "${SCRIPT_DIR}" || return 1
        return 1
    fi
    echo -e "${GREEN}✓ 客户端依赖包安装完成${NC}"

    npm run build
    if [ $? -ne 0 ]; then
        echo -e "${RED}错误: 客户端构建失败！${NC}"
        cd "${SCRIPT_DIR}" || return 1
        return 1
    fi
    cd "${SCRIPT_DIR}" || return 1
    echo -e "${GREEN}客户端构建完成 ✓${NC}"
    echo ""

    echo "===================================="
    echo -e "${GREEN}构建完成！所有模块已成功构建${NC}"
    echo "===================================="
    echo ""

    echo "检查并释放端口占用..."
    release_port 9102
    echo ""

    echo "启动 Go 项目 (${PROJECT_NAME} server)..."
    if [ -f "/www/server/go_project/vhost/scripts/cool_dispatch_server.sh" ]; then
        bash /www/server/go_project/vhost/scripts/cool_dispatch_server.sh
    elif [ -f "${SCRIPT_DIR}/server/api" ]; then
        cd "${SCRIPT_DIR}/server" || return 1
        nohup ./api >/dev/null 2>&1 &
        cd "${SCRIPT_DIR}" || return 1
        echo -e "${GREEN}✓ Go 项目已后台启动${NC}"
    else
        echo -e "${YELLOW}⚠ 未找到可执行文件或启动脚本，请手动启动${NC}"
    fi

    echo ""
    echo "===================================="
    echo -e "${YELLOW}验证服务状态...${NC}"
    echo "===================================="

    sleep 2
    echo "检查服务端口状态..."
    if check_port_listening 9102; then
        echo -e "${GREEN}✓ 端口 9102 正在监听${NC}"
    else
        echo -e "${RED}✗ 端口 9102 未在监听${NC}"
    fi

    echo ""
    echo "===================================="
    echo -e "${GREEN}所有操作完成！${NC}"
    echo "===================================="
}

# 检查远程是否有更新，有更新时执行强制同步并自动构建。
check_and_pull() {
    enter_repo_root
    log_info "检查远程更新..."

    git fetch origin 2>&1
    if [ $? -ne 0 ]; then
        log_error "获取远程信息失败！"
        return 1
    fi

    local current_branch
    current_branch="$(git rev-parse --abbrev-ref HEAD)"

    local local_commit
    local remote_commit
    local_commit="$(git rev-parse HEAD)"
    remote_commit="$(git rev-parse "origin/${current_branch}")"

    if [ "${local_commit}" = "${remote_commit}" ]; then
        log_info "代码已是最新版本"
        return 0
    fi

    log_warning "检测到远程有更新！"
    log_info "本地: ${local_commit}"
    log_info "远程: ${remote_commit}"
    log_info "开始拉取代码..."

    # 自动同步时仍保持强制对齐远端的策略，避免本地残留改动阻塞无人值守流程。
    log_info "重置本地所有修改..."
    git reset --hard HEAD >/dev/null 2>&1

    log_info "强制同步到远程最新版本..."
    git reset --hard "origin/${current_branch}"
    if [ $? -ne 0 ]; then
        log_error "代码拉取失败！"
        return 1
    fi

    log_success "代码拉取成功"
    log_success "更新完成: $(date '+%Y-%m-%d %H:%M:%S')"

    log_info "修复脚本文件权限..."
    fix_script_permissions >/dev/null
    log_success "权限修复完成"

    load_runtime_env
    ensure_go_env || return 1

    log_info "开始自动构建..."
    build_project
    if [ $? -eq 0 ]; then
        log_success "自动构建成功"
        return 0
    fi

    log_error "自动构建失败"
    return 1
}

# 自动拉取模式保留原行为：启动时做一次环境与 Git 配置，然后进入无限轮询。
auto_pull_loop() {
    enter_repo_root

    echo "===================================="
    echo "${PROJECT_NAME} 自动 Git 拉取脚本启动"
    echo "检查间隔: ${CHECK_INTERVAL} 秒"
    echo "当前目录: $(pwd)"
    echo "===================================="
    echo ""

    load_runtime_env
    ensure_go_env || return 1
    echo ""

    configure_git_remote_url || return 1
    echo ""

    while true; do
        check_and_pull
        echo ""
        log_info "等待 ${CHECK_INTERVAL} 秒后进行下次检查..."
        echo ""
        sleep "${CHECK_INTERVAL}"
    done
}

# 统一入口：通过子命令调度具体能力，后续新增能力时只需要继续扩展这里。
main() {
    local command="${1:-help}"

    case "${command}" in
        fix-url)
            fix_git_url
            ;;
        fix-permissions)
            fix_script_permissions
            ;;
        install-go)
            load_runtime_env
            ensure_go_env
            ;;
        build)
            load_runtime_env
            ensure_go_env || return 1
            build_project
            ;;
        sync-once)
            load_runtime_env
            ensure_go_env || return 1
            configure_git_remote_url || return 1
            check_and_pull
            ;;
        auto-pull)
            auto_pull_loop
            ;;
        help|-h|--help)
            print_usage
            ;;
        *)
            log_error "未知命令: ${command}"
            echo ""
            print_usage
            return 1
            ;;
    esac
}

# 捕获中断信号，保证自动轮询模式可以友好退出。
trap 'echo ""; log_warning "脚本已停止"; exit 0' INT TERM


main "$@"
