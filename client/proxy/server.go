package proxy

import (
	"bufio"
	"bytes"
	"cloud-proxy-pool/cloud"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"cloud-proxy-pool/dashboard"

	"github.com/armon/go-socks5"
	"github.com/elazarl/goproxy"
	"github.com/elazarl/goproxy/ext/auth"
	"github.com/fatih/color"
)

type ProxyServer struct {
	ListenAddrs []string
	SocksAddr   string
	User        string
	Password    string
	Dump        bool
	DumpFile    string
	Provider    *cloud.Provider
	Verbose     bool
	dumpMu      sync.Mutex

	// Stats (Atomic)
	TotalRequests   uint64
	SuccessRequests uint64
	FailedRequests  uint64
	nextDialIndex   uint64
}

// GetStats implements dashboard.StatsReporter
func (s *ProxyServer) GetStats() dashboard.ProxyStats {
	return dashboard.ProxyStats{
		TotalRequests:   atomic.LoadUint64(&s.TotalRequests),
		SuccessRequests: atomic.LoadUint64(&s.SuccessRequests),
		FailedRequests:  atomic.LoadUint64(&s.FailedRequests),
		Nodes:           s.Provider.Nodes,
	}
}

func NewProxyServer(listenAddrs []string, socksAddr, user, password string, dump bool, dumpFile string, provider *cloud.Provider, verbose bool) *ProxyServer {
	if dumpFile == "" {
		dumpFile = "traffic.log"
	}
	return &ProxyServer{
		ListenAddrs: listenAddrs,
		SocksAddr:   socksAddr,
		User:        user,
		Password:    password,
		Dump:        dump,
		DumpFile:    dumpFile,
		Provider:    provider,
		Verbose:     verbose,
	}
}

func (s *ProxyServer) Start() error {
	if err := LoadOrGenerateCA("certs"); err != nil {
		fmt.Printf("warning: failed to load/generate CA cert: %v\n", err)
	} else {
		fmt.Println("[cert] loaded CA from certs/")
	}

	if s.Dump {
		fmt.Printf("[dump] traffic dump enabled, output file: %s\n", s.DumpFile)
	}

	if len(s.ListenAddrs) == 0 {
		return fmt.Errorf("no HTTP listen addresses configured")
	}

	errCh := make(chan error, len(s.ListenAddrs)+1)

	for _, addr := range s.ListenAddrs {
		go func(addr string) {
			errCh <- s.startHTTPProxy(addr)
		}(addr)
	}

	if s.SocksAddr != "" {
		go func() {
			errCh <- s.startSocks5Proxy()
		}()
	}

	return <-errCh
}

func (s *ProxyServer) startHTTPProxy(addr string) error {
	proxy := goproxy.NewProxyHttpServer()
	proxy.Logger = log.New(io.Discard, "", 0)
	proxy.Verbose = false

	if s.User != "" && s.Password != "" {
		proxy.OnRequest().Do(auth.Basic("CloudProxy", func(user, passwd string) bool {
			return user == s.User && passwd == s.Password
		}))
	}

	proxy.OnRequest().HandleConnect(goproxy.AlwaysMitm)
	proxy.OnRequest().DoFunc(func(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		return r, s.handleRequest(r)
	})

	log.Printf("HTTP proxy listening on: %s", addr)
	if s.User != "" {
		log.Printf("HTTP proxy auth enabled (user: %s)", s.User)
	}

	if err := http.ListenAndServe(addr, proxy); err != nil {
		return fmt.Errorf("HTTP proxy startup failed (%s): %w", addr, err)
	}
	return nil
}

func (s *ProxyServer) startSocks5Proxy() error {
	conf := &socks5.Config{
		Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
			_, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("invalid SOCKS5 target address %q: %w", addr, err)
			}

			if port == "80" || port == "8080" {
				clientConn, serverConn := net.Pipe()
				go s.handleSocksHTTP(serverConn, addr)
				return &FakeTCPConn{clientConn}, nil
			}

			return s.handleSocks5Connect(ctx, addr)
		},
		Logger: log.New(io.Discard, "", 0),
	}

	server, err := socks5.New(conf)
	if err != nil {
		return fmt.Errorf("SOCKS5 server initialization failed: %w", err)
	}

	log.Printf("SOCKS5 proxy listening on: %s", s.SocksAddr)
	if err := server.ListenAndServe("tcp", s.SocksAddr); err != nil {
		return fmt.Errorf("SOCKS5 server runtime failed: %w", err)
	}
	return nil
}

func (s *ProxyServer) handleSocks5Connect(ctx context.Context, addr string) (net.Conn, error) {
	proxyAddr := s.nextHTTPDialAddr()
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", proxyAddr)
	if err != nil {
		return nil, fmt.Errorf("SOCKS5 dial local HTTP proxy failed (%s): %w", proxyAddr, err)
	}

	connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n", addr, addr)
	if s.User != "" && s.Password != "" {
		token := base64.StdEncoding.EncodeToString([]byte(s.User + ":" + s.Password))
		connectReq += "Proxy-Authorization: Basic " + token + "\r\n"
	}
	connectReq += "\r\n"

	if _, err := conn.Write([]byte(connectReq)); err != nil {
		conn.Close()
		return nil, fmt.Errorf("SOCKS5 write CONNECT request failed: %w", err)
	}

	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, &http.Request{Method: http.MethodConnect})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("SOCKS5 read CONNECT response failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		bodyPreview := ""
		if resp.Body != nil {
			bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			_ = resp.Body.Close()
			bodyPreview = string(bytes.TrimSpace(bodyBytes))
		}
		conn.Close()

		if bodyPreview != "" {
			return nil, fmt.Errorf("SOCKS5 CONNECT failed: %s, body: %s", resp.Status, bodyPreview)
		}
		return nil, fmt.Errorf("SOCKS5 CONNECT failed: %s", resp.Status)
	}

	return &BufferedConn{Conn: conn, Reader: reader}, nil
}

func (s *ProxyServer) handleSocksHTTP(conn net.Conn, targetAddr string) {
	defer conn.Close()

	reader := bufio.NewReader(conn)

	req, err := http.ReadRequest(reader)
	if err != nil {
		if err != io.EOF {
			log.Printf("[SOCKS-HTTP] failed to read request: %v", err)
		}
		return
	}

	// Fill scheme/host for requests built from raw HTTP text.
	if req.URL.Scheme == "" {
		req.URL.Scheme = "http"
	}
	if req.URL.Host == "" {
		req.URL.Host = targetAddr
	}

	resp := s.handleRequest(req)

	if err := resp.Write(conn); err != nil {
		log.Printf("[SOCKS-HTTP] failed to write response: %v", err)
	}
}

func (s *ProxyServer) handleRequest(r *http.Request) *http.Response {
	// Stats: total requests +1.
	atomic.AddUint64(&s.TotalRequests, 1)

	startTime := time.Now()

	var bodyBytes []byte
	if r.Body != nil {
		readBody, err := io.ReadAll(r.Body)
		if err != nil {
			log.Printf("[proxy] failed to read request body: %v", err)
		} else {
			bodyBytes = readBody
		}
	}
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	// Optional traffic dump.
	if s.Dump {
		s.dumpRequest(r, bodyBytes)
	}

	isBase64 := true
	encodedBody := base64.StdEncoding.EncodeToString(bodyBytes)

	headers := make(map[string]string)
	for k, v := range r.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}
	// Keep Host header explicit for upstream compatibility.
	if r.Host != "" {
		headers["Host"] = r.Host
	}

	payload := cloud.FunctionRequest{
		Method:       r.Method,
		URL:          r.URL.String(),
		Headers:      headers,
		Body:         encodedBody,
		IsBodyBase64: isBase64,
	}

	funcResp, err := s.Provider.Invoke(payload)
	duration := time.Since(startTime)

	if err != nil {
		// Stats: failed requests +1.
		atomic.AddUint64(&s.FailedRequests, 1)
		dashboard.AddLogEntry(dashboard.LogEntry{
			Time:     startTime.Format("15:04:05"),
			Level:    "error",
			Method:   r.Method,
			URL:      r.URL.String(),
			Status:   http.StatusBadGateway,
			Duration: duration.Milliseconds(),
			Error:    err.Error(),
		})

		fmt.Printf("[%s] %s %s -> invoke failed (%v)\n",
			startTime.Format("15:04:05"), r.Method, r.URL.Host, duration)
		log.Printf("[proxy] invoke failed for %s: %v", r.URL, err)

		if s.Dump {
			s.dumpError(r.URL.String(), err)
		}
		return goproxy.NewResponse(r, goproxy.ContentTypeText, http.StatusBadGateway, "Cloud Proxy Error: "+err.Error())
	}

	// Stats: success requests +1.
	atomic.AddUint64(&s.SuccessRequests, 1)
	level := "info"
	if funcResp.StatusCode >= 400 {
		level = "error"
	} else if funcResp.StatusCode >= 300 {
		level = "warn"
	}
	dashboard.AddLogEntry(dashboard.LogEntry{
		Time:     startTime.Format("15:04:05"),
		Level:    level,
		Method:   r.Method,
		URL:      r.URL.String(),
		Status:   funcResp.StatusCode,
		Duration: duration.Milliseconds(),
	})

	statusColor := color.New(color.FgGreen).SprintFunc()
	if funcResp.StatusCode >= 400 {
		statusColor = color.New(color.FgRed).SprintFunc()
	} else if funcResp.StatusCode >= 300 {
		statusColor = color.New(color.FgYellow).SprintFunc()
	}

	fmt.Printf("[%s] %s %s -> %s (%v)\n",
		startTime.Format("15:04:05"),
		r.Method,
		r.URL.Host,
		statusColor(fmt.Sprintf("%d", funcResp.StatusCode)),
		duration.Round(time.Millisecond))

	// Decode function response body when needed.
	var respBody []byte
	if funcResp.IsContentBase64 {
		decoded, err := base64.StdEncoding.DecodeString(funcResp.Content)
		if err != nil {
			log.Printf("[proxy] failed to decode base64 response body: %v", err)
			return goproxy.NewResponse(r, goproxy.ContentTypeText, http.StatusBadGateway, "Response Decode Error")
		}
		respBody = decoded
	} else {
		respBody = []byte(funcResp.Content)
	}

	// Optional traffic dump.
	if s.Dump {
		s.dumpResponse(r.URL.String(), funcResp.StatusCode, len(respBody))
	}

	resp := &http.Response{
		Request:       r,
		StatusCode:    funcResp.StatusCode,
		Status:        http.StatusText(funcResp.StatusCode),
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        make(http.Header),
		Body:          io.NopCloser(bytes.NewBuffer(respBody)),
		ContentLength: int64(len(respBody)),
	}

	for k, v := range funcResp.Headers {
		resp.Header.Set(k, v)
	}

	return resp
}

func (s *ProxyServer) dumpRequest(r *http.Request, body []byte) {
	s.dumpMu.Lock()
	defer s.dumpMu.Unlock()

	f, err := os.OpenFile(s.DumpFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("[dump] failed to open dump file %s: %v", s.DumpFile, err)
		return
	}
	defer f.Close()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	reqLine := fmt.Sprintf("[%s] REQUEST: %s %s\n", timestamp, r.Method, r.URL.String())
	f.WriteString(reqLine)

	// headers
	for k, v := range r.Header {
		f.WriteString(fmt.Sprintf("> %s: %s\n", k, v[0]))
	}
	f.WriteString("\n")

	// body preview (limit to 1KB)
	if len(body) > 0 {
		limit := 1024
		if len(body) < limit {
			limit = len(body)
		}
		f.WriteString(string(body[:limit]))
		if len(body) > limit {
			f.WriteString("\n... (body truncated)")
		}
		f.WriteString("\n")
	}
	f.WriteString("--------------------------------------------------\n")
}

func (s *ProxyServer) dumpResponse(url string, code int, size int) {
	s.dumpMu.Lock()
	defer s.dumpMu.Unlock()

	f, err := os.OpenFile(s.DumpFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	resLine := fmt.Sprintf("[%s] RESPONSE: %s -> %d (Size: %d bytes)\n", timestamp, url, code, size)
	f.WriteString(resLine)
	f.WriteString("==================================================\n\n")
}

func (s *ProxyServer) dumpError(url string, err error) {
	s.dumpMu.Lock()
	defer s.dumpMu.Unlock()

	f, err := os.OpenFile(s.DumpFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	errLine := fmt.Sprintf("[%s] ERROR: %s -> %v\n", timestamp, url, err)
	f.WriteString(errLine)
	f.WriteString("==================================================\n\n")
}

// FakeTCPConn wraps a net.Conn to return fake TCP addresses
// needed because armon/go-socks5 type assertions fail on net.Pipe()
type FakeTCPConn struct {
	net.Conn
}

func (c *FakeTCPConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345}
}

func (c *FakeTCPConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 80}
}

func (s *ProxyServer) nextHTTPDialAddr() string {
	if len(s.ListenAddrs) == 1 {
		return s.ListenAddrs[0]
	}

	idx := atomic.AddUint64(&s.nextDialIndex, 1)
	return s.ListenAddrs[(idx-1)%uint64(len(s.ListenAddrs))]
}

type BufferedConn struct {
	net.Conn
	Reader *bufio.Reader
}

func (c *BufferedConn) Read(b []byte) (int, error) {
	return c.Reader.Read(b)
}
