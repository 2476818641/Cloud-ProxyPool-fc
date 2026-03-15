package debug

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type PacketInspector struct {
	maxBodySize    int64
	verbose        bool
	filterPatterns []*regexp.Regexp
}

func NewPacketInspector(maxBodySize int64, verbose bool) *PacketInspector {
	if maxBodySize <= 0 {
		maxBodySize = 10240
	}

	return &PacketInspector{
		maxBodySize:    maxBodySize,
		verbose:        verbose,
		filterPatterns: make([]*regexp.Regexp, 0),
	}
}

func (p *PacketInspector) AddFilter(pattern string) error {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("invalid filter pattern: %w", err)
	}
	p.filterPatterns = append(p.filterPatterns, re)
	return nil
}

func (p *PacketInspector) InspectHTTPRequest(req *http.Request) *PacketInfo {
	info := &PacketInfo{
		Timestamp:     time.Now(),
		Type:          "http_request",
		Method:        req.Method,
		URL:           req.URL.String(),
		Protocol:      req.Proto,
		Headers:       headersToMap(req.Header),
		ContentType:   req.Header.Get("Content-Type"),
		ContentLength: req.ContentLength,
	}

	if req.Body != nil {
		bodyBytes, err := io.ReadAll(req.Body)
		if err == nil {
			info.RawBodySize = int64(len(bodyBytes))
			info.BodyPreview = p.truncateBody(bodyBytes)
			info.IsBase64Encoded = p.isBinaryContent(req.Header.Get("Content-Type"))
			if info.IsBase64Encoded {
				info.EncodedBody = base64.StdEncoding.EncodeToString(bodyBytes)
			}
			req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}
	}

	p.analyzeHeaders(info)

	if p.verbose {
		log.Printf("[packet] HTTP Request: %s %s [%d bytes]", info.Method, info.URL, info.ContentLength)
	}

	return info
}

func (p *PacketInspector) InspectHTTPResponse(resp *http.Response) *PacketInfo {
	info := &PacketInfo{
		Timestamp:     time.Now(),
		Type:          "http_response",
		StatusCode:    resp.StatusCode,
		Status:        resp.Status,
		Protocol:      resp.Proto,
		Headers:       headersToMap(resp.Header),
		ContentType:   resp.Header.Get("Content-Type"),
		ContentLength: resp.ContentLength,
	}

	if resp.Body != nil {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err == nil {
			info.RawBodySize = int64(len(bodyBytes))
			info.BodyPreview = p.truncateBody(bodyBytes)
			info.IsBase64Encoded = p.isBinaryContent(resp.Header.Get("Content-Type"))
			if info.IsBase64Encoded {
				info.EncodedBody = base64.StdEncoding.EncodeToString(bodyBytes)
			}
			resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}
	}

	p.analyzeHeaders(info)

	if p.verbose {
		log.Printf("[packet] HTTP Response: %d %s [%d bytes]", info.StatusCode, info.Status, info.ContentLength)
	}

	return info
}

func (p *PacketInspector) InspectTLSHandshake(peer string, cipherSuite string, serverName string, certCount int, duration time.Duration) *TLSInfo {
	info := &TLSInfo{
		Timestamp:   time.Now(),
		Type:        "tls_handshake",
		Peer:        peer,
		CipherSuite: cipherSuite,
		ServerName:  serverName,
		CertCount:   certCount,
		DurationMs:  duration.Milliseconds(),
	}

	if p.verbose {
		log.Printf("[tls] Handshake with %s: %s (SNI: %s, certs: %d, %dms)", peer, cipherSuite, serverName, certCount, duration.Milliseconds())
	}

	return info
}

func (p *PacketInspector) InspectCloudRequest(payload interface{}, targetURL string) *CloudPacketInfo {
	info := &CloudPacketInfo{
		Timestamp: time.Now(),
		Type:      "cloud_request",
		TargetURL: targetURL,
	}

	if data, ok := payload.([]byte); ok {
		info.PayloadSize = int64(len(data))
		info.PayloadPreview = string(data)
	} else if jsonBytes, err := json.Marshal(payload); err == nil {
		info.PayloadSize = int64(len(jsonBytes))
		info.PayloadPreview = p.truncateBody(jsonBytes)
		var parsed map[string]interface{}
		if json.Unmarshal(jsonBytes, &parsed) == nil {
			info.Method = parsed["method"].(string)
			info.URL = parsed["url"].(string)
			if headers, ok := parsed["headers"].(map[string]interface{}); ok {
				info.Headers = make(map[string]string)
				for k, v := range headers {
					if vs, ok := v.(string); ok {
						info.Headers[k] = vs
					}
				}
			}
		}
	}

	if p.verbose {
		log.Printf("[cloud] Request: %s %s [%d bytes]", info.Method, info.URL, info.PayloadSize)
	}

	return info
}

func (p *PacketInspector) InspectCloudResponse(statusCode int, headers map[string]string, content string, isBase64 bool) *CloudPacketInfo {
	info := &CloudPacketInfo{
		Timestamp:    time.Now(),
		Type:         "cloud_response",
		StatusCode:   statusCode,
		Headers:      headers,
		IsBase64:     isBase64,
		ResponseSize: int64(len(content)),
	}

	if isBase64 {
		info.PayloadPreview = fmt.Sprintf("[Base64: %d bytes]", len(content))
		if decoded, err := base64.StdEncoding.DecodeString(content); err == nil {
			info.DecodedSize = int64(len(decoded))
			info.PayloadPreview = p.truncateBody(decoded)
		}
	} else {
		info.PayloadPreview = p.truncateBody([]byte(content))
	}

	if p.verbose {
		log.Printf("[cloud] Response: %d [%d bytes]", statusCode, info.ResponseSize)
	}

	return info
}

func (p *PacketInspector) truncateBody(body []byte) string {
	if int64(len(body)) <= p.maxBodySize {
		return string(body)
	}
	return string(body[:p.maxBodySize]) + fmt.Sprintf("... [truncated, total: %d bytes]", len(body))
}

func (p *PacketInspector) isBinaryContent(contentType string) bool {
	if contentType == "" {
		return false
	}

	binaryTypes := []string{
		"application/octet-stream",
		"application/pdf",
		"application/zip",
		"image/",
		"video/",
		"audio/",
	}

	for _, t := range binaryTypes {
		if strings.Contains(contentType, t) {
			return true
		}
	}

	return false
}

func (p *PacketInspector) analyzeHeaders(info *PacketInfo) {
	for key, value := range info.Headers {
		lowerKey := strings.ToLower(key)

		switch lowerKey {
		case "authorization":
			if strings.HasPrefix(value, "Bearer ") {
				info.AuthType = "Bearer"
			} else if strings.HasPrefix(value, "Basic ") {
				info.AuthType = "Basic"
			} else {
				info.AuthType = "Other"
			}
		case "content-encoding":
			info.Compression = value
		case "transfer-encoding":
			info.Chunked = strings.Contains(strings.ToLower(value), "chunked")
		case "connection":
			info.KeepAlive = strings.Contains(strings.ToLower(value), "keep-alive")
		}
	}
}

func (p *PacketInspector) MatchFilters(info interface{}) bool {
	if len(p.filterPatterns) == 0 {
		return true
	}

	infoBytes, err := json.Marshal(info)
	if err != nil {
		return false
	}

	infoStr := string(infoBytes)

	for _, pattern := range p.filterPatterns {
		if pattern.MatchString(infoStr) {
			return true
		}
	}

	return false
}

type PacketInfo struct {
	Timestamp       time.Time              `json:"timestamp"`
	Type            string                 `json:"type"`
	Method          string                 `json:"method,omitempty"`
	URL             string                 `json:"url,omitempty"`
	Protocol        string                 `json:"protocol,omitempty"`
	StatusCode      int                    `json:"status_code,omitempty"`
	Status          string                 `json:"status,omitempty"`
	Headers         map[string]string      `json:"headers,omitempty"`
	ContentType     string                 `json:"content_type,omitempty"`
	ContentLength   int64                  `json:"content_length,omitempty"`
	RawBodySize     int64                  `json:"raw_body_size,omitempty"`
	BodyPreview     string                 `json:"body_preview,omitempty"`
	IsBase64Encoded bool                   `json:"is_base64_encoded,omitempty"`
	EncodedBody     string                 `json:"encoded_body,omitempty"`
	AuthType        string                 `json:"auth_type,omitempty"`
	Compression     string                 `json:"compression,omitempty"`
	Chunked         bool                   `json:"chunked,omitempty"`
	KeepAlive       bool                   `json:"keep_alive,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

type TLSInfo struct {
	Timestamp   time.Time `json:"timestamp"`
	Type        string    `json:"type"`
	Peer        string    `json:"peer"`
	CipherSuite string    `json:"cipher_suite"`
	ServerName  string    `json:"server_name"`
	CertCount   int       `json:"cert_count"`
	DurationMs  int64     `json:"duration_ms"`
	Success     bool      `json:"success"`
	Error       string    `json:"error,omitempty"`
}

type CloudPacketInfo struct {
	Timestamp      time.Time              `json:"timestamp"`
	Type           string                 `json:"type"`
	Method         string                 `json:"method,omitempty"`
	URL            string                 `json:"url,omitempty"`
	TargetURL      string                 `json:"target_url,omitempty"`
	Headers        map[string]string      `json:"headers,omitempty"`
	PayloadSize    int64                  `json:"payload_size,omitempty"`
	ResponseSize   int64                  `json:"response_size,omitempty"`
	DecodedSize    int64                  `json:"decoded_size,omitempty"`
	PayloadPreview string                 `json:"payload_preview,omitempty"`
	StatusCode     int                    `json:"status_code,omitempty"`
	IsBase64       bool                   `json:"is_base64,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

type LatencyProfiler struct {
	stages map[string][]time.Duration
	mu     sync.RWMutex
}

func NewLatencyProfiler() *LatencyProfiler {
	return &LatencyProfiler{
		stages: make(map[string][]time.Duration),
	}
}

func (l *LatencyProfiler) Record(stage string, duration time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.stages[stage] = append(l.stages[stage], duration)

	if len(l.stages[stage]) > 1000 {
		l.stages[stage] = l.stages[stage][1:]
	}
}

func (l *LatencyProfiler) GetStats(stage string) *LatencyStats {
	l.mu.RLock()
	defer l.mu.RUnlock()

	durations, exists := l.stages[stage]
	if !exists || len(durations) == 0 {
		return nil
	}

	stats := l.calculateStats(durations)
	stats.Stage = stage
	return stats
}

func (l *LatencyProfiler) GetAllStats() map[string]*LatencyStats {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make(map[string]*LatencyStats)
	for stage, durations := range l.stages {
		if len(durations) > 0 {
			stats := l.calculateStats(durations)
			stats.Stage = stage
			result[stage] = stats
		}
	}
	return result
}

func (l *LatencyProfiler) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.stages = make(map[string][]time.Duration)
}

func (l *LatencyProfiler) calculateStats(durations []time.Duration) *LatencyStats {
	if len(durations) == 0 {
		return &LatencyStats{}
	}

	sort.Slice(durations, func(i, j int) bool {
		return durations[i] < durations[j]
	})

	total := time.Duration(0)
	for _, d := range durations {
		total += d
	}

	return &LatencyStats{
		Count: len(durations),
		AvgMs: float64(total) / float64(len(durations)) / float64(time.Millisecond),
		MinMs: float64(durations[0]) / float64(time.Millisecond),
		MaxMs: float64(durations[len(durations)-1]) / float64(time.Millisecond),
		P50Ms: float64(durations[len(durations)*50/100]) / float64(time.Millisecond),
		P95Ms: float64(durations[len(durations)*95/100]) / float64(time.Millisecond),
		P99Ms: float64(durations[len(durations)*99/100]) / float64(time.Millisecond),
	}
}

type LatencyStats struct {
	Stage string  `json:"stage"`
	Count int     `json:"count"`
	AvgMs float64 `json:"avg_ms"`
	MinMs float64 `json:"min_ms"`
	MaxMs float64 `json:"max_ms"`
	P50Ms float64 `json:"p50_ms"`
	P95Ms float64 `json:"p95_ms"`
	P99Ms float64 `json:"p99_ms"`
}
