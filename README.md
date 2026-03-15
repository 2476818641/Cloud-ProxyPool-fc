# cc-Alpha Docker Web

这个仓库是 `cc-Alpha` 的 Docker 打包版，内置了：

- `redis` 服务
- 阿里云 FC 自动部署脚本
- Go 代理客户端
- Web Dashboard

## 项目结构

- `client/`: Go 代理客户端、Dashboard、调试工具
- `server/`: 部署到阿里云 FC 的 Python 入口
- `scripts/`: 部署和配置脚本
- `config/`: 部署配置模板
- `runtime/`: 运行时生成文件、TLS 证书副本
- `web/`: React Dashboard 前端源码

## 当前启动流程

推荐使用：

```bash
./start.sh
```

`start.sh` 当前会按下面的顺序交互：

1. 默认启用 Web Dashboard
2. 询问 Web 端口，留空默认 `8081`
3. 询问是否启用 SSL
4. 如果启用 SSL，继续询问证书文件路径和私钥文件路径
5. 询问 Dashboard 登录用户名和密码
6. 如果 `config/deploy.toml` 里还没有可用区域，先补充阿里云部署区域
7. 询问阿里云 `AccessKey ID` 和 `AccessKey Secret`
8. 写回配置并执行 `docker compose up -d --build`

## SSL 说明

- 如果启用 SSL，脚本会把你提供的证书和私钥复制到 `runtime/tls/`
- 容器内默认使用：
  - `/app/runtime/tls/dashboard.crt`
  - `/app/runtime/tls/dashboard.key`
- 启用后 Dashboard 会走 HTTPS

## Dashboard 登录

- 用户名和密码由 `start.sh` 启动时输入
- 前端登录时会发送 SHA-256 十六进制字符串的 Base64
- 后端已兼容当前脚本的密码处理方式

## 端口

默认端口：

- `10800-10810`: HTTP 代理端口范围
- `10801`: SOCKS5
- `8081`: Dashboard 默认端口
- `6379`: Redis
- `80`: Nginx 反代端口，仅在 `nginx-proxy` profile 下启用

说明：

- Dashboard 容器内监听固定为 `8081`
- 宿主机端口由 `WEB_DASHBOARD_PORT` 决定，`start.sh` 会自动设置

## 配置文件

主配置文件是 [config/deploy.toml](/c:/Users/liuasd/Desktop/工具/cc-Alpha-docker-web/config/deploy.toml)。

关键字段：

- `[client]`: 本地监听地址、SOCKS、Dashboard
- `[aliyun]`: 阿里云 AccessKey
- `[deployment]`: 部署区域、函数名、运行时等
- `[redis]`: Redis 连接和分布式调度参数
- `[health_check]`: 部署后的健康检查

运行时生成的客户端配置位于：

- [runtime/client.toml](/c:/Users/liuasd/Desktop/工具/cc-Alpha-docker-web/runtime/client.toml)

## 手动配置区域

如果你只想先配置区域，可以运行：

```bash
./scripts/configure-deploy.sh --regions-only
```

如果想一次性填写区域和阿里云 Key，可以运行：

```bash
./scripts/configure-deploy.sh
```

## 常用命令

启动：

```bash
docker compose up -d --build
```

查看日志：

```bash
docker compose logs -f cloud-proxy
```

停止：

```bash
docker compose down
```

前端开发：

```bash
cd web
npm install
npm run dev
```

Go 客户端测试：

```bash
cd client
go test ./...
```

## 环境变量

`docker-compose.yml` 当前使用这些关键变量：

- `ENABLE_WEB_DASHBOARD`
- `WEB_DASHBOARD_PORT`
- `DASHBOARD_TLS_ENABLED`
- `DASHBOARD_TLS_CERT_FILE`
- `DASHBOARD_TLS_KEY_FILE`
- `DASHBOARD_USER`
- `DASHBOARD_PASSWORD`
- `JWT_SECRET`

## 故障排查

Dashboard 无法访问时，优先检查：

1. `docker compose ps`
2. `docker compose logs cloud-proxy`
3. 端口是否被占用
4. 如果启用了 SSL，确认 `runtime/tls/` 中的证书和私钥有效

登录失败时，优先检查：

1. 是否使用了启动脚本里设置的账号密码
2. 密码长度是否至少 8 位
3. 容器日志里是否有登录错误

## 备注

- `config/deploy.toml` 已在 `.gitignore` 中，避免误提交真实密钥
- 容器启动时默认会重新执行部署流程
- 当前仓库已经清理了测试产生的 `__pycache__`、`.pyc` 和本地构建产物
