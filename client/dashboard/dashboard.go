package dashboard

import (
	"bytes"
	"cloud-proxy-pool/cloud"
	"cloud-proxy-pool/config"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"log"
	"net"
	"net/http"
	urlpkg "net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
)

type StatsReporter interface {
	GetStats() ProxyStats
}

type ProxyStats struct {
	TotalRequests   uint64
	SuccessRequests uint64
	FailedRequests  uint64
	Nodes           []*cloud.Node
}

type LogEntry struct {
	Time     string    `json:"time"`
	Level    string    `json:"level"`
	Method   string    `json:"method"`
	URL      string    `json:"url"`
	Status   int       `json:"status"`
	Duration int64     `json:"duration"`
	Error    string    `json:"error,omitempty"`
	Recorded time.Time `json:"-"`
}

type HistoryEntry struct {
	Time       string  `json:"time"`
	QPS        float64 `json:"qps"`
	AvgLatency int64   `json:"avgLatency"`
	P95Latency int64   `json:"p95Latency"`
	P99Latency int64   `json:"p99Latency"`
}

type NodeDetail struct {
	URL          string `json:"url"`
	Region       string `json:"region"`
	FailureCount int    `json:"failure_count"`
	LastFail     string `json:"last_fail"`
	Status       string `json:"status"`
	Latency      int64  `json:"latency"`
	Enabled      bool   `json:"enabled"`
}

type ConfigData struct {
	ListenAddr       string           `json:"listenAddr"`
	SocksAddr        string           `json:"socksAddr"`
	User             string           `json:"user"`
	Password         string           `json:"password"`
	DashboardEnabled bool             `json:"dashboardEnabled"`
	Users            []UserCredential `json:"users"`
}

type UserCredential struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token string `json:"token"`
}

type Claims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

type serverOptions struct {
	Scheme   string
	UseTLS   bool
	CertFile string
	KeyFile  string
}

var (
	Reporter         StatsReporter
	Provider         *cloud.Provider
	configPath       string
	logHistory       []LogEntry
	historyBuffer    []HistoryEntry
	logMutex         sync.RWMutex
	historyMutex     sync.RWMutex
	nodeEnabledMap   = make(map[string]bool)
	nodeLatencyMap   = make(map[string]int64)
	nodeLatencyMutex sync.RWMutex
	nodeMutex        sync.RWMutex
	lastRequestCount uint64
	lastHistoryTime  time.Time
	websocketClients = make(map[*websocket.Conn]bool)
	wsMutex          sync.RWMutex
	upgrader         = websocket.Upgrader{
		CheckOrigin: sameOrigin,
	}

	jwtSecret       []byte
	dashboardUser   string
	dashboardPass   string
	sessionDuration = 7 * 24 * time.Hour
)

func init() {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = generateEphemeralSecret()
		log.Print("[warn] JWT_SECRET not set, generated an ephemeral secret for this process")
	}
	jwtSecret = []byte(secret)

	dashboardUser = os.Getenv("DASHBOARD_USER")
	dashboardPass = os.Getenv("DASHBOARD_PASSWORD")
}

func generateEphemeralSecret() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return hex.EncodeToString(buf)
}

func sameOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}

	parsed, err := urlpkg.Parse(origin)
	if err != nil {
		return false
	}

	return sameHost(parsed.Host, r.Host)
}

func sameHost(left, right string) bool {
	leftHost := normalizeHost(left)
	rightHost := normalizeHost(right)
	return leftHost != "" && leftHost == rightHost
}

func normalizeHost(hostport string) string {
	hostport = strings.TrimSpace(strings.ToLower(hostport))
	if hostport == "" {
		return ""
	}

	if host, _, err := net.SplitHostPort(hostport); err == nil {
		return host
	}

	return hostport
}

func StartDashboard(addr string, reporter StatsReporter, provider *cloud.Provider, cfgPath string) {
	Reporter = reporter
	Provider = provider
	Provider.SetNodeEnabledFunc(IsNodeEnabled)
	configPath = cfgPath
	options := loadServerOptions()

	mux := http.NewServeMux()

	mux.HandleFunc("/", handleSPA)
	mux.HandleFunc("/api/login", handleLogin)
	mux.HandleFunc("/api/stats", requireAuth(handleStats))
	mux.HandleFunc("/api/history", requireAuth(handleHistory))
	mux.HandleFunc("/api/nodes", requireAuth(handleNodes))
	mux.HandleFunc("/api/nodes/test", requireAuth(handleTestLatency))
	mux.HandleFunc("/api/config", requireAuth(handleConfig))
	mux.HandleFunc("/api/logs", requireAuth(handleLogs))
	mux.HandleFunc("/ws/logs", requireAuthWS(handleWebSocketLogs))

	log.Printf("监控面板正在监听: %s://localhost%s", options.Scheme, addr)
	go func() {
		var err error
		if options.UseTLS {
			err = http.ListenAndServeTLS(addr, options.CertFile, options.KeyFile, mux)
		} else {
			err = http.ListenAndServe(addr, mux)
		}
		if err != nil {
			log.Fatalf("Dashboard启动失败: %v", err)
		}
	}()

	go updateHistory()
}

func loadServerOptions() serverOptions {
	options := serverOptions{
		Scheme: "http",
	}

	if !envEnabled("DASHBOARD_TLS_ENABLED") {
		return options
	}

	options.UseTLS = true
	options.Scheme = "https"
	options.CertFile = strings.TrimSpace(os.Getenv("DASHBOARD_TLS_CERT_FILE"))
	options.KeyFile = strings.TrimSpace(os.Getenv("DASHBOARD_TLS_KEY_FILE"))

	if options.CertFile == "" || options.KeyFile == "" {
		log.Fatal("Dashboard TLS 已启用，但没有提供证书或私钥路径")
	}

	return options
}

func envEnabled(name string) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	return value == "1" || value == "true" || value == "yes" || value == "y"
}

func handleSPA(w http.ResponseWriter, r *http.Request) {
	distPath := "/app/client/dashboard/dist"
	if r.URL.Path == "/" || r.URL.Path == "" {
		http.ServeFile(w, r, distPath+"/index.html")
		return
	}

	filePath := distPath + r.URL.Path
	_, err := os.Stat(filePath)
	if err == nil {
		http.ServeFile(w, r, filePath)
		return
	}

	http.ServeFile(w, r, distPath+"/index.html")
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if !authenticate(req.Username, req.Password) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid credentials"})
		return
	}

	token, err := generateJWT(req.Username)
	if err != nil {
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(LoginResponse{Token: token})
}

func authenticate(username, password string) bool {
	if dashboardUser == "" || dashboardPass == "" {
		return false
	}

	if username != dashboardUser {
		return false
	}

	return password == dashboardPass ||
		password == base64StdSHA256(dashboardPass) ||
		password == base64HexSHA256(dashboardPass)
}

func generateJWT(username string) (string, error) {
	claims := Claims{
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(sessionDuration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

func validateJWT(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, jwt.ErrSignatureInvalid
}

func requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "Missing authorization header"})
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		_, err := validateJWT(tokenString)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "Invalid or expired token"})
			return
		}

		next(w, r)
	}
}

func requireAuthWS(next func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokenString := websocketTokenFromRequest(r)
		if tokenString == "" {
			http.Error(w, "Missing token", http.StatusUnauthorized)
			return
		}

		_, err := validateJWT(tokenString)
		if err != nil {
			http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}

func websocketTokenFromRequest(r *http.Request) string {
	// Prefer WebSocket subprotocol to avoid putting JWTs in URL query logs.
	if raw := strings.TrimSpace(r.Header.Get("Sec-WebSocket-Protocol")); raw != "" {
		parts := strings.Split(raw, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		if len(parts) >= 2 && strings.EqualFold(parts[0], "bearer") && parts[1] != "" {
			return parts[1]
		}
		if len(parts) == 1 && parts[0] != "" {
			return parts[0]
		}
	}

	return strings.TrimSpace(r.URL.Query().Get("token"))
}

func handleStats(w http.ResponseWriter, r *http.Request) {
	if Reporter == nil {
		http.Error(w, "Stats not available", http.StatusServiceUnavailable)
		return
	}
	stats := Reporter.GetStats()

	nodeStats := make([]NodeDetail, len(stats.Nodes))
	for i, n := range stats.Nodes {
		status := "Healthy"
		if !n.IsHealthy() {
			status = "Melting (Fused)"
		}
		lastFail := ""
		if !n.LastFailTime.IsZero() {
			lastFail = n.LastFailTime.Format("15:04:05")
		}

		nodeLatencyMutex.RLock()
		latency := nodeLatencyMap[n.URL]
		nodeLatencyMutex.RUnlock()

		nodeMutex.RLock()
		enabled, exists := nodeEnabledMap[n.URL]
		if !exists {
			enabled = true
		}
		nodeMutex.RUnlock()

		nodeStats[i] = NodeDetail{
			URL:          n.URL,
			Region:       extractRegion(n.URL),
			FailureCount: n.FailureCount,
			Status:       status,
			LastFail:     lastFail,
			Latency:      latency,
			Enabled:      enabled,
		}
	}

	resp := map[string]interface{}{
		"total":   stats.TotalRequests,
		"success": stats.SuccessRequests,
		"failed":  stats.FailedRequests,
		"nodes":   nodeStats,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func handleHistory(w http.ResponseWriter, r *http.Request) {
	historyMutex.RLock()
	defer historyMutex.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(historyBuffer)
}

func handleNodes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if Reporter == nil {
			http.Error(w, "Stats not available", http.StatusServiceUnavailable)
			return
		}
		stats := Reporter.GetStats()

		nodeStats := make([]NodeDetail, len(stats.Nodes))
		for i, n := range stats.Nodes {
			status := "Healthy"
			if !n.IsHealthy() {
				status = "Melting (Fused)"
			}
			lastFail := ""
			if !n.LastFailTime.IsZero() {
				lastFail = n.LastFailTime.Format("15:04:05")
			}

			nodeLatencyMutex.RLock()
			latency := nodeLatencyMap[n.URL]
			nodeLatencyMutex.RUnlock()

			nodeMutex.RLock()
			enabled, exists := nodeEnabledMap[n.URL]
			if !exists {
				enabled = true
			}
			nodeMutex.RUnlock()

			nodeStats[i] = NodeDetail{
				URL:          n.URL,
				Region:       extractRegion(n.URL),
				FailureCount: n.FailureCount,
				Status:       status,
				LastFail:     lastFail,
				Latency:      latency,
				Enabled:      enabled,
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(nodeStats)

	case http.MethodPut:
		var req struct {
			URL     string `json:"url"`
			Enabled bool   `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		nodeMutex.Lock()
		nodeEnabledMap[req.URL] = req.Enabled
		nodeMutex.Unlock()

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleTestLatency(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	start := time.Now()
	testReq := cloud.FunctionRequest{
		Method:  "GET",
		URL:     "http://myip.ipip.net",
		Headers: map[string]string{"User-Agent": "LatencyTest"},
		Body:    "",
	}

	stats := Reporter.GetStats()
	var targetNode *cloud.Node
	for _, node := range stats.Nodes {
		if node.URL == req.URL {
			targetNode = node
			break
		}
	}

	if targetNode == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"latency": 0,
			"success": false,
			"error":   "Node not found",
		})
		return
	}

	latency := int64(0)
	success := false

	jsonData, _ := json.Marshal(testReq)
	httpReq, _ := http.NewRequest("POST", targetNode.URL, bytes.NewBuffer(jsonData))
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		latency = time.Since(start).Milliseconds()
		log.Printf("Latency test failed for %s: %v", req.URL, err)
	} else {
		defer resp.Body.Close()
		latency = time.Since(start).Milliseconds()
		success = resp.StatusCode == 200
	}

	nodeLatencyMutex.Lock()
	nodeLatencyMap[req.URL] = latency
	nodeLatencyMutex.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"latency": latency,
		"success": success,
	})
}

func handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		conf, err := config.LoadConfig(configPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		data := ConfigData{
			ListenAddr:       conf.Client.ListenAddr,
			SocksAddr:        conf.Client.SocksAddr,
			User:             conf.Client.User,
			Password:         "",
			DashboardEnabled: conf.Client.DashboardAddr != "",
			Users: []UserCredential{
				{Username: conf.Client.User, Password: ""},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(data)

	case http.MethodPut:
		var data ConfigData
		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		conf, err := config.LoadConfig(configPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		conf.Client.ListenAddr = data.ListenAddr
		conf.Client.SocksAddr = data.SocksAddr

		if len(data.Users) > 0 {
			conf.Client.User = strings.TrimSpace(data.Users[0].Username)
			if password := strings.TrimSpace(data.Users[0].Password); password != "" {
				conf.Client.Password = password
			}
		} else {
			conf.Client.User = strings.TrimSpace(data.User)
			if password := strings.TrimSpace(data.Password); password != "" {
				conf.Client.Password = password
			}
		}

		if conf.Client.User == "" {
			conf.Client.Password = ""
		}

		if data.DashboardEnabled {
			if conf.Client.DashboardAddr == "" {
				conf.Client.DashboardAddr = "0.0.0.0:8081"
			}
		} else {
			conf.Client.DashboardAddr = ""
		}

		f, err := os.Create(configPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer f.Close()

		encoder := toml.NewEncoder(f)
		if err := encoder.Encode(conf); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleLogs(w http.ResponseWriter, r *http.Request) {
	logMutex.RLock()
	defer logMutex.RUnlock()

	filteredLogs := logHistory

	statusFilter := r.URL.Query().Get("status")
	timeRange := r.URL.Query().Get("timeRange")
	cutoff := parseTimeRangeCutoff(timeRange)

	if !cutoff.IsZero() {
		timeFiltered := make([]LogEntry, 0, len(filteredLogs))
		for _, entry := range filteredLogs {
			recorded := entry.Recorded
			if recorded.IsZero() {
				continue
			}
			if recorded.After(cutoff) || recorded.Equal(cutoff) {
				timeFiltered = append(timeFiltered, entry)
			}
		}
		filteredLogs = timeFiltered
	}

	if statusFilter != "" {
		statusFiltered := make([]LogEntry, 0, len(filteredLogs))
		for _, log := range filteredLogs {
			switch statusFilter {
			case "error":
				if log.Status >= 400 {
					statusFiltered = append(statusFiltered, log)
				}
			case "success":
				if log.Status >= 200 && log.Status < 400 {
					statusFiltered = append(statusFiltered, log)
				}
			case "warn":
				if log.Level == "warn" || log.Status >= 400 {
					statusFiltered = append(statusFiltered, log)
				}
			}
		}
		filteredLogs = statusFiltered
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(filteredLogs)
}

func handleWebSocketLogs(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer ws.Close()

	wsMutex.Lock()
	websocketClients[ws] = true
	wsMutex.Unlock()

	defer func() {
		wsMutex.Lock()
		delete(websocketClients, ws)
		wsMutex.Unlock()
	}()

	for {
		_, _, err := ws.ReadMessage()
		if err != nil {
			break
		}
	}
}

func AddLogEntry(entry LogEntry) {
	logMutex.Lock()
	defer logMutex.Unlock()

	if entry.Recorded.IsZero() {
		entry.Recorded = time.Now()
	}
	if entry.Time == "" {
		entry.Time = entry.Recorded.Format("15:04:05")
	}

	logHistory = append(logHistory, entry)
	if len(logHistory) > 1000 {
		logHistory = logHistory[1:]
	}

	broadcastLog(entry)
}

func broadcastLog(entry LogEntry) {
	wsMutex.RLock()
	defer wsMutex.RUnlock()

	data, _ := json.Marshal(entry)
	for client := range websocketClients {
		client.WriteMessage(websocket.TextMessage, data)
	}
}

func updateHistory() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if Reporter == nil {
			continue
		}

		stats := Reporter.GetStats()
		now := time.Now()

		historyMutex.Lock()
		if !lastHistoryTime.IsZero() {
			duration := now.Sub(lastHistoryTime).Seconds()
			newRequests := stats.TotalRequests - lastRequestCount
			qps := float64(newRequests) / duration
			avgLatency, p95Latency, p99Latency := collectLatencyStats(lastHistoryTime)

			historyBuffer = append(historyBuffer, HistoryEntry{
				Time:       now.Format("15:04:05"),
				QPS:        qps,
				AvgLatency: avgLatency,
				P95Latency: p95Latency,
				P99Latency: p99Latency,
			})

			if len(historyBuffer) > 100 {
				historyBuffer = historyBuffer[1:]
			}
		}
		lastRequestCount = stats.TotalRequests
		lastHistoryTime = now
		historyMutex.Unlock()
	}
}

func parseTimeRangeCutoff(value string) time.Time {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "1h":
		return time.Now().Add(-1 * time.Hour)
	case "6h":
		return time.Now().Add(-6 * time.Hour)
	case "24h":
		return time.Now().Add(-24 * time.Hour)
	case "7d":
		return time.Now().Add(-7 * 24 * time.Hour)
	default:
		return time.Time{}
	}
}

func collectLatencyStats(since time.Time) (int64, int64, int64) {
	logMutex.RLock()
	defer logMutex.RUnlock()

	latencies := make([]int64, 0, 32)
	var total int64
	for _, entry := range logHistory {
		if entry.Recorded.IsZero() || entry.Recorded.Before(since) {
			continue
		}
		if entry.Duration <= 0 {
			continue
		}
		latencies = append(latencies, entry.Duration)
		total += entry.Duration
	}

	if len(latencies) == 0 {
		return 0, 0, 0
	}

	sort.Slice(latencies, func(i, j int) bool {
		return latencies[i] < latencies[j]
	})

	avg := total / int64(len(latencies))
	p95 := percentileLatency(latencies, 0.95)
	p99 := percentileLatency(latencies, 0.99)
	return avg, p95, p99
}

func percentileLatency(values []int64, percentile float64) int64 {
	if len(values) == 0 {
		return 0
	}
	if len(values) == 1 {
		return values[0]
	}

	index := int(float64(len(values)-1) * percentile)
	if index < 0 {
		index = 0
	}
	if index >= len(values) {
		index = len(values) - 1
	}
	return values[index]
}

func extractRegion(url string) string {
	parsed, err := urlpkg.Parse(url)
	if err != nil {
		return "Unknown"
	}

	host := parsed.Host
	if host == "" {
		host = url
	}
	host = strings.ToLower(host)

	if hostOnly, _, err := net.SplitHostPort(host); err == nil {
		host = hostOnly
	}

	labels := strings.Split(host, ".")
	for _, label := range labels {
		if isRegionLabel(label) {
			return label
		}
	}

	if len(labels) >= 2 {
		return labels[0]
	}

	return "Unknown"
}

func base64StdSHA256(value string) string {
	sum := sha256.Sum256([]byte(value))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func base64HexSHA256(value string) string {
	sum := sha256.Sum256([]byte(value))
	hex := make([]byte, 0, len(sum)*2)
	const alphabet = "0123456789abcdef"
	for _, b := range sum {
		hex = append(hex, alphabet[b>>4], alphabet[b&0x0f])
	}
	return base64.StdEncoding.EncodeToString(hex)
}

func isRegionLabel(label string) bool {
	if !strings.Contains(label, "-") {
		return false
	}

	prefixes := []string{"cn-", "ap-", "eu-", "us-", "me-", "af-"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(label, prefix) {
			return true
		}
	}

	return false
}

func IsNodeEnabled(url string) bool {
	nodeMutex.RLock()
	defer nodeMutex.RUnlock()
	enabled, exists := nodeEnabledMap[url]
	if !exists {
		return true
	}
	return enabled
}
