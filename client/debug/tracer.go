package debug

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

type Tracer struct {
	outputDir   string
	enableTrace bool
	traceFile   *os.File
	requestID   uint64
	batchSize   int
	batchBuffer []TraceEntry
	flushTimer  *time.Timer
}

type TraceEntry struct {
	RequestID  uint64                 `json:"request_id"`
	Timestamp  time.Time              `json:"timestamp"`
	Stage      string                 `json:"stage"`
	Method     string                 `json:"method,omitempty"`
	URL        string                 `json:"url,omitempty"`
	StatusCode int                    `json:"status_code,omitempty"`
	Error      string                 `json:"error,omitempty"`
	LatencyMs  int64                  `json:"latency_ms,omitempty"`
	Headers    map[string]string      `json:"headers,omitempty"`
	BodySize   int64                  `json:"body_size,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

func NewTracer(outputDir string, enableTrace bool) (*Tracer, error) {
	if !enableTrace {
		return &Tracer{enableTrace: false}, nil
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create trace directory: %w", err)
	}

	t := &Tracer{
		outputDir:   outputDir,
		enableTrace: true,
		batchSize:   100,
		batchBuffer: make([]TraceEntry, 0, 100),
	}

	tracePath := filepath.Join(outputDir, fmt.Sprintf("trace_%s.jsonl", time.Now().Format("20060102_150405")))
	traceFile, err := os.OpenFile(tracePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open trace file: %w", err)
	}
	t.traceFile = traceFile

	t.flushTimer = time.AfterFunc(5*time.Second, t.flushBatch)

	return t, nil
}

func (t *Tracer) Close() {
	if t.flushTimer != nil {
		t.flushTimer.Stop()
	}
	t.flushBatch()
	if t.traceFile != nil {
		t.traceFile.Close()
	}
}

func (t *Tracer) TraceRequest(method, url string, headers map[string]string) uint64 {
	if !t.enableTrace {
		return 0
	}

	t.requestID++

	entry := TraceEntry{
		RequestID: t.requestID,
		Timestamp: time.Now(),
		Stage:     "client_request",
		Method:    method,
		URL:       url,
		Headers:   headers,
	}

	t.addEntry(entry)
	return t.requestID
}

func (t *Tracer) TraceProxyRequest(requestID uint64, targetURL string) {
	if !t.enableTrace || requestID == 0 {
		return
	}

	entry := TraceEntry{
		RequestID: requestID,
		Timestamp: time.Now(),
		Stage:     "proxy_forward",
		URL:       targetURL,
	}

	t.addEntry(entry)
}

func (t *Tracer) TraceCloudRequest(requestID uint64, cloudNode, targetURL string) {
	if !t.enableTrace || requestID == 0 {
		return
	}

	entry := TraceEntry{
		RequestID: requestID,
		Timestamp: time.Now(),
		Stage:     "cloud_invoke",
		URL:       targetURL,
		Metadata: map[string]interface{}{
			"cloud_node": cloudNode,
		},
	}

	t.addEntry(entry)
}

func (t *Tracer) TraceCloudResponse(requestID uint64, statusCode int, latency time.Duration) {
	if !t.enableTrace || requestID == 0 {
		return
	}

	entry := TraceEntry{
		RequestID:  requestID,
		Timestamp:  time.Now(),
		Stage:      "cloud_response",
		StatusCode: statusCode,
		LatencyMs:  latency.Milliseconds(),
	}

	t.addEntry(entry)
}

func (t *Tracer) TraceProxyResponse(requestID uint64, statusCode int, latency time.Duration) {
	if !t.enableTrace || requestID == 0 {
		return
	}

	entry := TraceEntry{
		RequestID:  requestID,
		Timestamp:  time.Now(),
		Stage:      "proxy_response",
		StatusCode: statusCode,
		LatencyMs:  latency.Milliseconds(),
	}

	t.addEntry(entry)
}

func (t *Tracer) TraceClientResponse(requestID uint64, statusCode int, totalLatency time.Duration) {
	if !t.enableTrace || requestID == 0 {
		return
	}

	entry := TraceEntry{
		RequestID:  requestID,
		Timestamp:  time.Now(),
		Stage:      "client_response",
		StatusCode: statusCode,
		LatencyMs:  totalLatency.Milliseconds(),
	}

	t.addEntry(entry)
}

func (t *Tracer) TraceError(requestID uint64, stage, errorMsg string) {
	if !t.enableTrace || requestID == 0 {
		return
	}

	entry := TraceEntry{
		RequestID: requestID,
		Timestamp: time.Now(),
		Stage:     stage,
		Error:     errorMsg,
	}

	t.addEntry(entry)
}

func (t *Tracer) TraceTLS(stage, peer, cipherSuite string, err error) {
	if !t.enableTrace {
		return
	}

	metadata := map[string]interface{}{
		"peer":         peer,
		"cipher_suite": cipherSuite,
	}

	entry := TraceEntry{
		RequestID: t.requestID,
		Timestamp: time.Now(),
		Stage:     stage,
		Metadata:  metadata,
	}

	if err != nil {
		entry.Error = err.Error()
	}

	t.addEntry(entry)
}

func (t *Tracer) addEntry(entry TraceEntry) {
	t.batchBuffer = append(t.batchBuffer, entry)

	if len(t.batchBuffer) >= t.batchSize {
		t.flushBatch()
	}
}

func (t *Tracer) flushBatch() {
	if len(t.batchBuffer) == 0 || t.traceFile == nil {
		return
	}

	for _, entry := range t.batchBuffer {
		data, err := json.Marshal(entry)
		if err != nil {
			log.Printf("[trace] failed to marshal entry: %v", err)
			continue
		}

		if _, err := t.traceFile.Write(append(data, '\n')); err != nil {
			log.Printf("[trace] failed to write entry: %v", err)
		}
	}

	t.batchBuffer = t.batchBuffer[:0]
	t.traceFile.Sync()
}

type HTTPDebugger struct {
	baseURL string
	timeout time.Duration
	verbose bool
}

func NewHTTPDebugger(baseURL string) *HTTPDebugger {
	return &HTTPDebugger{
		baseURL: baseURL,
		timeout: 30 * time.Second,
		verbose: true,
	}
}

func (d *HTTPDebugger) TestEndpoint(targetURL string) (*TestResult, error) {
	startTime := time.Now()

	client := &http.Client{
		Timeout: d.timeout,
		Transport: &http.Transport{
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
			DisableKeepAlives:     false,
			MaxIdleConns:          10,
			IdleConnTimeout:       30 * time.Second,
		},
	}

	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "CloudProxyPool-Debugger")
	req.Header.Set("Accept", "*/*")

	resp, err := client.Do(req)
	if err != nil {
		return &TestResult{
			Success: false,
			Error:   err.Error(),
		}, nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &TestResult{
			Success:    false,
			StatusCode: resp.StatusCode,
			Error:      fmt.Sprintf("failed to read body: %v", err),
			Headers:    headersToMap(resp.Header),
		}, nil
	}

	result := &TestResult{
		Success:     true,
		StatusCode:  resp.StatusCode,
		Headers:     headersToMap(resp.Header),
		BodySize:    int64(len(body)),
		BodyPreview: string(body),
		LatencyMs:   time.Since(startTime).Milliseconds(),
	}

	if d.verbose {
		log.Printf("[debug] Test result for %s: %d (%dms)", targetURL, resp.StatusCode, result.LatencyMs)
	}

	return result, nil
}

func (d *HTTPDebugger) TestEndpointViaProxy(proxyAddr, targetURL string) (*TestResult, error) {
	startTime := time.Now()

	proxyURL, err := url.Parse("http://" + proxyAddr)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy URL: %w", err)
	}

	client := &http.Client{
		Timeout: d.timeout,
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
			TLSHandshakeTimeout:   15 * time.Second,
			ResponseHeaderTimeout: 15 * time.Second,
		},
	}

	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "CloudProxyPool-Debugger")

	resp, err := client.Do(req)
	if err != nil {
		return &TestResult{
			Success: false,
			Error:   err.Error(),
		}, nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &TestResult{
			Success:    false,
			StatusCode: resp.StatusCode,
			Error:      fmt.Sprintf("failed to read body: %v", err),
			Headers:    headersToMap(resp.Header),
		}, nil
	}

	result := &TestResult{
		Success:     true,
		StatusCode:  resp.StatusCode,
		Headers:     headersToMap(resp.Header),
		BodySize:    int64(len(body)),
		BodyPreview: string(body),
		LatencyMs:   time.Since(startTime).Milliseconds(),
	}

	if d.verbose {
		log.Printf("[debug] Proxy test result for %s via %s: %d (%dms)", targetURL, proxyAddr, resp.StatusCode, result.LatencyMs)
	}

	return result, nil
}

type TestResult struct {
	Success     bool              `json:"success"`
	StatusCode  int               `json:"status_code,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	BodySize    int64             `json:"body_size,omitempty"`
	BodyPreview string            `json:"body_preview,omitempty"`
	LatencyMs   int64             `json:"latency_ms,omitempty"`
	Error       string            `json:"error,omitempty"`
}

func headersToMap(headers http.Header) map[string]string {
	result := make(map[string]string)
	for k, v := range headers {
		if len(v) > 0 {
			result[k] = v[0]
		}
	}
	return result
}
