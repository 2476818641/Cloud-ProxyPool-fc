#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_PATH="${ROOT_DIR}/config/deploy.toml"
CONFIGURE_SCRIPT="${ROOT_DIR}/scripts/configure-deploy.sh"

need_configure="false"

if [[ ! -f "$CONFIG_PATH" ]]; then
  need_configure="true"
elif grep -q 'YOUR_ACCESS_KEY_ID\|YOUR_ACCESS_KEY_SECRET' "$CONFIG_PATH"; then
  need_configure="true"
elif ! grep -Eq '^regions\s*=\s*\[[^]]*[[:alnum:]]' "$CONFIG_PATH"; then
  need_configure="true"
fi

if [[ "$need_configure" == "true" ]]; then
  echo "检测到阿里云配置尚未完成，先进入交互式配置。"
  "$CONFIGURE_SCRIPT" "$CONFIG_PATH"
fi

echo "启动 Docker 服务..."
cd "$ROOT_DIR"
docker compose up -d --build

echo
echo "服务已启动。"
echo "查看日志: docker compose logs -f cloud-proxy"
echo "停止服务: docker compose down"
