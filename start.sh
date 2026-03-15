#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_PATH="${ROOT_DIR}/config/deploy.toml"
EXAMPLE_CONFIG_PATH="${ROOT_DIR}/config/deploy.toml.example"
TLS_DIR="${ROOT_DIR}/runtime/tls"

GROUP_AP_CHINA=(
  "cn-hangzhou"
  "cn-shanghai"
  "cn-qingdao"
  "cn-beijing"
  "cn-zhangjiakou"
  "cn-shenzhen"
  "cn-chengdu"
  "cn-hongkong"
)

GROUP_AP_OTHER=(
  "ap-northeast-1"
  "ap-northeast-2"
  "ap-southeast-1"
  "ap-southeast-3"
  "ap-southeast-5"
  "ap-southeast-7"
)

GROUP_EU_US=(
  "eu-central-1"
  "eu-west-1"
  "us-west-1"
  "us-east-1"
)

trim() {
  local value="$1"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf '%s' "$value"
}

python_cmd() {
  if command -v python3 >/dev/null 2>&1; then
    printf 'python3'
    return
  fi
  if command -v python >/dev/null 2>&1; then
    printf 'python'
    return
  fi

  echo "未找到 python3/python，无法自动写入配置。" >&2
  exit 1
}

ensure_config_file() {
  if [[ -f "$CONFIG_PATH" ]]; then
    return
  fi

  mkdir -p "$(dirname "$CONFIG_PATH")"
  cp "$EXAMPLE_CONFIG_PATH" "$CONFIG_PATH"
}

write_dashboard_addr() {
  local py
  py="$(python_cmd)"

  "$py" - "$CONFIG_PATH" <<'PY'
import pathlib
import re
import sys

config_path = pathlib.Path(sys.argv[1])
text = config_path.read_text(encoding="utf-8")
updated, count = re.subn(
    r'(^dashboard_addr\s*=\s*).*$',
    r'\1"0.0.0.0:8081"',
    text,
    flags=re.MULTILINE,
)
if count != 1:
    raise SystemExit(f"配置文件中未能唯一定位 dashboard_addr: {config_path}")
config_path.write_text(updated, encoding="utf-8")
PY
}

write_access_keys() {
  local py
  py="$(python_cmd)"

  "$py" - "$CONFIG_PATH" "$ALIYUN_ACCESS_KEY_ID" "$ALIYUN_ACCESS_KEY_SECRET" <<'PY'
import pathlib
import re
import sys

config_path = pathlib.Path(sys.argv[1])
access_key_id = sys.argv[2]
access_key_secret = sys.argv[3]
text = config_path.read_text(encoding="utf-8")

patterns = {
    "access_key_id": r'(^access_key_id\s*=\s*).*$',
    "access_key_secret": r'(^access_key_secret\s*=\s*).*$',
}
values = {
    "access_key_id": access_key_id,
    "access_key_secret": access_key_secret,
}

for key, pattern in patterns.items():
    updated, count = re.subn(
        pattern,
        rf'\1"{values[key]}"',
        text,
        flags=re.MULTILINE,
    )
    if count != 1:
        raise SystemExit(f"配置文件中未能唯一定位 {key}: {config_path}")
    text = updated

config_path.write_text(text, encoding="utf-8")
PY
}

append_region() {
  local value
  value="$(trim "$1")"
  if [[ -z "$value" ]]; then
    return
  fi

  local existing
  for existing in "${REGIONS[@]:-}"; do
    if [[ "$existing" == "$value" ]]; then
      return
    fi
  done

  REGIONS+=("$value")
}

build_regions_toml() {
  local joined="" region
  for region in "${REGIONS[@]}"; do
    if [[ -n "$joined" ]]; then
      joined+=", "
    fi
    joined+="\"$region\""
  done
  printf '[%s]' "$joined"
}

write_regions() {
  local py regions_toml
  py="$(python_cmd)"
  regions_toml="$(build_regions_toml)"

  "$py" - "$CONFIG_PATH" "$regions_toml" <<'PY'
import pathlib
import re
import sys

config_path = pathlib.Path(sys.argv[1])
regions_toml = sys.argv[2]
text = config_path.read_text(encoding="utf-8")
updated, count = re.subn(
    r'(^regions\s*=\s*).*$',
    rf'\1{regions_toml}',
    text,
    flags=re.MULTILINE,
)
if count != 1:
    raise SystemExit(f"配置文件中未能唯一定位 regions: {config_path}")
config_path.write_text(updated, encoding="utf-8")
PY
}

hash_dashboard_password() {
  local py
  py="$(python_cmd)"

  "$py" - "$1" <<'PY'
import base64
import hashlib
import sys

password = sys.argv[1]
digest = hashlib.sha256(password.encode("utf-8")).hexdigest().encode("ascii")
print(base64.b64encode(digest).decode("ascii"), end="")
PY
}

prompt_dashboard_port() {
  while true; do
    read -r -p "请输入 Web 端口 [8081]: " input_port
    input_port="$(trim "${input_port:-8081}")"

    if [[ "$input_port" =~ ^[0-9]+$ ]] && (( input_port >= 1 && input_port <= 65535 )); then
      WEB_DASHBOARD_PORT="$input_port"
      export WEB_DASHBOARD_PORT
      return
    fi

    echo "端口必须是 1-65535 之间的数字。"
  done
}

prompt_ssl_settings() {
  local ssl_choice cert_path key_path

  read -r -p "是否启用 SSL [y/N]: " ssl_choice
  ssl_choice="$(trim "${ssl_choice:-}")"

  if [[ "$ssl_choice" =~ ^[Yy]([Ee][Ss])?$ ]]; then
    while true; do
      read -r -p "请输入证书文件路径: " cert_path
      cert_path="$(trim "$cert_path")"
      if [[ -f "$cert_path" ]]; then
        break
      fi
      echo "证书文件不存在，请重新输入。"
    done

    while true; do
      read -r -p "请输入私钥文件路径: " key_path
      key_path="$(trim "$key_path")"
      if [[ -f "$key_path" ]]; then
        break
      fi
      echo "私钥文件不存在，请重新输入。"
    done

    mkdir -p "$TLS_DIR"
    cp "$cert_path" "${TLS_DIR}/dashboard.crt"
    cp "$key_path" "${TLS_DIR}/dashboard.key"

    export DASHBOARD_TLS_ENABLED="true"
    export DASHBOARD_TLS_CERT_FILE="/app/runtime/tls/dashboard.crt"
    export DASHBOARD_TLS_KEY_FILE="/app/runtime/tls/dashboard.key"
    DASHBOARD_SCHEME="https"
    echo "已启用 HTTPS，证书已复制到 runtime/tls/。"
    return
  fi

  rm -f "${TLS_DIR}/dashboard.crt" "${TLS_DIR}/dashboard.key"
  export DASHBOARD_TLS_ENABLED="false"
  export DASHBOARD_TLS_CERT_FILE="/app/runtime/tls/dashboard.crt"
  export DASHBOARD_TLS_KEY_FILE="/app/runtime/tls/dashboard.key"
  DASHBOARD_SCHEME="http"
  echo "已使用 HTTP 访问 Dashboard。"
}

prompt_dashboard_auth() {
  local dashboard_pass dashboard_pass_confirm

  while true; do
    read -r -p "请输入 Dashboard 用户名: " DASHBOARD_USER
    DASHBOARD_USER="$(trim "$DASHBOARD_USER")"
    if [[ -n "$DASHBOARD_USER" ]]; then
      export DASHBOARD_USER
      break
    fi
    echo "用户名不能为空。"
  done

  while true; do
    read -r -s -p "请输入 Dashboard 密码（至少 8 位）: " dashboard_pass
    echo
    if [[ ${#dashboard_pass} -ge 8 ]]; then
      break
    fi
    echo "密码长度至少为 8 位。"
  done

  read -r -s -p "请再次输入密码确认: " dashboard_pass_confirm
  echo
  if [[ "$dashboard_pass" != "$dashboard_pass_confirm" ]]; then
    echo "两次输入的密码不一致。"
    exit 1
  fi

  DASHBOARD_PASSWORD="$(hash_dashboard_password "$dashboard_pass")"
  export DASHBOARD_PASSWORD
}

prompt_aliyun_keys() {
  while true; do
    read -r -p "请输入阿里云 AccessKey ID: " ALIYUN_ACCESS_KEY_ID
    ALIYUN_ACCESS_KEY_ID="$(trim "$ALIYUN_ACCESS_KEY_ID")"
    if [[ -n "$ALIYUN_ACCESS_KEY_ID" ]]; then
      break
    fi
    echo "AccessKey ID 不能为空。"
  done

  while true; do
    read -r -s -p "请输入阿里云 AccessKey Secret: " ALIYUN_ACCESS_KEY_SECRET
    echo
    ALIYUN_ACCESS_KEY_SECRET="$(trim "$ALIYUN_ACCESS_KEY_SECRET")"
    if [[ -n "$ALIYUN_ACCESS_KEY_SECRET" ]]; then
      break
    fi
    echo "AccessKey Secret 不能为空。"
  done
}

prompt_regions() {
  local mode group_choice raw item region

  while true; do
    echo
    echo "请选择地区配置方式："
    echo "  1. 按区域选择"
    echo "  2. 手动填写城市"
    read -r -p "请输入选项 [1/2]: " mode
    mode="$(trim "$mode")"

    if [[ "$mode" == "1" ]]; then
      while true; do
        echo "请选择区域组："
        echo "  1. 亚太（中国）"
        echo "  2. 亚太（非中国）"
        echo "  3. 欧美"
        read -r -p "请输入选项 [1/2/3]: " group_choice
        group_choice="$(trim "$group_choice")"

        REGIONS=()
        case "$group_choice" in
          1)
            for region in "${GROUP_AP_CHINA[@]}"; do
              append_region "$region"
            done
            return
            ;;
          2)
            for region in "${GROUP_AP_OTHER[@]}"; do
              append_region "$region"
            done
            return
            ;;
          3)
            for region in "${GROUP_EU_US[@]}"; do
              append_region "$region"
            done
            return
            ;;
          *)
            echo "无效选项，请输入 1/2/3。"
            ;;
        esac
      done
    fi

    if [[ "$mode" == "2" ]]; then
      while true; do
        read -r -p "请输入城市（可多个，逗号分隔，如 cn-shanghai,cn-beijing）: " raw
        REGIONS=()
        IFS=',' read -r -a items <<< "$raw"
        for item in "${items[@]}"; do
          append_region "$item"
        done
        if [[ "${#REGIONS[@]}" -gt 0 ]]; then
          return
        fi
        echo "未输入有效城市，请重新输入。"
      done
    fi

    echo "无效选项，请输入 1 或 2。"
  done
}

main() {
  ensure_config_file

  export ENABLE_WEB_DASHBOARD="true"

  echo "=== cc-Alpha Docker 启动配置 ==="
  prompt_dashboard_port
  prompt_ssl_settings
  prompt_dashboard_auth

  write_dashboard_addr

  echo
  prompt_aliyun_keys
  write_access_keys

  prompt_regions
  write_regions

  echo
  echo "开始构建并启动 Docker 服务..."
  cd "$ROOT_DIR"
  docker compose up -d --build

  echo
  echo "服务已启动。"
  echo "Dashboard: ${DASHBOARD_SCHEME}://localhost:${WEB_DASHBOARD_PORT}"
  echo "查看日志: docker compose logs -f cloud-proxy"
  echo "停止服务: docker compose down"
}

main "$@"
