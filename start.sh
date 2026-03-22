#!/usr/bin/env bash

set -euo pipefail

# 统一定位仓库根目录，保证从任意位置执行脚本都能找到前后端目录。
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CLIENT_DIR="$ROOT_DIR/client"
SERVER_DIR="$ROOT_DIR/server"
POSTGRES_COMPOSE_FILE="$ROOT_DIR/docker-compose.postgres.yml"
POSTGRES_SERVICE_NAME="${POSTGRES_SERVICE_NAME:-postgres}"
POSTGRES_IMAGE="${POSTGRES_IMAGE:-postgres:16-alpine}"
POSTGRES_CONTAINER_NAME="${POSTGRES_CONTAINER_NAME:-cool-dispatch-postgres}"
MODE="${1:-menu}"

log() {
  printf '[quick-start] %s\n' "$1"
}

# load_env_file 自动加载仓库根目录下的 .env，保持脚本与手工启动时的环境变量一致。
load_env_file() {
  if [ ! -f "$ROOT_DIR/.env" ]; then
    return
  fi

  set -a
  # shellcheck disable=SC1091
  . "$ROOT_DIR/.env"
  set +a
}

# command_exists 统一判断系统命令是否可用，避免不同分支重复写检测逻辑。
command_exists() {
  command -v "$1" >/dev/null 2>&1
}

# ensure_docker_compose 检查 Docker 与 Compose 能力是否可用，缺失时给出明确报错。
ensure_docker_compose() {
  if ! command_exists docker; then
    echo "docker 未安装，无法启动 PostgreSQL 容器。" >&2
    exit 1
  fi

  if ! docker compose version >/dev/null 2>&1; then
    echo "docker compose 不可用，请先安装或启用 Docker Compose v2。" >&2
    exit 1
  fi
}

# default_postgres_data_dir 按运行环境给出 PostgreSQL 数据目录默认值。
# macOS 固定落到外置盘目录；Ubuntu 22.04 优先使用 /srv，不可写时回退到当前用户家目录。
default_postgres_data_dir() {
  case "$(uname -s)" in
    Darwin)
      printf '%s' "/Volumes/externalHard/data/cool-dispatch/postgresql"
      ;;
    Linux)
      if [ -w "/srv" ] || [ "$(id -u)" -eq 0 ]; then
        printf '%s' "/srv/cool-dispatch/postgresql"
      else
        printf '%s' "$HOME/data/cool-dispatch/postgresql"
      fi
      ;;
    *)
      printf '%s' "$ROOT_DIR/.data/postgresql"
      ;;
  esac
}

load_env_file

# resolve_config_file 返回当前后端应读取的配置文件路径；若不存在则返回空字符串。
resolve_config_file() {
  if [ -n "${CONFIG_FILE:-}" ] && [ -f "${CONFIG_FILE}" ]; then
    printf '%s' "${CONFIG_FILE}"
    return
  fi

  if [ -f "$ROOT_DIR/config.yaml" ]; then
    printf '%s' "$ROOT_DIR/config.yaml"
    return
  fi

  if [ -f "$ROOT_DIR/config.yml" ]; then
    printf '%s' "$ROOT_DIR/config.yml"
    return
  fi
}

# config_file_has_value 判断配置文件里某个 key 是否存在非空值，避免启动脚本默认环境变量覆盖 config.yaml。
config_file_has_value() {
  local key="$1"
  local config_file
  config_file="$(resolve_config_file)"
  if [ -z "$config_file" ]; then
    return 1
  fi

  grep -Eq "^[[:space:]]*${key}:[[:space:]]*.+$" "$config_file"
}

# config_file_bool_true 判断配置文件里的布尔 key 是否显式为 true。
config_file_bool_true() {
  local key="$1"
  local config_file
  config_file="$(resolve_config_file)"
  if [ -z "$config_file" ]; then
    return 1
  fi

  grep -Eqi "^[[:space:]]*${key}:[[:space:]]*true([[:space:]]|#.*)?$" "$config_file"
}

# setup_postgres_env 统一补齐 compose 所需环境变量，保证本地与服务器目录策略一致。
setup_postgres_env() {
  export POSTGRES_DB="${POSTGRES_DB:-cool_dispatch}"
  export POSTGRES_USER="${POSTGRES_USER:-postgres}"
  export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-postgres}"
  export POSTGRES_PORT="${POSTGRES_PORT:-9101}"
  export POSTGRES_CONTAINER_PORT="${POSTGRES_CONTAINER_PORT:-5432}"
  export POSTGRES_DATA_DIR="${POSTGRES_DATA_DIR:-$(default_postgres_data_dir)}"

  # DATABASE_URL 只在外部未显式指定且 config.yaml 未声明时兜底注入，
  # 避免脚本默认值反过来覆盖后端配置文件里的数据库连接串。
  if [ -z "${DATABASE_URL:-}" ] && ! config_file_has_value "database_url"; then
    export DATABASE_URL="postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@localhost:${POSTGRES_PORT}/${POSTGRES_DB}?sslmode=disable}"
  fi
}

# ensure_postgres_data_dir 确保持久化目录存在，避免 bind mount 首次启动失败。
ensure_postgres_data_dir() {
  if [ -d "$POSTGRES_DATA_DIR" ]; then
    return
  fi

  log "creating postgres data directory: $POSTGRES_DATA_DIR"
  mkdir -p "$POSTGRES_DATA_DIR"
}

# postgres_container_exists 判断目标容器是否已创建，便于优先直接 start。
postgres_container_exists() {
  docker ps -a --format '{{.Names}}' | grep -Fx "$POSTGRES_CONTAINER_NAME" >/dev/null 2>&1
}

# postgres_image_exists 判断目标镜像是否已存在，满足“已有镜像不重新下载”的要求。
postgres_image_exists() {
  docker image inspect "$POSTGRES_IMAGE" >/dev/null 2>&1
}

# wait_for_postgres_ready 轮询容器健康状态，避免后端在数据库尚未就绪时抢先启动。
wait_for_postgres_ready() {
  local retries=30
  local status=""

  while [ "$retries" -gt 0 ]; do
    status="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$POSTGRES_CONTAINER_NAME" 2>/dev/null || true)"
    if [ "$status" = "healthy" ] || [ "$status" = "running" ]; then
      log "postgres container is ready: $POSTGRES_CONTAINER_NAME ($status)"
      return
    fi

    retries=$((retries - 1))
    sleep 1
  done

  echo "postgres 容器未在预期时间内就绪，请执行 docker compose -f $POSTGRES_COMPOSE_FILE ps 查看状态。" >&2
  exit 1
}

# start_postgres 启动 PostgreSQL 容器。
# 启动顺序固定为：已有容器直接 start -> 仅有镜像时直接 up -> 都没有时先 pull 再 up。
start_postgres() {
  ensure_docker_compose
  setup_postgres_env
  ensure_postgres_data_dir

  log "starting postgres with compose file $POSTGRES_COMPOSE_FILE"
  log "postgres data dir: $POSTGRES_DATA_DIR"

  if postgres_container_exists; then
    log "reusing existing postgres container: $POSTGRES_CONTAINER_NAME"
    docker compose -f "$POSTGRES_COMPOSE_FILE" start "$POSTGRES_SERVICE_NAME" >/dev/null
    wait_for_postgres_ready
    return
  fi

  if postgres_image_exists; then
    log "reusing local postgres image without pulling: $POSTGRES_IMAGE"
    docker compose -f "$POSTGRES_COMPOSE_FILE" up -d --no-build "$POSTGRES_SERVICE_NAME" >/dev/null
    wait_for_postgres_ready
    return
  fi

  log "postgres image not found locally, pulling: $POSTGRES_IMAGE"
  docker compose -f "$POSTGRES_COMPOSE_FILE" pull "$POSTGRES_SERVICE_NAME" >/dev/null
  docker compose -f "$POSTGRES_COMPOSE_FILE" up -d "$POSTGRES_SERVICE_NAME" >/dev/null
  wait_for_postgres_ready
}

# 兼容本机未显式注入 Go 环境变量的情况。
ensure_go_path() {
  if command -v go >/dev/null 2>&1; then
    return
  fi

  export PATH="/usr/local/go/bin:/opt/homebrew/bin:$PATH"
}

# ensure_go_cache_dir 将 Go 构建缓存固定到仓库内可写目录，避免不同终端/沙箱落到不可写系统路径。
ensure_go_cache_dir() {
  export GOCACHE="${GOCACHE:-$ROOT_DIR/.cache/go-build}"
  mkdir -p "$GOCACHE"
}

# ensure_go_runtime_dirs 同步补齐模块缓存、GOPATH 与 HOME，避免受限环境里 go test/go build 因默认目录不可写失败。
ensure_go_runtime_dirs() {
  export GOMODCACHE="${GOMODCACHE:-$ROOT_DIR/.cache/go-mod}"
  export GOPATH="${GOPATH:-$ROOT_DIR/.cache/go-path}"
  export HOME="${HOME:-$ROOT_DIR/.cache/home}"
  mkdir -p "$GOMODCACHE" "$GOPATH" "$HOME"
}

# ensure_seed_passwords 在启用 demo seed 时强制要求显式提供演示账号配置，
# 支持从环境变量或 config.yaml 读取，避免后端再次回退到仓库硬编码默认账号与密码。
ensure_seed_passwords() {
  local seed_demo_data="${SEED_DEMO_DATA:-}"
  if [ "$seed_demo_data" != "true" ]; then
    if [ -n "$seed_demo_data" ] || ! config_file_bool_true "seed_demo_data"; then
      return
    fi
  fi

  if [ -n "${SEED_ADMIN_NAME:-}" ] && [ -n "${SEED_ADMIN_PHONE:-}" ] && [ -n "${SEED_ADMIN_PASSWORD:-}" ] && [ -n "${SEED_TECHNICIAN_PASSWORD:-}" ]; then
    return
  fi

  local config_file
  config_file="$(resolve_config_file)"

  if [ -n "$config_file" ] \
    && grep -Eq '^[[:space:]]*seed_admin_name:[[:space:]]*.+$' "$config_file" \
    && grep -Eq '^[[:space:]]*seed_admin_phone:[[:space:]]*.+$' "$config_file" \
    && grep -Eq '^[[:space:]]*seed_admin_password:[[:space:]]*.+$' "$config_file" \
    && grep -Eq '^[[:space:]]*seed_technician_password:[[:space:]]*.+$' "$config_file"; then
    return
  fi

  echo "SEED_DEMO_DATA=true 时必须提供 seed_admin_name、seed_admin_phone、seed_admin_password 与 seed_technician_password。" >&2
  echo "可通过环境变量或 config.yaml 提供；示例见 config.yaml.example。" >&2
  exit 1
}

# ensure_npm 检查 Node/npm 是否可用，避免脚本在缺少前端运行环境时直接报错退出。
ensure_npm() {
  if command_exists npm; then
    return
  fi

  echo "npm 未安装，无法启动或校验前端。" >&2
  exit 1
}

# run_frontend 启动 Vite 开发服务器。
run_frontend() {
  ensure_npm
  log "starting frontend from $CLIENT_DIR with: npm run dev"
  cd "$CLIENT_DIR"
  exec npm run dev
}

# run_backend 启动 Go API 服务，并保持自动迁移与 demo seed 默认关闭，
# 只有调用方显式导出相关环境变量时才开启对应能力。
run_backend() {
  start_postgres
  ensure_go_path
  ensure_go_cache_dir
  ensure_go_runtime_dirs
  log "starting backend from $SERVER_DIR with: go run ./cmd/api"
  cd "$SERVER_DIR"
  ensure_seed_passwords
  exec go run ./cmd/api
}

# run_both 先后台拉起服务端，再以前台方式运行前端，便于 Ctrl+C 一次性退出。
# 与 run_backend 一致，自动迁移和 demo seed 仍默认关闭，避免开发脚本绕过安全基线。
run_both() {
  start_postgres
  ensure_npm
  log "starting backend from $SERVER_DIR with: go run ./cmd/api"
  (
    ensure_go_path
    ensure_go_cache_dir
    ensure_go_runtime_dirs
    cd "$SERVER_DIR"
    ensure_seed_passwords
    go run ./cmd/api
  ) &
  backend_pid=$!

  cleanup() {
    kill "$backend_pid" >/dev/null 2>&1 || true
  }

  trap cleanup EXIT INT TERM

  log "starting frontend from $CLIENT_DIR with: npm run dev"
  cd "$CLIENT_DIR"
  npm run dev
}

show_help() {
  cat <<'EOF'
Usage: ./start.sh [frontend|postgres|backend|both|build|check]

frontend  Start the Vite client dev server
backend   Start PostgreSQL, then run the Go API server
both      Start PostgreSQL and the Go API in background, then run the Vite client
postgres  Start only the PostgreSQL Docker service
build     Build both client and server
check     Run frontend type check and backend go test

Project layout:
  client/  React + Vite frontend
  server/  Go API backend

PostgreSQL defaults:
  macOS data dir   /Volumes/externalHard/data/cool-dispatch/postgresql
  Ubuntu data dir  /srv/cool-dispatch/postgresql
  Ubuntu fallback  $HOME/data/cool-dispatch/postgresql
  Override with    POSTGRES_DATA_DIR=/custom/path
EOF
}

# prompt_mode 在未传参数时提供交互式菜单，兼容旧脚本的使用习惯。
prompt_mode() {
  cat <<'EOF'
==============================
 Cool-Dispatch 快速启动
==============================
1) 前端
2) PostgreSQL
3) 后端（自动拉起 PostgreSQL）
4) 前后端联启（自动拉起 PostgreSQL）
5) 构建前后端
6) 校验（前端 check + 后端 go test）
7) 帮助
EOF

  read -r -p "请输入选项（1/2/3/4/5/6/7）: " choice

  case "$choice" in
    1)
      run_frontend
      ;;
    2)
      start_postgres
      ;;
    3)
      run_backend
      ;;
    4)
      run_both
      ;;
    5)
      build_all
      ;;
    6)
      run_checks
      ;;
    7)
      show_help
      ;;
    *)
      echo "无效选项，请输入 1、2、3、4、5、6 或 7。" >&2
      exit 1
      ;;
  esac
}

# build_all 顺序构建前后端，确保目录迁移后的关键路径仍然可用。
build_all() {
  ensure_npm
  log "building frontend in $CLIENT_DIR with: npm run build"
  (
    cd "$CLIENT_DIR"
    npm run build
  )

  log "building backend in $SERVER_DIR with: go build ./..."
  (
    ensure_go_path
    ensure_go_cache_dir
    ensure_go_runtime_dirs
    cd "$SERVER_DIR"
    go build ./...
  )
}

# run_checks 顺序执行前端类型检查与后端单元测试，供审查收口和迁移回归使用。
run_checks() {
  ensure_npm
  ensure_go_path
  ensure_go_cache_dir
  ensure_go_runtime_dirs

  log "checking frontend in $CLIENT_DIR with: npm run check"
  (
    cd "$CLIENT_DIR"
    npm run check
  )

  log "testing backend in $SERVER_DIR with: go test ./..."
  (
    cd "$SERVER_DIR"
    go test ./...
  )
}

case "$MODE" in
  frontend)
    run_frontend
    ;;
  postgres)
    start_postgres
    ;;
  backend)
    run_backend
    ;;
  both)
    run_both
    ;;
  build)
    build_all
    ;;
  check)
    run_checks
    ;;
  menu)
    prompt_mode
    ;;
  help|-h|--help)
    show_help
    ;;
  *)
    echo "Unknown mode: $MODE" >&2
    show_help
    exit 1
    ;;
esac
