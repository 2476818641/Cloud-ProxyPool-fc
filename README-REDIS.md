# Cloud ProxyPool Alpha - Redis租借管理版本

## 📋 版本说明

**cc-Alpha** 是 Cloud ProxyPool 的升级版本，增加了 **Redis租借管理** 功能，支持多实例分布式部署。

---

## 🎯 核心特性

### 1. Redis租借系统
- **节点租借**：每次请求前通过Redis租借一个节点，避免多客户端冲突
- **冷却机制**：失败的节点自动进入冷却期，多实例共享失败状态
- **分布式协调**：多个客户端实例通过Redis协调，避免重复使用问题

### 2. 多实例支持
- ✅ 支持运行多个客户端实例
- ✅ 自动负载均衡
- ✅ 故障节点自动隔离
- ✅ 集中式状态管理

### 3. 降级机制
- 如果Redis不可用，自动降级到本地Round-Robin
- 保证服务可用性

---

## 🔄 与原版本对比

| 特性 | 原版本 (cc) | Alpha版本 (cc-Alpha) |
|------|-------------|----------------------|
| 节点选择 | 本地Round-Robin | Redis租借 + 本地降级 |
| 失败处理 | 本地计数器 | Redis共享冷却 |
| 多实例支持 | ❌ 不支持 | ✅ 完全支持 |
| 分布式部署 | ❌ 需手动协调 | ✅ 自动协调 |
| 负载均衡 | 本地轮询 | Redis租借 + 轮询 |
| 需要Redis | ❌ 不需要 | ✅ 需要（可选） |

---

## 🚀 快速开始

### 前提条件

1. **阿里云账户**（用于部署云函数）
2. **Redis实例**（本地或云端）

### 步骤1：准备Redis

#### 选项A：本地Redis（测试开发）

**使用Docker（推荐）：**
```bash
# Windows
docker run -d -p 6379:6379 --name redis redis:latest

# 验证连接
docker exec redis redis-cli ping
# 应该返回: PONG
```

**使用Windows版Redis：**
```bash
# 下载: https://github.com/microsoftarchive/redis/releases
redis-server.exe
```

#### 选项B：阿里云Redis（生产环境）

```bash
1. 访问阿里云控制台
2. 创建Redis实例
3. 获取连接地址和密码
4. 配置白名单
```

### 步骤2：配置deploy.toml

创建 `deploy.toml` 文件：

```toml
[aliyun]
access_key_id = "YOUR_ACCESS_KEY_ID"
access_key_secret = "YOUR_ACCESS_KEY_SECRET"

[deployment]
regions = ["cn-shanghai", "cn-beijing", "cn-shenzhen"]
function_name = "cloud_proxy_pool_func"
handler = "index.handler"
runtime = "python3.10"
timeout = 60
memory_size = 512
trigger_name = "http_trigger"
auth_type = "anonymous"
internet_access = true

[redis]
# Redis连接配置
addr = "127.0.0.1:6379"
password = ""
db = 0
key_prefix = "cloud_proxy_pool"

# 租借配置（秒）
lease_ttl_seconds = 120      # 租借时长，匹配云函数超时时间
cooldown_seconds = 120        # 失败节点冷却时长

# 重试配置
acquire_retries = 3           # 租借失败重试次数
retry_delay_ms = 200          # 重试延迟（毫秒）

[health_check]
enable = true
test_url = "http://myip.ipip.net"
expected_status = 200
```

### 步骤3：部署云函数

```bash
cd c:\Users\liuasd\Desktop\工具\cc-Alpha\deploy

# 安装依赖
pip install -r requirements.txt

# 复制配置文件
copy deploy.toml.example deploy.toml

# 编辑deploy.toml，填入AccessKey和Redis地址

# 部署（选择区域组 1,2,3）
python deploy.py
```

部署脚本会：
1. 打包云函数代码
2. 部署到选择的区域
3. 执行健康检查
4. 生成 `client/config.toml`（包含Redis配置）

### 步骤4：启动客户端

```bash
cd c:\Users\liuasd\Desktop\工具\cc-Alpha\client

# 方式1：使用Go运行
go run main.go

# 方式2：编译后运行
go build -o cloud-proxy.exe
./cloud-proxy.exe
```

启动后会看到：

```
    ██████╗ ██╗████████╗ ██████╗ ██████╗ ██╗ ██████╗███████╗
   ██╔════╝ ██║╚══██╔══╝██╔═══██╗██╔══██╗██║██╔════╝██╔════╝
   ...

================================================================
 [客户端] 监听地址  : 127.0.0.1:10800
 [云函数] 加载节点数: 10 个 Function URL
 [Redis]  租借管理  : 已启用 (127.0.0.1:6379)
 [状  态] 健康检查  : 通过 (PASS)
 [云  端] 当前出口IP: x.x.x.x (随请求自动轮换)
================================================================
```

---

## 🎮 使用场景

### 场景1：单实例使用（简单）

**配置：**
```toml
[cloud.redis]
addr = ""  # 留空则不使用Redis
```

**行为：**
- 降级到本地Round-Robin
- 功能与原版本相同

---

### 场景2：多实例部署（推荐）

**在多台机器上运行：**

```bash
# 机器1
cd c:\Users\liuasd\Desktop\工具\cc-Alpha\client
./cloud-proxy.exe -C config.toml

# 机器2
cd c:\Users\liuasd\Desktop\工具\cc-Alpha\client
./cloud-proxy.exe -C config.toml

# 机器3
...
```

**效果：**
- 所有实例共享Redis
- 自动负载均衡
- 失败节点全局隔离

---

### 场景3：MSMC多线程

**配置MSMC使用HTTP代理：**

```json
{
  "Settings": {
    "Threads": 1000,
    "ProxyType": "http",
    "ProxyFile": "127.0.0.1:10800"
  }
}
```

**优势：**
- 1000个并发请求
- 自动分配到多个云函数节点
- 避免单节点过载

---

## 📊 Redis数据结构

```
# 租借状态
cloud_proxy_pool:lease:{node_sha1}
  Value: {lease_token}
  TTL: 120秒

# 冷却状态
cloud_proxy_pool:cooldown:{node_sha1}
  Value: "1"
  TTL: 120秒
```

---

## ⚙️ 配置说明

### Redis租借参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `lease_ttl_seconds` | 120 | 租借时长（秒），应匹配云函数timeout |
| `cooldown_seconds` | 120 | 冷却时长（秒），失败节点隔离时间 |
| `acquire_retries` | 3 | 租借失败重试次数 |
| `retry_delay_ms` | 200 | 重试延迟（毫秒） |

### 推荐配置

**高并发场景：**
```toml
lease_ttl_seconds = 60
cooldown_seconds = 60
acquire_retries = 5
retry_delay_ms = 100
```

**低延迟场景：**
```toml
lease_ttl_seconds = 30
cooldown_seconds = 30
acquire_retries = 2
retry_delay_ms = 50
```

---

## 🔍 监控和调试

### 查看Redis状态

```bash
redis-cli
> KEYS cloud_proxy_pool:*
> TTL cloud_proxy_pool:lease:{node_sha1}
> GET cloud_proxy_pool:cooldown:{node_sha1}
```

### 客户端日志

启用调试模式：

```toml
[client]
debug = true
```

### Dashboard监控

访问 `http://localhost:8081` 查看实时统计。

---

## 🛠️ 故障排查

### 问题1：Redis连接失败

**症状：**
```
[warn] redis lease acquisition failed, falling back to local scheduler
```

**解决：**
1. 检查Redis是否运行：`redis-cli ping`
2. 检查配置：`addr` 和 `password`
3. 检查防火墙和端口

### 问题2：节点被锁死

**症状：**
```
all nodes are in cooldown
```

**解决：**
```bash
redis-cli
> DEL cloud_proxy_pool:cooldown:*
```

### 问题3：租借频繁失败

**症状：**
```
no redis lease available after 3 attempts
```

**解决：**
- 增加 `lease_ttl_seconds`
- 减少 `acquire_retries`
- 增加云函数节点数量

---

## 📈 性能优化

### 1. 调整租借时长

**原则：**
- 租借时长 = 云函数timeout + 10秒缓冲
- 太短：频繁租借，增加延迟
- 太长：节点长时间被占用

### 2. 调整冷却时长

**原则：**
- 冷却时长 = 节点恢复时间
- 太短：失败节点过早恢复
- 太长：可用节点减少

### 3. 调整重试参数

**原则：**
- 高并发：减少重试，快速降级
- 低并发：增加重试，提高成功率

---

## 🔄 迁移指南

### 从原版本迁移到Alpha

**步骤：**

1. 部署Redis
2. 修改 `deploy.toml`，添加Redis配置
3. 重新运行 `python deploy.py`
4. 使用新的 `client/config.toml`

**无需修改：**
- 云函数代码
- 客户端代码（自动兼容）

---

## 📝 注意事项

1. **Redis必须可用**：生产环境建议使用高可用Redis
2. **时钟同步**：多实例之间必须时钟同步
3. **配置一致性**：所有实例使用相同的Redis配置
4. **监控告警**：监控Redis连接和租借失败率
5. **备份恢复**：定期备份Redis配置

---

## 🆘 获取帮助

- **问题反馈**：GitHub Issues
- **文档更新**：查阅最新版本
- **示例配置**：参考 `deploy.toml.example`

---

## 📄 许可证

基于原版本修改，遵循原许可证。