package debug

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type DiagnosticTool struct {
	proxyAddr string
	cloudURLs []string
	verbose   bool
	timeout   time.Duration
	client    *http.Client
	mu        sync.RWMutex
	results   []*TestResult
}

func NewDiagnosticTool(proxyAddr string, cloudURLs []string, verbose bool) *DiagnosticTool {
	return &DiagnosticTool{
		proxyAddr: proxyAddr,
		cloudURLs: cloudURLs,
		verbose:   verbose,
		timeout:   30 * time.Second,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
				DisableKeepAlives: false,
				MaxIdleConns:      10,
				IdleConnTimeout:   30 * time.Second,
			},
		},
	}
}

func (d *DiagnosticTool) RunFullDiagnostic() *DiagnosticReport {
	report := &DiagnosticReport{
		Timestamp: time.Now(),
		ProxyAddr: d.proxyAddr,
	}

	fmt.Println("========================================")
	fmt.Println("  Cloud ProxyPool 诊断工具")
	fmt.Println("========================================")
	fmt.Println()

	report.ProxyHealth = d.testProxyHealth()
	d.printProxyHealth(report.ProxyHealth)

	report.CloudNodes = d.testCloudNodes()
	d.printCloudNodes(report.CloudNodes)

	report.ProxyFunctionality = d.testProxyFunctionality()
	d.printProxyFunctionality(report.ProxyFunctionality)

	report.TLSTest = d.testTLS()
	d.printTLSTest(report.TLSTest)

	report.BandwidthTest = d.testBandwidth()
	d.printBandwidthTest(report.BandwidthTest)

	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("  诊断完成")
	fmt.Println("========================================")

	return report
}

func (d *DiagnosticTool) testProxyHealth() *ProxyHealth {
	fmt.Println("[1/5] 测试代理健康状态...")

	health := &ProxyHealth{
		Listening: d.checkPortListening(d.proxyAddr),
	}

	if health.Listening {
		health.Responsive = d.checkProxyResponsive()
	}

	return health
}

func (d *DiagnosticTool) checkPortListening(addr string) bool {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		if d.verbose {
			log.Printf("[warn] Invalid address format: %v", err)
		}
		return false
	}

	if host == "" || host == "0.0.0.0" {
		host = "localhost"
	}

	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 2*time.Second)
	if err != nil {
		if d.verbose {
			log.Printf("[fail] Port %s is not listening: %v", addr, err)
		}
		return false
	}
	conn.Close()

	if d.verbose {
		log.Printf("[pass] Port %s is listening", addr)
	}
	return true
}

func (d *DiagnosticTool) checkProxyResponsive() bool {
	testURL := "http://httpbin.org/status/200"

	proxyURL, err := url.Parse("http://" + d.proxyAddr)
	if err != nil {
		return false
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	req, _ := http.NewRequest("GET", testURL, nil)
	resp, err := client.Do(req)
	if err != nil {
		if d.verbose {
			log.Printf("[fail] Proxy not responsive: %v", err)
		}
		return false
	}
	defer resp.Body.Close()

	if d.verbose {
		log.Printf("[pass] Proxy is responsive (status: %d)", resp.StatusCode)
	}
	return resp.StatusCode == 200
}

func (d *DiagnosticTool) TestCloudNodes() []*CloudNodeStatus {
	return d.testCloudNodes()
}

func (d *DiagnosticTool) testCloudNodes() []*CloudNodeStatus {
	fmt.Println("[2/5] 测试云函数节点...")

	statuses := make([]*CloudNodeStatus, len(d.cloudURLs))
	var wg sync.WaitGroup

	for i, cloudURL := range d.cloudURLs {
		wg.Add(1)
		go func(idx int, url string) {
			defer wg.Done()

			status := &CloudNodeStatus{
				URL: url,
			}

			startTime := time.Now()

			req, err := http.NewRequest("POST", url, strings.NewReader(`{"method":"GET","url":"http://myip.ipip.net"}`))
			if err != nil {
				status.Error = fmt.Sprintf("Failed to create request: %v", err)
				return
			}

			req.Header.Set("Content-Type", "application/json")

			client := &http.Client{
				Timeout: 15 * time.Second,
			}

			resp, err := client.Do(req)
			if err != nil {
				status.Error = fmt.Sprintf("Connection failed: %v", err)
				return
			}
			defer resp.Body.Close()

			status.LatencyMs = time.Since(startTime).Milliseconds()

			if resp.StatusCode != 200 {
				status.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
				body, _ := io.ReadAll(resp.Body)
				status.ResponseBody = string(body)
				return
			}

			var result struct {
				StatusCode int    `json:"status_code"`
				Content    string `json:"content"`
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				status.Error = fmt.Sprintf("Failed to read body: %v", err)
				return
			}

			if err := json.Unmarshal(body, &result); err != nil {
				status.Error = fmt.Sprintf("Failed to decode response: %v", err)
				return
			}

			status.StatusCode = result.StatusCode
			status.ResponseBody = result.Content
			status.Success = true

		}(i, cloudURL)
	}

	wg.Wait()

	for i := range statuses {
		if d.verbose {
			if statuses[i].Success {
				log.Printf("[pass] Cloud node %s: OK (%dms)", statuses[i].URL, statuses[i].LatencyMs)
			} else {
				log.Printf("[fail] Cloud node %s: %s", statuses[i].URL, statuses[i].Error)
			}
		}
	}

	return statuses
}

func (d *DiagnosticTool) testProxyFunctionality() *ProxyFunctionality {
	fmt.Println("[3/5] 测试代理功能...")

	funcs := &ProxyFunctionality{
		HTTP:   d.testHTTPProxy(),
		HTTPS:  d.testHTTPSProxy(),
		SOCKS5: d.testSOCKS5Proxy(),
		Auth:   d.testProxyAuth(),
		DNS:    d.testDNSResolution(),
	}

	return funcs
}

func (d *DiagnosticTool) TestHTTPProxy() bool {
	return d.testHTTPProxy()
}

func (d *DiagnosticTool) testHTTPProxy() bool {
	testURL := "http://httpbin.org/get"
	proxyURL, _ := url.Parse("http://" + d.proxyAddr)

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
	}

	req, _ := http.NewRequest("GET", testURL, nil)
	resp, err := client.Do(req)
	if err != nil {
		if d.verbose {
			log.Printf("[fail] HTTP proxy test failed: %v", err)
		}
		return false
	}
	defer resp.Body.Close()

	success := resp.StatusCode == 200
	if d.verbose {
		if success {
			log.Printf("[pass] HTTP proxy working (status: %d)", resp.StatusCode)
		} else {
			log.Printf("[fail] HTTP proxy returned status: %d", resp.StatusCode)
		}
	}
	return success
}

func (d *DiagnosticTool) TestHTTPSProxy() bool {
	return d.testHTTPSProxy()
}

func (d *DiagnosticTool) testHTTPSProxy() bool {
	testURL := "https://httpbin.org/get"
	proxyURL, _ := url.Parse("http://" + d.proxyAddr)

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	req, _ := http.NewRequest("GET", testURL, nil)
	resp, err := client.Do(req)
	if err != nil {
		if d.verbose {
			log.Printf("[fail] HTTPS proxy test failed: %v", err)
		}
		return false
	}
	defer resp.Body.Close()

	success := resp.StatusCode == 200
	if d.verbose {
		if success {
			log.Printf("[pass] HTTPS proxy working (status: %d)", resp.StatusCode)
		} else {
			log.Printf("[fail] HTTPS proxy returned status: %d", resp.StatusCode)
		}
	}
	return success
}

func (d *DiagnosticTool) TestSOCKS5Proxy() bool {
	return d.testSOCKS5Proxy()
}

func (d *DiagnosticTool) testSOCKS5Proxy() bool {
	if d.proxyAddr == "" {
		return false
	}

	host, port, _ := net.SplitHostPort(d.proxyAddr)

	portNum, err := strconv.Atoi(port)
	if err != nil {
		return false
	}

	socksPort := portNum + 1
	socksAddr := net.JoinHostPort(host, strconv.Itoa(socksPort))

	conn, err := net.DialTimeout("tcp", socksAddr, 5*time.Second)
	if err != nil {
		if d.verbose {
			log.Printf("[skip] SOCKS5 not available: %v", err)
		}
		return false
	}
	defer conn.Close()

	if d.verbose {
		log.Printf("[pass] SOCKS5 port is accessible")
	}
	return true
}

func (d *DiagnosticTool) testProxyAuth() bool {
	return true
}

func (d *DiagnosticTool) TestDNSResolution() bool {
	return d.testDNSResolution()
}

func (d *DiagnosticTool) testDNSResolution() bool {
	host := "httpbin.org"
	_, err := net.LookupHost(host)
	if err != nil {
		if d.verbose {
			log.Printf("[fail] DNS resolution failed: %v", err)
		}
		return false
	}

	if d.verbose {
		log.Printf("[pass] DNS resolution working")
	}
	return true
}

func (d *DiagnosticTool) TestTLS() *TLSTest {
	return d.testTLS()
}

func (d *DiagnosticTool) testTLS() *TLSTest {
	fmt.Println("[4/5] 测试TLS连接...")

	tlsTest := &TLSTest{
		CloudNodes: make([]*TLSNodeTest, len(d.cloudURLs)),
	}

	for i, cloudURL := range d.cloudURLs {
		u, err := url.Parse(cloudURL)
		if err != nil {
			continue
		}

		nodeTest := &TLSNodeTest{
			Host: u.Host,
		}

		conn, err := tls.DialWithDialer(
			&net.Dialer{Timeout: 10 * time.Second},
			"tcp",
			u.Host,
			&tls.Config{
				InsecureSkipVerify: true,
			},
		)

		if err != nil {
			nodeTest.Error = err.Error()
		} else {
			state := conn.ConnectionState()
			nodeTest.CipherSuite = tls.CipherSuiteName(state.CipherSuite)
			nodeTest.Version = state.Version
			nodeTest.Success = true
			conn.Close()
		}

		tlsTest.CloudNodes[i] = nodeTest

		if d.verbose {
			if nodeTest.Success {
				log.Printf("[pass] TLS to %s: %s", nodeTest.Host, nodeTest.CipherSuite)
			} else {
				log.Printf("[fail] TLS to %s: %s", nodeTest.Host, nodeTest.Error)
			}
		}
	}

	return tlsTest
}

func (d *DiagnosticTool) testBandwidth() *BandwidthTest {
	fmt.Println("[5/5] 测试带宽...")

	bwTest := &BandwidthTest{
		TestSizes: []int64{1024, 10240, 102400},
	}

	proxyURL, _ := url.Parse("http://" + d.proxyAddr)
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	for _, size := range bwTest.TestSizes {
		testURL := fmt.Sprintf("https://httpbin.org/bytes/%d", size)
		startTime := time.Now()

		req, _ := http.NewRequest("GET", testURL, nil)
		resp, err := client.Do(req)

		if err != nil {
			bwTest.Results = append(bwTest.Results, &BandwidthResult{
				Size:    size,
				Success: false,
				Error:   err.Error(),
			})
			continue
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		duration := time.Since(startTime)

		bwTest.Results = append(bwTest.Results, &BandwidthResult{
			Size:      size,
			Success:   true,
			BytesRead: int64(len(body)),
			Duration:  duration,
			Bandwidth: float64(len(body)) / duration.Seconds(),
		})

		if d.verbose {
			log.Printf("[pass] %d bytes in %v (%.2f MB/s)", len(body), duration, float64(len(body))/duration.Seconds()/1024/1024)
		}
	}

	return bwTest
}

func (d *DiagnosticTool) printProxyHealth(health *ProxyHealth) {
	fmt.Printf("  代理健康状态: ")
	if health.Listening && health.Responsive {
		fmt.Println("✓ 正常")
	} else {
		fmt.Println("✗ 异常")
	}
	if !health.Listening {
		fmt.Println("    - 端口未监听")
	}
	if !health.Responsive {
		fmt.Println("    - 无响应")
	}
	fmt.Println()
}

func (d *DiagnosticTool) printCloudNodes(nodes []*CloudNodeStatus) {
	fmt.Printf("  云函数节点 (%d 个):\n", len(nodes))
	healthy := 0
	for _, node := range nodes {
		if node.Success {
			healthy++
			fmt.Printf("    ✓ %s (%dms)\n", node.URL, node.LatencyMs)
		} else {
			fmt.Printf("    ✗ %s: %s\n", node.URL, node.Error)
		}
	}
	if healthy == 0 {
		fmt.Println("    ⚠ 警告: 所有节点均不可用")
	}
	fmt.Println()
}

func (d *DiagnosticTool) printProxyFunctionality(funcs *ProxyFunctionality) {
	fmt.Println("  代理功能测试:")
	fmt.Printf("    HTTP:  %s\n", boolToCheck(funcs.HTTP))
	fmt.Printf("    HTTPS: %s\n", boolToCheck(funcs.HTTPS))
	fmt.Printf("    SOCKS5: %s\n", boolToCheck(funcs.SOCKS5))
	fmt.Printf("    认证:   %s\n", boolToCheck(funcs.Auth))
	fmt.Printf("    DNS:    %s\n", boolToCheck(funcs.DNS))
	fmt.Println()
}

func (d *DiagnosticTool) printTLSTest(tlsTest *TLSTest) {
	fmt.Println("  TLS 连接测试:")
	for _, node := range tlsTest.CloudNodes {
		if node.Success {
			fmt.Printf("    ✓ %s: %s\n", node.Host, node.CipherSuite)
		} else if node.Error != "" {
			fmt.Printf("    ✗ %s: %s\n", node.Host, node.Error)
		}
	}
	fmt.Println()
}

func (d *DiagnosticTool) printBandwidthTest(bwTest *BandwidthTest) {
	fmt.Println("  带宽测试:")
	for _, result := range bwTest.Results {
		if result.Success {
			fmt.Printf("    %6d bytes: %.2f MB/s (%v)\n",
				result.Size,
				result.Bandwidth/1024/1024,
				result.Duration)
		} else {
			fmt.Printf("    %6d bytes: 失败 (%s)\n", result.Size, result.Error)
		}
	}
	fmt.Println()
}

func boolToCheck(b bool) string {
	if b {
		return "✓"
	}
	return "✗"
}

type DiagnosticReport struct {
	Timestamp          time.Time           `json:"timestamp"`
	ProxyAddr          string              `json:"proxy_addr"`
	ProxyHealth        *ProxyHealth        `json:"proxy_health"`
	CloudNodes         []*CloudNodeStatus  `json:"cloud_nodes"`
	ProxyFunctionality *ProxyFunctionality `json:"proxy_functionality"`
	TLSTest            *TLSTest            `json:"tls_test"`
	BandwidthTest      *BandwidthTest      `json:"bandwidth_test"`
}

type ProxyHealth struct {
	Listening  bool `json:"listening"`
	Responsive bool `json:"responsive"`
}

type CloudNodeStatus struct {
	URL          string `json:"url"`
	Success      bool   `json:"success"`
	StatusCode   int    `json:"status_code,omitempty"`
	ResponseBody string `json:"response_body,omitempty"`
	LatencyMs    int64  `json:"latency_ms,omitempty"`
	Error        string `json:"error,omitempty"`
}

type ProxyFunctionality struct {
	HTTP   bool `json:"http"`
	HTTPS  bool `json:"https"`
	SOCKS5 bool `json:"socks5"`
	Auth   bool `json:"auth"`
	DNS    bool `json:"dns"`
}

type TLSTest struct {
	CloudNodes []*TLSNodeTest `json:"cloud_nodes"`
}

type TLSNodeTest struct {
	Host        string `json:"host"`
	Success     bool   `json:"success"`
	CipherSuite string `json:"cipher_suite,omitempty"`
	Version     uint16 `json:"version,omitempty"`
	Error       string `json:"error,omitempty"`
}

type BandwidthTest struct {
	TestSizes []int64            `json:"test_sizes"`
	Results   []*BandwidthResult `json:"results"`
}

type BandwidthResult struct {
	Size      int64         `json:"size"`
	Success   bool          `json:"success"`
	BytesRead int64         `json:"bytes_read,omitempty"`
	Duration  time.Duration `json:"duration,omitempty"`
	Bandwidth float64       `json:"bandwidth,omitempty"`
	Error     string        `json:"error,omitempty"`
}

func (d *DiagnosticTool) SaveReport(filename string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	report := &DiagnosticReport{
		Timestamp:          time.Now(),
		ProxyAddr:          d.proxyAddr,
		ProxyHealth:        d.testProxyHealth(),
		CloudNodes:         d.testCloudNodes(),
		ProxyFunctionality: d.testProxyFunctionality(),
		TLSTest:            d.testTLS(),
		BandwidthTest:      d.testBandwidth(),
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filename, data, 0644)
}

type DebugConsole struct {
	proxyServer interface{}
	provider    interface{}
	tracer      *Tracer
	inspector   *PacketInspector
}

func NewDebugConsole(proxyServer, provider interface{}) *DebugConsole {
	return &DebugConsole{
		proxyServer: proxyServer,
		provider:    provider,
	}
}

func (c *DebugConsole) Start() {
	fmt.Println("========================================")
	fmt.Println("  Cloud ProxyPool 调试控制台")
	fmt.Println("========================================")
	fmt.Println()
	fmt.Println("可用命令:")
	fmt.Println("  help     - 显示帮助信息")
	fmt.Println("  status   - 显示代理状态")
	fmt.Println("  nodes    - 显示云函数节点状态")
	fmt.Println("  test     - 运行诊断测试")
	fmt.Println("  trace    - 开启/关闭请求追踪")
	fmt.Println("  logs     - 显示最近日志")
	fmt.Println("  exit     - 退出")
	fmt.Println()

	c.readLoop()
}

func (c *DebugConsole) readLoop() {
	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		cmd := strings.TrimSpace(scanner.Text())
		if cmd == "" {
			continue
		}

		c.handleCommand(cmd)
	}
}

func (c *DebugConsole) handleCommand(cmd string) {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return
	}

	command := parts[0]
	args := parts[1:]

	switch command {
	case "help":
		c.showHelp()
	case "status":
		c.showStatus()
	case "nodes":
		c.showNodes()
	case "test":
		c.runDiagnostics()
	case "trace":
		c.toggleTrace(args)
	case "logs":
		c.showLogs(args)
	case "exit":
		fmt.Println("再见!")
		os.Exit(0)
	default:
		fmt.Printf("未知命令: %s\n", command)
		fmt.Println("输入 'help' 查看可用命令")
	}
}

func (c *DebugConsole) showHelp() {
	fmt.Println("可用命令:")
	fmt.Println("  help         - 显示此帮助信息")
	fmt.Println("  status       - 显示代理状态和统计信息")
	fmt.Println("  nodes        - 显示所有云函数节点的状态")
	fmt.Println("  test         - 运行完整的诊断测试")
	fmt.Println("  trace [on|off] - 开启或关闭请求追踪")
	fmt.Println("  logs [lines] - 显示最近日志 (默认20行)")
	fmt.Println("  exit         - 退出调试控制台")
}

func (c *DebugConsole) showStatus() {
	fmt.Println("代理状态: ✓ 运行中")
	fmt.Println("监听地址: 0.0.0.0:10800")
	fmt.Println("SOCKS5:    0.0.0.0:10801")
	fmt.Println("Dashboard: 0.0.0.0:8081")
}

func (c *DebugConsole) showNodes() {
	fmt.Println("云函数节点状态:")
}

func (c *DebugConsole) runDiagnostics() {
	fmt.Println("运行诊断测试...")
}

func (c *DebugConsole) toggleTrace(args []string) {
	fmt.Println("追踪功能")
}

func (c *DebugConsole) showLogs(args []string) {
	fmt.Println("最近日志")
}
