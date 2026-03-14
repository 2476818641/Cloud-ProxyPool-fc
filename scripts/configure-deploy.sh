#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
CONFIG_PATH="${1:-${ROOT_DIR}/config/deploy.toml}"
EXAMPLE_PATH="${ROOT_DIR}/config/deploy.toml.example"

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
    read -r -p "请输入阿里云地区，多个用逗号分隔（例如 cn-shanghai,cn-beijing）: " raw
    REGIONS=()
    IFS=',' read -r -a items <<< "$raw"
    for item in "${items[@]}"; do
      append_region "$item"
    done
    if [[ "${#REGIONS[@]}" -gt 0 ]]; then
      return
    fi
    echo "未输入有效地区，请重新填写。"
  done
}

collect_group_regions() {
  local raw choice
  while true; do
    echo "可选区域组："
    echo "  1. 亚太（中国）    : cn-shanghai, cn-hangzhou, cn-beijing, cn-shenzhen, cn-chengdu"
    echo "  2. 亚太（非中国）  : ap-northeast-1, ap-northeast-2, ap-southeast-1, ap-southeast-3, ap-southeast-5, ap-southeast-7"
    echo "  3. 欧美地区        : eu-central-1, eu-west-1, us-west-1, us-east-1"
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
    echo "请选择地区填写方式："
    echo "  1. 自定义地区（直接填写阿里云地区名，多个用逗号分隔）"
    echo "  2. 按区域组选择（亚太中国 / 亚太非中国 / 欧美地区）"
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

ensure_config_file() {
  if [[ -f "$CONFIG_PATH" ]]; then
    return
  fi

  mkdir -p "$(dirname "$CONFIG_PATH")"
  if [[ -f "$EXAMPLE_PATH" ]]; then
    cp "$EXAMPLE_PATH" "$CONFIG_PATH"
    return
  fi

  echo "未找到配置文件模板: $EXAMPLE_PATH" >&2
  exit 1
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

write_config() {
  local regions_toml backup_path
  regions_toml="$(build_regions_toml)"
  backup_path="${CONFIG_PATH}.bak.$(date +%Y%m%d%H%M%S)"
  cp "$CONFIG_PATH" "$backup_path"

  python3 - "$CONFIG_PATH" "$ACCESS_KEY_ID" "$ACCESS_KEY_SECRET" "$regions_toml" <<'PY'
import pathlib
import re
import sys

config_path = pathlib.Path(sys.argv[1])
access_key_id = sys.argv[2]
access_key_secret = sys.argv[3]
regions_toml = sys.argv[4]

text = config_path.read_text(encoding="utf-8")
patterns = {
    "access_key_id": rf'(^access_key_id\s*=\s*).*$',
    "access_key_secret": rf'(^access_key_secret\s*=\s*).*$',
    "regions": rf'(^regions\s*=\s*).*$',
}
replacements = {
    "access_key_id": rf'\1"{access_key_id}"',
    "access_key_secret": rf'\1"{access_key_secret}"',
    "regions": rf"\1{regions_toml}",
}

for key, pattern in patterns.items():
    updated, count = re.subn(pattern, replacements[key], text, flags=re.MULTILINE)
    if count != 1:
        raise SystemExit(f"配置文件中未能唯一定位 `{key}`，请检查 {config_path}")
    text = updated

config_path.write_text(text, encoding="utf-8")
PY

  echo "配置已写入: $CONFIG_PATH"
  echo "已备份旧配置: $backup_path"
}

confirm_summary() {
  local region_line
  region_line="$(IFS=', '; echo "${REGIONS[*]}")"
  echo
  echo "即将写入以下内容："
  echo "  Regions        : $region_line"
  echo "  AccessKey ID   : $ACCESS_KEY_ID"
  echo "  AccessKeySecret: 已填写"
  echo
  read -r -p "确认写入到 ${CONFIG_PATH} ? [Y/n]: " confirm
  confirm="$(trim "${confirm:-}")"
  if [[ -n "$confirm" && ! "$confirm" =~ ^[Yy]([Ee][Ss])?$ ]]; then
    echo "已取消。"
    exit 0
  fi
}

main() {
  ensure_config_file

  echo "=== cc-Alpha Docker 阿里云配置助手 ==="
  select_regions

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

  confirm_summary
  write_config
  echo "完成。现在可以执行: docker compose up -d --build"
}

main "$@"
