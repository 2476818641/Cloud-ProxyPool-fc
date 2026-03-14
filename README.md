# cc-Alpha Docker

这个目录是 `cc-Alpha` 的 Docker 打包版，内置：

- `redis` 服务
- 阿里云 FC 自动部署脚本
- Cloud ProxyPool 客户端

容器启动流程是：

1. 读取 [config/deploy.toml](/root/vscode/cc-Alpha-docker/config/deploy.toml)
2. 根据阿里云 `AccessKey` 和 `regions` 部署/更新 FC
3. 生成运行期客户端配置到 `runtime/client.toml`
4. 启动本地 HTTP/SOCKS 代理

## 使用

先编辑 [config/deploy.toml](/root/vscode/cc-Alpha-docker/config/deploy.toml)：

- 填入 `access_key_id`
- 填入 `access_key_secret`
- 修改 `deployment.regions`
- 按需要修改 `client.listen_addr` 或 `client.listen_addrs`

如果你不想手改配置，可以先运行交互式脚本：

```bash
cd /root/vscode/cc-Alpha-docker
./scripts/configure-deploy.sh
```

脚本会：

- 让你选择“自定义地区”或“按区域组选择”
- 自定义地区支持用逗号填写多个，例如 `cn-shanghai,cn-beijing`
- 区域组支持选择：
  - 亚太（中国）
  - 亚太（非中国）
  - 欧美地区
- 最后再提示输入阿里云 `AccessKey ID` 和 `AccessKey Secret`
- 自动写入 [config/deploy.toml](/root/vscode/cc-Alpha-docker/config/deploy.toml)

也可以直接用一键启动脚本：

```bash
cd /root/vscode/cc-Alpha-docker
./start.sh
```

这个脚本会：

- 检查 [config/deploy.toml](/root/vscode/cc-Alpha-docker/config/deploy.toml)
- 如果发现阿里云 Key 还是占位值，自动调用 [configure-deploy.sh](/root/vscode/cc-Alpha-docker/scripts/configure-deploy.sh#L1)
- 配置完成后自动执行 `docker compose up -d --build`

如果你更喜欢手动启动，也可以直接运行：

```bash
cd /root/vscode/cc-Alpha-docker
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

## 端口

默认映射：

- `10800-10810`：HTTP 代理端口范围
- `8081`：Dashboard
- `6379`：内置 Redis

说明：

- 默认 `socks_addr = "0.0.0.0:10801"`，已经包含在 `10800-10810` 这个映射范围里。
- 如果你在 `listen_addrs` 里用了更大的端口范围，需要同步修改 [docker-compose.yml](/root/vscode/cc-Alpha-docker/docker-compose.yml) 的 `ports`。

## 配置文件说明

主要配置都放在 [config/deploy.toml](/root/vscode/cc-Alpha-docker/config/deploy.toml)：

- `[client]`：本地监听地址、是否开启多端口、SOCKS、Dashboard
- `[aliyun]`：阿里云 AccessKey
- `[deployment]`：要部署的 FC 区域和函数参数
- `[redis]`：内置 Redis 连接和租借参数
- `[health_check]`：部署后健康检查

生成后的客户端配置在：

- [runtime/client.toml](/root/vscode/cc-Alpha-docker/runtime/client.toml)

## 备注

- `config/deploy.toml` 已加入 `.gitignore`，避免误提交你的阿里云密钥。
- 容器默认每次启动都会重新执行一次部署流程；如果你只想复用已有 `runtime/client.toml`，可以把 `docker-compose.yml` 里的 `AUTO_DEPLOY_ON_START` 改成 `"false"`。
