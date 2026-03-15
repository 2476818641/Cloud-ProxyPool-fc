package cloud

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type FunctionRequest struct {
	Protocol     string            `json:"protocol,omitempty"`
	Method       string            `json:"method,omitempty"`
	URL          string            `json:"url,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	Body         string            `json:"body,omitempty"`
	IsBodyBase64 bool              `json:"is_body_base64,omitempty"`
	Command      int               `json:"command,omitempty"`
	Host         string            `json:"host,omitempty"`
	Port         string            `json:"port,omitempty"`
}

type FunctionResponse struct {
	StatusCode      int               `json:"status_code"`
	Headers         map[string]string `json:"headers"`
	Content         string            `json:"content"`
	IsContentBase64 bool              `json:"is_content_base64"`
}

type Node struct {
	URL          string
	FailureCount int
	LastFailTime time.Time
	mu           sync.RWMutex
}

func (n *Node) MarkFailure() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.FailureCount++
	n.LastFailTime = time.Now()
}

func (n *Node) MarkSuccess() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.FailureCount > 0 {
		n.FailureCount = 0
	}
}

func (n *Node) IsHealthy() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.FailureCount >= 5 && time.Since(n.LastFailTime) < 2*time.Minute {
		return false
	}
	return true
}

type RedisOptions struct {
	Addr            string
	Password        string
	DB              int
	KeyPrefix       string
	LeaseTTLSeconds int
	CooldownSeconds int
	AcquireRetries  int
	RetryDelayMs    int
}

type leaseHandle struct {
	node  *Node
	token string
	key   string
}

type Provider struct {
	Nodes        []*Node
	CurrentIndex int
	Client       *http.Client
	mu           sync.Mutex

	redisEnabled bool
	redisClient  *redis.Client
	ctx          context.Context
	keyPrefix    string
	leaseTTL     time.Duration
	cooldownTTL  time.Duration
	acquireRetry int
	retryDelay   time.Duration

	NodeEnabledFunc func(url string) bool
}

func NewProvider(functionURLs []string, redisOpt RedisOptions) (*Provider, error) {
	nodes := make([]*Node, len(functionURLs))
	for i, url := range functionURLs {
		nodes[i] = &Node{URL: url}
	}

	provider := &Provider{
		Nodes:        nodes,
		CurrentIndex: 0,
		Client: &http.Client{
			Timeout: 90 * time.Second,
		},
		ctx:          context.Background(),
		keyPrefix:    redisOpt.KeyPrefix,
		leaseTTL:     durationOrDefault(redisOpt.LeaseTTLSeconds, 120) * time.Second,
		cooldownTTL:  durationOrDefault(redisOpt.CooldownSeconds, 120) * time.Second,
		acquireRetry: intOrDefault(redisOpt.AcquireRetries, 3),
		retryDelay:   durationOrDefault(redisOpt.RetryDelayMs, 200) * time.Millisecond,
	}

	if redisOpt.Addr == "" {
		return provider, nil
	}

	if provider.keyPrefix == "" {
		provider.keyPrefix = "cloud_proxy_pool"
	}

	client := redis.NewClient(&redis.Options{
		Addr:     redisOpt.Addr,
		Password: redisOpt.Password,
		DB:       redisOpt.DB,
	})

	if err := client.Ping(provider.ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}

	provider.redisClient = client
	provider.redisEnabled = true
	return provider, nil
}

func (p *Provider) UsingRedisLease() bool {
	return p.redisEnabled
}

func (p *Provider) SetNodeEnabledFunc(fn func(url string) bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.NodeEnabledFunc = fn
}

func (p *Provider) HealthCheck() (string, error) {
	payload := FunctionRequest{
		Method: "GET",
		URL:    "http://myip.ipip.net",
		Headers: map[string]string{
			"User-Agent": "CloudProxyPool-HealthCheck",
		},
	}

	var lastErr error
	for _, node := range p.Nodes {
		resp, err := p.invokeNode(node, nil, payload)
		if err == nil && resp.StatusCode == 200 {
			content, decodeErr := base64.StdEncoding.DecodeString(resp.Content)
			if decodeErr == nil {
				return string(content), nil
			}
			lastErr = decodeErr
			continue
		}
		lastErr = err
	}

	return "", fmt.Errorf("all cloud nodes are unavailable, last error: %v", lastErr)
}

func (p *Provider) Invoke(payload FunctionRequest) (*FunctionResponse, error) {
	if p.redisEnabled {
		lease, err := p.acquireRedisLease()
		if err == nil {
			return p.invokeNode(lease.node, lease, payload)
		}
		log.Printf("[warn] redis lease acquisition failed, falling back to local scheduler: %v", err)
	}

	node := p.getNextLocalNode()
	if node == nil {
		return nil, fmt.Errorf("no function URLs configured")
	}
	return p.invokeNode(node, nil, payload)
}

func (p *Provider) invokeNode(node *Node, lease *leaseHandle, payload FunctionRequest) (*FunctionResponse, error) {
	if lease != nil {
		defer p.releaseLease(lease)
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", node.URL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.Client.Do(req)
	if err != nil {
		node.MarkFailure()
		p.markRedisCooldown(node)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		node.MarkFailure()
		p.markRedisCooldown(node)
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("function invoke failed: %s | response: %s", resp.Status, string(body))
	}

	var funcResp FunctionResponse
	if err := json.NewDecoder(resp.Body).Decode(&funcResp); err != nil {
		node.MarkFailure()
		p.markRedisCooldown(node)
		return nil, fmt.Errorf("failed to decode function response: %v", err)
	}

	node.MarkSuccess()
	return &funcResp, nil
}

func (p *Provider) getNextLocalNode() *Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.Nodes) == 0 {
		return nil
	}

	startIdx := p.CurrentIndex
	for i := 0; i < len(p.Nodes); i++ {
		idx := (startIdx + i) % len(p.Nodes)
		node := p.Nodes[idx]
		if !node.IsHealthy() {
			continue
		}
		if p.NodeEnabledFunc != nil && !p.NodeEnabledFunc(node.URL) {
			continue
		}
		p.CurrentIndex = (idx + 1) % len(p.Nodes)
		return node
	}

	log.Printf("[warn] all cloud nodes are locally unhealthy or disabled, forcing node: %s", p.Nodes[p.CurrentIndex].URL)
	node := p.Nodes[p.CurrentIndex]
	p.CurrentIndex = (p.CurrentIndex + 1) % len(p.Nodes)
	return node
}

func (p *Provider) acquireRedisLease() (*leaseHandle, error) {
	if len(p.Nodes) == 0 {
		return nil, fmt.Errorf("no function URLs configured")
	}

	for attempt := 0; attempt < p.acquireRetry; attempt++ {
		startIdx := p.nextStartIndex()
		for i := 0; i < len(p.Nodes); i++ {
			idx := (startIdx + i) % len(p.Nodes)
			node := p.Nodes[idx]
			if !node.IsHealthy() {
				continue
			}
			if p.isRedisCooldown(node) {
				continue
			}
			if p.NodeEnabledFunc != nil && !p.NodeEnabledFunc(node.URL) {
				continue
			}
			lease, err := p.tryLeaseNode(node)
			if err == nil {
				return lease, nil
			}
		}
		time.Sleep(p.retryDelay)
	}

	return nil, fmt.Errorf("no redis lease available after %d attempts", p.acquireRetry)
}

func (p *Provider) nextStartIndex() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.Nodes) == 0 {
		return 0
	}
	start := p.CurrentIndex
	p.CurrentIndex = (p.CurrentIndex + 1) % len(p.Nodes)
	return start
}

func (p *Provider) tryLeaseNode(node *Node) (*leaseHandle, error) {
	token := fmt.Sprintf("%d", time.Now().UnixNano())
	key := p.leaseKey(node)
	ok, err := p.redisClient.SetNX(p.ctx, key, token, p.leaseTTL).Result()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("node already leased")
	}
	return &leaseHandle{node: node, token: token, key: key}, nil
}

func (p *Provider) releaseLease(lease *leaseHandle) {
	if !p.redisEnabled || lease == nil {
		return
	}

	const releaseScript = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0
`
	if err := p.redisClient.Eval(p.ctx, releaseScript, []string{lease.key}, lease.token).Err(); err != nil {
		log.Printf("[warn] failed to release redis lease for %s: %v", lease.node.URL, err)
	}
}

func (p *Provider) isRedisCooldown(node *Node) bool {
	if !p.redisEnabled {
		return false
	}
	exists, err := p.redisClient.Exists(p.ctx, p.cooldownKey(node)).Result()
	if err != nil {
		log.Printf("[warn] failed to check redis cooldown for %s: %v", node.URL, err)
		return false
	}
	return exists > 0
}

func (p *Provider) markRedisCooldown(node *Node) {
	if !p.redisEnabled {
		return
	}
	if err := p.redisClient.Set(p.ctx, p.cooldownKey(node), "1", p.cooldownTTL).Err(); err != nil {
		log.Printf("[warn] failed to set redis cooldown for %s: %v", node.URL, err)
	}
}

func (p *Provider) leaseKey(node *Node) string {
	return fmt.Sprintf("%s:lease:%s", p.keyPrefix, nodeKey(node.URL))
}

func (p *Provider) cooldownKey(node *Node) string {
	return fmt.Sprintf("%s:cooldown:%s", p.keyPrefix, nodeKey(node.URL))
}

func nodeKey(url string) string {
	sum := sha1.Sum([]byte(url))
	return hex.EncodeToString(sum[:])
}

func durationOrDefault(value, fallback int) time.Duration {
	if value <= 0 {
		return time.Duration(fallback)
	}
	return time.Duration(value)
}

func intOrDefault(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}
