package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"cloud-proxy-pool/config"
	"cloud-proxy-pool/debug"

	"github.com/fatih/color"
)

var (
	command     string
	configPath  string
	outputDir   string
	verbose     bool
	traceEnable bool
	proxyAddr   string
)

func init() {
	flag.StringVar(&configPath, "C", "config.toml", "配置文件路径")
	flag.StringVar(&outputDir, "o", "./debug_output", "调试输出目录")
	flag.BoolVar(&verbose, "v", false, "详细输出")
	flag.BoolVar(&traceEnable, "trace", false, "启用请求追踪")
	flag.StringVar(&proxyAddr, "proxy", "0.0.0.0:10800", "代理地址")
}

func main() {
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		printUsage()
		os.Exit(1)
	}

	command = args[0]

	switch command {
	case "diagnose":
		runDiagnose()
	case "trace":
		runTrace()
	case "inspect":
		runInspect()
	case "console":
		runConsole()
	case "test":
		runTest()
	case "capture":
		runCapture()
	default:
		color.Red("未知命令: %s", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Cloud ProxyPool 调试工具")
	fmt.Println()
	fmt.Println("用法:")
	fmt.Println("  cloud-proxy-debug <command> [options]")
	fmt.Println()
	fmt.Println("命令:")
	fmt.Println("  diagnose   - 运行完整诊断")
	fmt.Println("  trace      - 追踪请求")
	fmt.Println("  inspect    - 检查数据包")
	fmt.Println("  console    - 启动调试控制台")
	fmt.Println("  test       - 测试特定功能")
	fmt.Println("  capture    - 捕获流量")
	fmt.Println()
	fmt.Println("选项:")
	fmt.Println("  -C <path>      配置文件路径 (默认: config.toml)")
	fmt.Println("  -o <dir>       输出目录 (默认: ./debug_output)")
	fmt.Println("  -v             详细输出")
	fmt.Println("  -trace         启用请求追踪")
	fmt.Println("  -proxy <addr>  代理地址 (默认: 0.0.0.0:10800)")
	fmt.Println()
	fmt.Println("示例:")
	fmt.Println("  cloud-proxy-debug diagnose")
	fmt.Println("  cloud-proxy-debug trace -v")
	fmt.Println("  cloud-proxy-debug test -proxy 127.0.0.1:10800")
}

func runDiagnose() {
	conf, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	if len(conf.Cloud.FunctionURLs) == 0 {
		color.Red("错误: 配置中没有云函数URL")
		os.Exit(1)
	}

	diagTool := debug.NewDiagnosticTool(proxyAddr, conf.Cloud.FunctionURLs, verbose)
	_ = diagTool.RunFullDiagnostic()

	reportFile := fmt.Sprintf("%s/diagnostic_report_%s.json", outputDir, time.Now().Format("20060102_150405"))
	if err := diagTool.SaveReport(reportFile); err != nil {
		log.Printf("保存报告失败: %v", err)
	} else {
		color.Green("诊断报告已保存到: %s", reportFile)
	}
}

func runTrace() {
	tracer, err := debug.NewTracer(outputDir, traceEnable)
	if err != nil {
		log.Fatalf("创建追踪器失败: %v", err)
	}
	defer tracer.Close()

	color.Cyan("请求追踪已启动...")
	if traceEnable {
		color.Green("追踪输出目录: %s", outputDir)
	} else {
		color.Yellow("追踪模式: 仅内存记录")
	}

	simulateRequests(tracer)
}

func runInspect() {
	inspector := debug.NewPacketInspector(10240, verbose)

	color.Cyan("数据包检查器已启动")
	fmt.Println()

	conf, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	for i, url := range conf.Cloud.FunctionURLs {
		color.Yellow("检查云函数节点 %d/%d: %s", i+1, len(conf.Cloud.FunctionURLs), url)

		payload := map[string]interface{}{
			"method": "GET",
			"url":    "http://myip.ipip.net",
			"headers": map[string]string{
				"User-Agent": "CloudProxyPool-Inspector",
			},
			"body": "",
		}

		info := inspector.InspectCloudRequest(payload, "http://myip.ipip.net")
		printPacketInfo(info)
	}
}

func runConsole() {
	console := debug.NewDebugConsole(nil, nil)
	console.Start()
}

func runTest() {
	if len(flag.Args()) < 2 {
		color.Red("请指定要测试的功能")
		fmt.Println("可用功能: http, https, socks5, dns, tls, cloud, all")
		os.Exit(1)
	}

	testType := flag.Args()[1]

	conf, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	diagTool := debug.NewDiagnosticTool(proxyAddr, conf.Cloud.FunctionURLs, verbose)

	switch testType {
	case "http":
		testHTTP(diagTool)
	case "https":
		testHTTPS(diagTool)
	case "socks5":
		testSOCKS5(diagTool)
	case "dns":
		testDNS(diagTool)
	case "tls":
		testTLS(diagTool)
	case "cloud":
		testCloud(diagTool)
	case "all":
		testAll(diagTool)
	default:
		color.Red("未知的测试类型: %s", testType)
		os.Exit(1)
	}
}

func runCapture() {
	color.Cyan("流量捕获模式")
	fmt.Println("正在启动代理...")

	conf, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	conf.Client.Dump = true
	conf.Client.DumpFile = fmt.Sprintf("%s/traffic_capture_%s.log", outputDir, time.Now().Format("20060102_150405"))

	color.Green("流量捕获已启用，输出到: %s", conf.Client.DumpFile)
	fmt.Println("请手动配置并重启代理以应用更改")
}

func simulateRequests(tracer *debug.Tracer) {
	fmt.Println()
	fmt.Println("模拟请求测试...")

	requests := []struct {
		method string
		url    string
	}{
		{"GET", "http://httpbin.org/get"},
		{"GET", "http://httpbin.org/ip"},
		{"GET", "http://httpbin.org/headers"},
	}

	for _, req := range requests {
		color.Cyan("模拟: %s %s", req.method, req.url)

		requestID := tracer.TraceRequest(req.method, req.url, map[string]string{
			"User-Agent": "CloudProxyPool-Test",
		})

		tracer.TraceProxyRequest(requestID, req.url)

		time.Sleep(100 * time.Millisecond)

		tracer.TraceCloudRequest(requestID, "example.cloud.function", req.url)

		time.Sleep(200 * time.Millisecond)

		tracer.TraceCloudResponse(requestID, 200, 300*time.Millisecond)

		tracer.TraceProxyResponse(requestID, 200, 500*time.Millisecond)

		tracer.TraceClientResponse(requestID, 200, 600*time.Millisecond)

		time.Sleep(50 * time.Millisecond)
	}

	fmt.Println()
	color.Green("模拟请求完成")
}

func testHTTP(diagTool *debug.DiagnosticTool) {
	fmt.Println("测试 HTTP 代理...")
	success := diagTool.TestHTTPProxy()
	if success {
		color.Green("✓ HTTP 代理测试通过")
	} else {
		color.Red("✗ HTTP 代理测试失败")
	}
}

func testHTTPS(diagTool *debug.DiagnosticTool) {
	fmt.Println("测试 HTTPS 代理...")
	success := diagTool.TestHTTPSProxy()
	if success {
		color.Green("✓ HTTPS 代理测试通过")
	} else {
		color.Red("✗ HTTPS 代理测试失败")
	}
}

func testSOCKS5(diagTool *debug.DiagnosticTool) {
	fmt.Println("测试 SOCKS5 代理...")
	success := diagTool.TestSOCKS5Proxy()
	if success {
		color.Green("✓ SOCKS5 代理测试通过")
	} else {
		color.Yellow("○ SOCKS5 代理不可用")
	}
}

func testDNS(diagTool *debug.DiagnosticTool) {
	fmt.Println("测试 DNS 解析...")
	success := diagTool.TestDNSResolution()
	if success {
		color.Green("✓ DNS 解析测试通过")
	} else {
		color.Red("✗ DNS 解析测试失败")
	}
}

func testTLS(diagTool *debug.DiagnosticTool) {
	fmt.Println("测试 TLS 连接...")
	tlsTest := diagTool.TestTLS()

	successCount := 0
	for _, node := range tlsTest.CloudNodes {
		if node.Success {
			successCount++
			color.Green("✓ %s: %s", node.Host, node.CipherSuite)
		} else {
			color.Red("✗ %s: %s", node.Host, node.Error)
		}
	}

	if successCount == len(tlsTest.CloudNodes) {
		color.Green("✓ 所有 TLS 连接测试通过")
	} else if successCount > 0 {
		color.Yellow("○ 部分节点 TLS 连接失败")
	} else {
		color.Red("✗ 所有 TLS 连接测试失败")
	}
}

func testCloud(diagTool *debug.DiagnosticTool) {
	fmt.Println("测试云函数节点...")
	nodes := diagTool.TestCloudNodes()

	successCount := 0
	for _, node := range nodes {
		if node.Success {
			successCount++
			color.Green("✓ %s (%dms)", node.URL, node.LatencyMs)
		} else {
			color.Red("✗ %s: %s", node.URL, node.Error)
		}
	}

	if successCount == len(nodes) {
		color.Green("✓ 所有云函数节点测试通过")
	} else if successCount > 0 {
		color.Yellow("○ 部分云函数节点测试失败")
	} else {
		color.Red("✗ 所有云函数节点测试失败")
	}
}

func testAll(diagTool *debug.DiagnosticTool) {
	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("  运行所有测试")
	fmt.Println("========================================")
	fmt.Println()

	testHTTP(diagTool)
	testHTTPS(diagTool)
	testSOCKS5(diagTool)
	testDNS(diagTool)
	testTLS(diagTool)
	testCloud(diagTool)

	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("  测试完成")
	fmt.Println("========================================")
}

func printPacketInfo(info *debug.CloudPacketInfo) {
	fmt.Printf("  类型: %s\n", info.Type)
	fmt.Printf("  方法: %s\n", info.Method)
	fmt.Printf("  URL: %s\n", info.URL)
	fmt.Printf("  大小: %d bytes\n", info.PayloadSize)

	if info.Headers != nil {
		fmt.Println("  Headers:")
		for k, v := range info.Headers {
			fmt.Printf("    %s: %s\n", k, v)
		}
	}

	if len(info.PayloadPreview) > 0 {
		fmt.Printf("  预览: %s\n", info.PayloadPreview)
	}
	fmt.Println()
}
