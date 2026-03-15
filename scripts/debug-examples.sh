#!/bin/bash

# Cloud ProxyPool 调试工具包 - 示例脚本
# 本脚本演示如何使用调试工具包进行各种诊断和测试

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 打印函数
print_header() {
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}========================================${NC}"
}

print_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

print_error() {
    echo -e "${RED}✗ $1${NC}"
}

print_warning() {
    echo -e "${YELLOW}○ $1${NC}"
}

# 检查命令是否存在
check_command() {
    if ! command -v $1 &> /dev/null; then
        print_error "$1 未找到，请先安装"
        exit 1
    fi
}

# 检查调试工具是否存在
check_debug_tool() {
    if [ ! -f "client/cloud-proxy-debug" ]; then
        print_warning "调试工具未找到，正在构建..."
        cd client
        go build -o cloud-proxy-debug cmd/debug.go
        cd ..
        print_success "调试工具构建完成"
    fi
}

# 创建输出目录
create_output_dir() {
    local output_dir=${1:-"debug_output"}
    if [ ! -d "$output_dir" ]; then
        mkdir -p "$output_dir"
        print_success "创建输出目录: $output_dir"
    fi
}

# 示例 1: 基本诊断
example_basic_diagnosis() {
    print_header "示例 1: 基本诊断"
    
    check_debug_tool
    create_output_dir
    
    echo "运行基本诊断..."
    ./client/cloud-proxy-debug diagnose
    
    echo ""
    print_success "诊断报告已保存到 debug_output/"
    echo ""
}

# 示例 2: 详细诊断
example_detailed_diagnosis() {
    print_header "示例 2: 详细诊断"
    
    check_debug_tool
    
    echo "运行详细诊断（包含更多输出）..."
    ./client/cloud-proxy-debug diagnose -v -o debug_output/detailed
    
    echo ""
    print_success "详细诊断报告已保存到 debug_output/detailed/"
    echo ""
}

# 示例 3: 特定功能测试
example_specific_tests() {
    print_header "示例 3: 特定功能测试"
    
    check_debug_tool
    
    echo "测试 HTTP 代理..."
    ./client/cloud-proxy-debug test http
    echo ""
    
    echo "测试 HTTPS 代理..."
    ./client/cloud-proxy-debug test https
    echo ""
    
    echo "测试云函数节点..."
    ./client/cloud-proxy-debug test cloud
    echo ""
    
    print_success "所有测试完成"
    echo ""
}

# 示例 4: 请求追踪
example_request_tracing() {
    print_header "示例 4: 请求追踪"
    
    check_debug_tool
    create_output_dir
    
    echo "启动请求追踪..."
    ./client/cloud-proxy-debug trace -v -o debug_output/tracing
    
    echo ""
    print_success "追踪数据已保存到 debug_output/tracing/"
    
    if [ -f "debug_output/tracing/trace_*.jsonl" ]; then
        echo ""
        echo "追踪数据示例："
        head -n 3 debug_output/tracing/trace_*.jsonl 2>/dev/null | while read line; do
            echo "  $line" | jq -r 'select(.stage != null) | "\(.stage): \(.url // "N/A")"'
        done
    fi
    echo ""
}

# 示例 5: 数据包检查
example_packet_inspection() {
    print_header "示例 5: 数据包检查"
    
    check_debug_tool
    
    echo "检查数据包..."
    ./client/cloud-proxy-debug inspect -v
    
    echo ""
    print_success "数据包检查完成"
    echo ""
}

# 示例 6: 流量捕获
example_traffic_capture() {
    print_header "示例 6: 流量捕获"
    
    check_debug_tool
    create_output_dir
    
    echo "启用流量捕获模式..."
    ./client/cloud-proxy-debug capture -o debug_output/capture
    
    echo ""
    print_warning "请在配置文件中设置 dump = true 并重启代理"
    echo "流量将被捕获到 debug_output/capture/"
    echo ""
}

# 示例 7: 交互式控制台
example_interactive_console() {
    print_header "示例 7: 交互式控制台"
    
    check_debug_tool
    
    echo "启动调试控制台..."
    echo "可用命令: help, status, nodes, test, trace, logs, exit"
    echo ""
    
    read -p "按 Enter 键启动控制台，或 Ctrl+C 退出..."
    
    ./client/cloud-proxy-debug console
}

# 示例 8: 分析诊断报告
example_analyze_report() {
    print_header "示例 8: 分析诊断报告"
    
    if ! command -v jq &> /dev/null; then
        print_warning "jq 未安装，跳过 JSON 分析"
        return
    fi
    
    local report_file=$(find debug_output -name "diagnostic_report_*.json" -type f | head -n 1)
    
    if [ -z "$report_file" ]; then
        print_warning "未找到诊断报告，请先运行诊断"
        return
    fi
    
    echo "分析报告: $report_file"
    echo ""
    
    echo "代理健康状态:"
    cat "$report_file" | jq '.proxy_health'
    echo ""
    
    echo "云函数节点状态:"
    cat "$report_file" | jq '.cloud_nodes[] | {url, success, latency_ms}'
    echo ""
    
    echo "代理功能测试:"
    cat "$report_file" | jq '.proxy_functionality'
    echo ""
    
    print_success "分析完成"
    echo ""
}

# 示例 9: 延迟分析
example_latency_analysis() {
    print_header "示例 9: 延迟分析"
    
    if ! command -v jq &> /dev/null; then
        print_warning "jq 未安装，跳过延迟分析"
        return
    fi
    
    local trace_file=$(find debug_output -name "trace_*.jsonl" -type f | head -n 1)
    
    if [ -z "$trace_file" ]; then
        print_warning "未找到追踪文件，请先运行追踪"
        return
    fi
    
    echo "分析追踪文件: $trace_file"
    echo ""
    
    echo "各阶段请求数量:"
    cat "$trace_file" | jq -r '.stage' | sort | uniq -c
    echo ""
    
    echo "平均延迟 (ms):"
    cat "$trace_file" | jq -r 'select(.latency_ms != null) | .latency_ms' | \
        awk '{sum+=$1; count++} END {if (count>0) print sum/count}'
    echo ""
    
    echo "最大延迟 (ms):"
    cat "$trace_file" | jq -r 'select(.latency_ms != null) | .latency_ms' | \
        awk 'BEGIN {max=0} {if ($1>max) max=$1} END {print max}'
    echo ""
    
    print_success "分析完成"
    echo ""
}

# 示例 10: 批量测试多个节点
example_batch_test_nodes() {
    print_header "示例 10: 批量测试多个节点"
    
    check_debug_tool
    
    echo "测试所有云函数节点..."
    ./client/cloud-proxy-debug test cloud -v
    
    echo ""
    print_success "批量测试完成"
    echo ""
}

# 主菜单
show_menu() {
    echo ""
    echo "Cloud ProxyPool 调试工具包 - 示例脚本"
    echo ""
    echo "请选择要运行的示例:"
    echo "  1. 基本诊断"
    echo "  2. 详细诊断"
    echo "  3. 特定功能测试"
    echo "  4. 请求追踪"
    echo "  5. 数据包检查"
    echo "  6. 流量捕获"
    echo "  7. 交互式控制台"
    echo "  8. 分析诊断报告"
    echo "  9. 延迟分析"
    echo " 10. 批量测试节点"
    echo "  a. 运行所有示例"
    echo "  q. 退出"
    echo ""
}

# 主函数
main() {
    print_header "Cloud ProxyPool 调试工具包"
    
    if [ $# -eq 0 ]; then
        show_menu
        read -p "请输入选项: " choice
    else
        choice=$1
    fi
    
    case $choice in
        1)
            example_basic_diagnosis
            ;;
        2)
            example_detailed_diagnosis
            ;;
        3)
            example_specific_tests
            ;;
        4)
            example_request_tracing
            ;;
        5)
            example_packet_inspection
            ;;
        6)
            example_traffic_capture
            ;;
        7)
            example_interactive_console
            ;;
        8)
            example_analyze_report
            ;;
        9)
            example_latency_analysis
            ;;
        10)
            example_batch_test_nodes
            ;;
        a|A)
            example_basic_diagnosis
            example_detailed_diagnosis
            example_specific_tests
            example_request_tracing
            example_packet_inspection
            example_traffic_capture
            example_analyze_report
            example_latency_analysis
            example_batch_test_nodes
            ;;
        q|Q)
            echo "退出"
            exit 0
            ;;
        *)
            print_error "无效选项: $choice"
            exit 1
            ;;
    esac
    
    print_success "示例执行完成"
}

# 运行主函数
main "$@"