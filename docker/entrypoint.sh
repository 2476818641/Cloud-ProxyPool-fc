#!/bin/sh
set -eu

CONFIG_PATH="${DEPLOY_CONFIG_FILE:-/app/config/deploy.toml}"
CLIENT_PATH="${CLIENT_CONFIG_PATH:-/app/runtime/client.toml}"
AUTO_DEPLOY="${AUTO_DEPLOY_ON_START:-true}"

mkdir -p "$(dirname "$CLIENT_PATH")"

if [ ! -f "$CONFIG_PATH" ]; then
  echo "[error] missing config file: $CONFIG_PATH"
  echo "[hint] edit /root/vscode/cc-Alpha-docker/config/deploy.toml on the host first"
  exit 1
fi

if [ "$AUTO_DEPLOY" = "true" ] || [ ! -s "$CLIENT_PATH" ]; then
  echo "[entrypoint] deploying Alibaba Cloud FC functions and generating client config..."
  export DEPLOY_CONFIG_FILE="$CONFIG_PATH"
  export CLIENT_CONFIG_PATH="$CLIENT_PATH"
  python /app/deploy/deploy.py
else
  echo "[entrypoint] skipping deploy, reusing existing client config: $CLIENT_PATH"
fi

echo "[entrypoint] starting proxy client..."
exec /usr/local/bin/cloud-proxy -C "$CLIENT_PATH"
