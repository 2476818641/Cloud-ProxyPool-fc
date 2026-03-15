#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
CONFIG_PATH="${ROOT_DIR}/config/deploy.toml"
EXAMPLE_PATH="${ROOT_DIR}/config/deploy.toml.example"
REGIONS_ONLY="false"

GROUP_AP_CHINA=(
  "cn-shanghai"
  "cn-hangzhou"
  "cn-beijing"
  "cn-shenzhen"
  "cn-chengdu"
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

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --regions-only)
        REGIONS_ONLY="true"
        shift
        ;;
      *)
        CONFIG_PATH="$1"
        shift
        ;;
    esac
  done
}

ensure_config_file() {
  if [[ -f "$CONFIG_PATH" ]]; then
    return
  fi

  mkdir -p "$(dirname "$CONFIG_PATH")"
  cp "$EXAMPLE_PATH" "$CONFIG_PATH"
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

collect_custom_regions() {
  local raw item
  while true; do
    read -r -p "请输入阿里云区域，多个用逗号分隔（例如 cn-shanghai,cn-beijing）: " raw
    REGIONS=()
    IFS=',' read -r -a items <<< "$raw"
    for item in "${items[@]}"; do
      append_region "$item"
    done
    if [[ "${#REGIONS[@]}" -gt 0 ]]; then
      return
    fi
    echo "未输入有效区域，请重新填写。"
  done
}

collect_group_regions() {
  local raw choice
  while true; do
    echo "可选区域组："
    echo "  1. 亚太（中国）      : cn-shanghai, cn-hangzhou, cn-beijing, cn-shenzhen, cn-chengdu"
    echo "  2. 亚太（非中国）    : ap-northeast-1, ap-northeast-2, ap-southeast-1, ap-southeast-3, ap-southeast-5, ap-southeast-7"
    echo "  3. 欧洲与美洲        : eu-central-1, eu-west-1, us-west-1, us-east-1"
    read -r -p "请输入分组编号，多个用逗号分隔（例如 1,3）: " raw

    REGIONS=()
    IFS=',' read -r -a choices <<< "$raw"
    for choice in "${choices[@]}"; do
      choice="$(trim "$choice")"
      case "$choice" in
        1)
          for region in "${GROUP_AP_CHINA[@]}"; do
            append_region "$region"
          done
          ;;
        2)
          for region in "${GROUP_AP_OTHER[@]}"; do
            append_region "$region"
          done
          ;;
        3)
          for region in "${GROUP_EU_US[@]}"; do
            append_region "$region"
          done
          ;;
        "")
          ;;
        *)
          echo "无效选项: $choice"
          REGIONS=()
          break
          ;;
      esac
    done

    if [[ "${#REGIONS[@]}" -gt 0 ]]; then
      return
    fi
    echo "未选择有效区域组，请重新选择。"
  done
}

select_regions() {
  local mode
  while true; do
    echo "请选择区域填写方式："
    echo "  1. 自定义区域"
    echo "  2. 按区域组选择"
    read -r -p "请输入选项 [1/2]: " mode
    case "$(trim "$mode")" in
      1)
        collect_custom_regions
        return
        ;;
      2)
        collect_group_regions
        return
        ;;
      *)
        echo "无效选项，请输入 1 或 2。"
        ;;
    esac
  done
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

prompt_access_keys() {
  while true; do
    read -r -p "请输入阿里云 AccessKey ID: " ACCESS_KEY_ID
    ACCESS_KEY_ID="$(trim "$ACCESS_KEY_ID")"
    if [[ -n "$ACCESS_KEY_ID" ]]; then
      break
    fi
    echo "AccessKey ID 不能为空。"
  done

  while true; do
    read -r -s -p "请输入阿里云 AccessKey Secret: " ACCESS_KEY_SECRET
    echo
    ACCESS_KEY_SECRET="$(trim "$ACCESS_KEY_SECRET")"
    if [[ -n "$ACCESS_KEY_SECRET" ]]; then
      break
    fi
    echo "AccessKey Secret 不能为空。"
  done
}

write_config() {
  local py regions_toml
  py="$(python_cmd)"
  regions_toml="$(build_regions_toml)"

  if [[ "$REGIONS_ONLY" == "true" ]]; then
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
    return
  fi

  "$py" - "$CONFIG_PATH" "$ACCESS_KEY_ID" "$ACCESS_KEY_SECRET" "$regions_toml" <<'PY'
import pathlib
import re
import sys

config_path = pathlib.Path(sys.argv[1])
access_key_id = sys.argv[2]
access_key_secret = sys.argv[3]
regions_toml = sys.argv[4]
text = config_path.read_text(encoding="utf-8")

patterns = {
    "access_key_id": r'(^access_key_id\s*=\s*).*$',
    "access_key_secret": r'(^access_key_secret\s*=\s*).*$',
    "regions": r'(^regions\s*=\s*).*$',
}
values = {
    "access_key_id": f'"{access_key_id}"',
    "access_key_secret": f'"{access_key_secret}"',
    "regions": regions_toml,
}

for key, pattern in patterns.items():
    updated, count = re.subn(pattern, rf'\1{values[key]}', text, flags=re.MULTILINE)
    if count != 1:
        raise SystemExit(f"配置文件中未能唯一定位 {key}: {config_path}")
    text = updated

config_path.write_text(text, encoding="utf-8")
PY
}

main() {
  parse_args "$@"
  ensure_config_file

  echo "=== cc-Alpha Docker 阿里云配置助手 ==="
  select_regions

  if [[ "$REGIONS_ONLY" == "false" ]]; then
    prompt_access_keys
  fi

  write_config
  echo "配置已写入: $CONFIG_PATH"
}

main "$@"
