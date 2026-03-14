# Cloud ProxyPool

Distributed HTTP/SOCKS proxy pool backed by Alibaba Cloud Function Compute.

Based on and modified from:
- https://github.com/25smoking/CloudProxyPool

## Layout

```text
client/   Go proxy client
server/   FC function code (Python)
deploy/   FC deployment script and config
```

## Alibaba Cloud FC

This project now targets Alibaba Cloud Function Compute 3.0.

- Runtime: `python3.10`
- Handler: `index.handler`
- Trigger: HTTP trigger with anonymous access
- Deploy SDK: `alibabacloud_fc20230330`

The function receives a JSON request like:

```json
{
  "method": "GET",
  "url": "http://example.com",
  "headers": {
    "User-Agent": "CloudProxyPool"
  },
  "body": "",
  "is_body_base64": false
}
```

The function returns:

```json
{
  "status_code": 200,
  "headers": {
    "content-type": "text/plain"
  },
  "content": "base64-encoded-response-body",
  "is_content_base64": true
}
```

## Deploy

```bash
cd deploy
pip install -r requirements.txt
copy deploy.toml.example deploy.toml
python deploy.py
```

Edit `deploy/deploy.toml` first:

```toml
[aliyun]
access_key_id = "YOUR_ACCESS_KEY_ID"
access_key_secret = "YOUR_ACCESS_KEY_SECRET"

[deployment]
# Optional fallback for non-interactive runs.
# In a normal terminal, deploy.py will prompt you to choose region groups.
regions = ["cn-shanghai", "cn-hangzhou", "cn-beijing", "cn-shenzhen"]
function_name = "cloud_proxy_pool_func"
handler = "index.handler"
runtime = "python3.10"
timeout = 60
memory_size = 512
trigger_name = "http_trigger"
auth_type = "anonymous"
internet_access = true
```

After deployment, `deploy.py` writes the generated function URLs into `client/config.toml`.

Interactive region groups:

- `1` Asia Pacific - China: Shanghai, Hangzhou, Beijing, Shenzhen, Chengdu
- `2` Asia Pacific - Other: Tokyo, Seoul, Singapore, Kuala Lumpur, Jakarta, Bangkok
- `3` Europe & Americas: Frankfurt, London, Silicon Valley, Virginia

The China group intentionally excludes Hohhot and Ulanqab.

## Run Client

```bash
cd client
go build
./cloud-proxy.exe -C config.toml
```

Default config examples point to FC HTTP trigger URLs such as:

```text
https://your-function.cn-shanghai.fc.aliyuncs.com
```

## Notes

- The Go client logic is cloud-vendor agnostic; only the deployed function URL format changed.
- `server/index.py` keeps `main_handler` as an alias for backward compatibility.
- The deployment script performs an HTTP health check before writing the node into client config.
