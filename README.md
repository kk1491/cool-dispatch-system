# Cool Dispatch

项目已整理为明确的前后端分层：

- `client/`: React + Vite 前端与前端构建配置
- `server/`: Go + Gin + Gorm 后端、迁移与内部包
- `dist/client`: 前端生产构建输出，Go 服务可直接托管

## 目录说明

- `client/src`: 前端页面、组件、hooks、业务逻辑
- `server/cmd/api`: Go API 入口
- `server/cmd/migrate`: Go 迁移入口
- `server/cmd/useradmin`: 账号初始化 / 重置密码入口
- `server/internal`: 配置、数据库、模型、HTTP handler、seed
- `server/database/migrations`: SQL 迁移文件

## 已兼容接口

- `GET /api/health`
- `POST /api/auth/login`
- `POST /api/webhook/line`
- `GET /api/reviews/token/:reviewToken/context`
- `POST /api/reviews/token/:reviewToken`

当前接口已按权限分层：

- 公开接口：
  - `GET /api/health`
  - `POST /api/auth/login`
  - `POST /api/webhook/line`
  - `GET /api/reviews/token/:reviewToken/context`
  - `POST /api/reviews/token/:reviewToken`
  - `PATCH /api/reviews/token/:reviewToken/share-line`
- 登录后可访问：
  - `GET /api/auth/me`
  - `POST /api/auth/logout`
  - `GET /api/bootstrap`
  - `GET /api/appointments`
  - `PATCH /api/appointments/:id`
  - `GET /api/technicians`
  - `GET /api/customers`
  - `GET /api/zones`
  - `GET /api/service-items`
  - `GET /api/extra-items`
  - `GET /api/cash-ledger`
  - `GET /api/reviews`
  - `GET /api/notifications`
  - `GET /api/settings`
  - `GET /api/line-data`
  - `GET /api/*-page-data` 与兼容旧路径的 `GET /api/pages/*`
- 仅管理员可写：
  - `POST /api/appointments`
  - `DELETE /api/appointments/:id`
  - `PUT /api/technicians`
  - `PUT /api/zones`
  - `PUT /api/service-items`
  - `PUT /api/extra-items`
  - `POST /api/cash-ledger`
  - `POST /api/notifications`
  - `PUT /api/settings/reminder-days`
  - `PUT /api/settings/webhook-enabled`
  - `PUT /api/customers`
  - `DELETE /api/customers/:id`
  - `PUT /api/line-friends/:lineUid/customer`

其中 `GET /api/line-data` 同时返回 `joined_at` 与 `line_joined_at` 字段，以兼容现有前端。

预约写接口当前按“读写分离 DTO”执行：

- `POST /api/appointments` 仅接受创建预约所需业务字段。
- `PATCH /api/appointments/:id` 仅接受排程与作业进度字段。
- `id`、`created_at`、`technician_name`、`zone_id`、`total_amount`、`payment_time` 等只读/派生字段由后端拒绝或重算，不能由客户端直传。
- `paid_amount`、`payment_received` 仅允许在更新预约时提交；创建预约时这两项仍由后端收口。
- `zone_id`、`technician_name`、`payment_time` 会由后端根据地址、师傅与支付状态自动推导或沿用，客户端不得覆盖。
- 历史脏资料里若存在 `payment_method=未收款`，普通编辑会沿用原支付字段；只有技师确认收款或管理员显式补录真实付款方式时，前后端才会一起收敛为 `現金 / 轉帳 / 無收款`。
- `payment_method=無收款` 时，后端会自动清零 `paid_amount` 并清空 `payment_received/payment_time`；前端流程应直接跳过收款确认并进入任务完成。

## 配置文件与环境变量

后端现在支持按以下优先级读取配置：

1. 代码内置默认值
2. `config.yaml` / `config.yml`
3. 环境变量覆盖

可先复制示例文件：

```bash
cp config.yaml.example config.yaml
```

如需自定义配置文件路径，可设置 `CONFIG_FILE=/your/path/config.yaml`。

## 环境变量

复制 `.env.example` 后按需调整：

```bash
cp .env.example .env
```

关键变量：

- `DATABASE_URL`: PostgreSQL 连接串
- `POSTGRES_IMAGE`: PostgreSQL Docker 镜像，默认 `postgres:16-alpine`
- `POSTGRES_CONTAINER_NAME`: PostgreSQL 容器名，默认 `cool-dispatch-postgres`
- `POSTGRES_DB` / `POSTGRES_USER` / `POSTGRES_PASSWORD`: PostgreSQL 初始化库名与账号
- `POSTGRES_PORT`: 宿主机暴露端口，默认 `9101`
- `POSTGRES_CONTAINER_PORT`: 容器内部 PostgreSQL 端口，默认 `5432`
- `POSTGRES_DATA_DIR`: PostgreSQL 数据目录。本地建议 `/Volumes/externalHard/data/cool-dispatch/postgresql`，Ubuntu 22.04 建议 `/srv/cool-dispatch/postgresql`
- `PORT`: Go 服务端口，默认 `9102`
- `FRONTEND_ORIGIN`: 后端 CORS 放行的前端开发来源，默认 `http://localhost:5173`
- `LINE_CHANNEL_SECRET`: LINE Developers Channel Secret；`POST /api/webhook/line` 会用它做 `HMAC-SHA256 + base64` 验签，未配置时 webhook 会返回 `500`
- `COOKIE_SECURE`: 控制认证 Cookie 是否只允许在 HTTPS 传输；默认 development 为 `false`、production 为 `true`
- `COOKIE_SAME_SITE`: 认证 Cookie 的 SameSite 策略，可选 `lax` / `strict` / `none`
- `VITE_API_BASE_URL`: 可选。若设置则前端开发态直连该后端地址；未设置时默认走 Vite `/api` 代理
- `AUTO_MIGRATE`: 应用与启动脚本默认关闭；仅在显式设置为 `true` 时启用
- `SEED_DEMO_DATA`: 应用与启动脚本默认关闭；显式开启时，必须同时提供 `SEED_ADMIN_NAME`、`SEED_ADMIN_PHONE`、`SEED_ADMIN_PASSWORD` 与 `SEED_TECHNICIAN_PASSWORD`
- `SEED_ADMIN_NAME`: 开发态 demo 管理员名称
- `SEED_ADMIN_PHONE`: 开发态 demo 管理员登录手机号
- `SEED_ADMIN_PASSWORD`: 开发态 demo 管理员密码，最少 8 位；启用 demo seed 时会覆盖数据库中的管理员密码
- `SEED_TECHNICIAN_PASSWORD`: 开发态 demo 技师密码，最少 8 位
- `SERVE_STATIC`: 非 production 下若也要让 Go 托管前端静态资源，可设为 `true`
- `FRONTEND_DIST`: Go 托管的前端产物目录，默认 `../dist/client`
- `HTTP_READ_HEADER_TIMEOUT_SECONDS` / `HTTP_READ_TIMEOUT_SECONDS` / `HTTP_WRITE_TIMEOUT_SECONDS` / `HTTP_IDLE_TIMEOUT_SECONDS`: Go API 显式 HTTP 超时
- `HTTP_MAX_HEADER_BYTES`: 请求头大小上限
- `MAX_JSON_BODY_BYTES`: 普通 JSON 写接口请求体上限
- `MAX_WEBHOOK_BODY_BYTES`: LINE webhook 请求体上限

## 开发启动

安装前端依赖：

```bash
npm install --prefix client
```

脚本已内置 PostgreSQL Docker 启动逻辑：

```bash
./start.sh postgres
./start.sh frontend
./start.sh backend
./start.sh both
./start.sh check
```

- `./start.sh postgres`: 仅启动 PostgreSQL Docker 服务
- `./start.sh backend`: 启动 PostgreSQL 后再启动 Go API
- `./start.sh both`: 启动 PostgreSQL 与 Go API，再启动前端
- `./start.sh check`: 运行前端 `npm run check` 与后端 `go test ./...`

安全基线更新后，如需在开发态灌入 demo 数据，请先显式开启开关并准备 demo 账号密码：

```bash
export AUTO_MIGRATE=true
export SEED_DEMO_DATA=true
export SEED_ADMIN_NAME='管理員'
export SEED_ADMIN_PHONE='0912345678'
export SEED_ADMIN_PASSWORD='your-admin-password'
export SEED_TECHNICIAN_PASSWORD='your-tech-password'
./start.sh backend
```

若缺少上述演示账号配置而又启用了 `SEED_DEMO_DATA=true`，脚本会直接拒绝启动，这是预期行为。

只要启用了 `SEED_DEMO_DATA=true`，后端都会优先使用 `config.yaml` 或环境变量中的管理员名称、手机号与密码覆盖数据库中现有管理员账号，避免旧库残留账号与当前配置不一致。

如果本机已存在 `postgres:16-alpine` 镜像，脚本只会启动容器，不会重新拉取镜像；镜像缺失时才会按需下载。

仓库内的 PostgreSQL Compose 文件为 `docker-compose.postgres.yml`。脚本在不同环境下会自动给出数据目录默认值：

- macOS: `/Volumes/externalHard/data/cool-dispatch/postgresql`
- Linux / Ubuntu 22.04: 优先 `/srv/cool-dispatch/postgresql`，若当前用户无权写入则回退到 `$HOME/data/cool-dispatch/postgresql`

也可以手动运行：

```bash
set -a && source ./.env && set +a
docker compose -f docker-compose.postgres.yml up -d postgres
cd server && go run ./cmd/api
cd client && npm run dev
```

如果不导出 `.env`，Go 服务与 `start.sh` 都会回退到更安全的默认值：不开自动迁移、不灌演示数据。需要复用自定义 `DATABASE_URL`、`PORT`、`FRONTEND_ORIGIN` 或 seed 密码时，优先使用 `./start.sh` 或先 `source .env`。

LINE webhook 联调前还需要确认 `LINE_CHANNEL_SECRET` 与 LINE Developers 后台当前频道一致；否则即使请求体正确，也会因为 `X-Line-Signature` 验签失败返回 `401`。

开发模式下默认走 Vite `/api` 代理转发到 `http://localhost:9102`，因此本地与 Replit 预览都能使用同源请求。
若需要绕过代理直连其它后端，再显式设置 `VITE_API_BASE_URL`，此时后端会按 `FRONTEND_ORIGIN` 返回跨域响应头。

## 公开评价链接

- 评价外链现在统一使用预约上的随机 `review_token`，不再暴露自增预约 ID。
- 管理端复制出来的链接格式为：
  - `/review/<review_token>`
- 公开评价接口也同步改为基于 token：
  - `GET /api/reviews/token/:reviewToken/context`
  - `POST /api/reviews/token/:reviewToken`
  - `PATCH /api/reviews/token/:reviewToken/share-line`

## 构建验证

前端构建：

```bash
cd client && npm run build
```

前端类型检查：

```bash
cd client && npm run check
```

后端构建：

```bash
cd server && GOCACHE=$(pwd)/../.cache/go-build go build ./...
```

一键构建两端：

```bash
./start.sh build
```

一键执行当前审查基线校验：

```bash
./start.sh check
```

`start.sh` 会自动把 Go 运行所需目录固定到仓库内 `.cache/` 下，包括 `.cache/go-build`、`.cache/go-mod`、`.cache/go-path` 与 `.cache/home`，避免不同环境使用不可写系统缓存目录导致构建或测试失败。

生产模式下，先构建前端，再启动 `server/cmd/api`。Go 服务会直接托管 `dist/client`。

Replit 开发态不应通过 `start.sh both` 拉起 Docker PostgreSQL；`.replit` 已改为直接启动 Go API 与 Vite，并优先使用平台注入的 `DATABASE_URL`。

## 账号初始化与重置密码

非 demo 环境如果需要初始化管理员账号或重置现有用户密码，可直接使用：

```bash
cd server
go run ./cmd/useradmin --action upsert --phone 0912345678 --password 'new-admin-password' --name 管理员 --role admin
```

已存在用户只想重置密码时，可只提供手机号和新密码：

```bash
cd server
go run ./cmd/useradmin --action upsert --phone 0912345678 --password 'reset-password-123'
```

列出当前账号：

```bash
cd server
go run ./cmd/useradmin --action list
```

撤销单个账号全部登录态：

```bash
cd server
go run ./cmd/useradmin --action revoke-user --phone 0912345678
```

紧急情况下撤销全系统登录态：

```bash
cd server
go run ./cmd/useradmin --action revoke-all
```

说明：

- 创建新用户时：`--name` 与 `--role` 必填，`--user-id` 可选；省略时会自动分配下一个用户 ID
- 更新已有用户时：按手机号查找并更新密码，`--name` / `--role` 仅在显式传入时覆盖原值
- `--role` 仅支持 `admin` 或 `technician`
- `--action list` 会输出不含密码哈希的 JSON 账号摘要
- `--action revoke-user` / `--action revoke-all` 仅删除 `auth_tokens`，不会删除用户本身

## 默认演示数据

- Demo 账号手机号仍固定为管理员 `0912345678`、技师 `0987654321`
- Demo 账号密码不再写死在仓库内，必须由 `SEED_ADMIN_PASSWORD` 与 `SEED_TECHNICIAN_PASSWORD` 显式提供
- 仍内置 6 条 LINE 好友演示数据
