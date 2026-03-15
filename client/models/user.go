package models

import (
	"crypto/sha256"
	"encoding/base64"
	"sync"
	"time"
)

type UserRole string

const (
	RoleAdmin UserRole = "admin"
	RoleUser  UserRole = "user"
)

type User struct {
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"` // 不在JSON中序列化
	Role         UserRole  `json:"role"`
	Enabled      bool      `json:"enabled"`
	CreatedAt    time.Time `json:"created_at"`
	TrafficLimit int64     `json:"traffic_limit"` // 字节
	TrafficUsed  int64     `json:"traffic_used"`
	RequestLimit int64     `json:"request_limit"`
	RequestCount int64     `json:"request_count"`
	RateLimit    int       `json:"rate_limit"` // 每秒请求数
	LastLoginAt  time.Time `json:"last_login_at"`
	LastLoginIP  string    `json:"last_login_ip"`
	mu           sync.RWMutex
}

type UserStats struct {
	Username         string  `json:"username"`
	TotalTraffic     int64   `json:"total_traffic"`
	TotalRequests    int64   `json:"total_requests"`
	SuccessRequests  int64   `json:"success_requests"`
	FailedRequests   int64   `json:"failed_requests"`
	AvgLatency       float64 `json:"avg_latency"`
	P95Latency       float64 `json:"p95_latency"`
	P99Latency       float64 `json:"p99_latency"`
	TrafficLimit     int64   `json:"traffic_limit"`
	TrafficUsed      int64   `json:"traffic_used"`
	TrafficRemaining int64   `json:"traffic_remaining"`
	RequestLimit     int64   `json:"request_limit"`
	RequestUsed      int64   `json:"request_used"`
	RequestRemaining int64   `json:"request_remaining"`
	LimitExceeded    bool    `json:"limit_exceeded"`
}

func (u *User) CheckPassword(password string) bool {
	u.mu.RLock()
	defer u.mu.RUnlock()

	sum := sha256.Sum256([]byte(password))
	hashedInput := base64.StdEncoding.EncodeToString(sum[:])
	return u.PasswordHash == hashedInput
}

func (u *User) UpdateLogin(ip string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.LastLoginAt = time.Now()
	u.LastLoginIP = ip
}

func (u *User) AddTraffic(bytes int64) bool {
	u.mu.Lock()
	defer u.mu.Unlock()

	if u.TrafficLimit > 0 && u.TrafficUsed+bytes > u.TrafficLimit {
		return false
	}
	u.TrafficUsed += bytes
	return true
}

func (u *User) AddRequest() bool {
	u.mu.Lock()
	defer u.mu.Unlock()

	if u.RequestLimit > 0 && u.RequestCount >= u.RequestLimit {
		return false
	}
	u.RequestCount++
	return true
}

func (u *User) GetStats() UserStats {
	u.mu.RLock()
	defer u.mu.RUnlock()

	trafficRemaining := int64(0)
	if u.TrafficLimit > 0 {
		trafficRemaining = u.TrafficLimit - u.TrafficUsed
	}

	requestRemaining := int64(0)
	if u.RequestLimit > 0 {
		requestRemaining = u.RequestLimit - u.RequestCount
	}

	return UserStats{
		Username:         u.Username,
		TotalTraffic:     u.TrafficUsed,
		TotalRequests:    u.RequestCount,
		TrafficLimit:     u.TrafficLimit,
		TrafficUsed:      u.TrafficUsed,
		TrafficRemaining: trafficRemaining,
		RequestLimit:     u.RequestLimit,
		RequestUsed:      u.RequestCount,
		RequestRemaining: requestRemaining,
		LimitExceeded: (u.TrafficLimit > 0 && u.TrafficUsed >= u.TrafficLimit) ||
			(u.RequestLimit > 0 && u.RequestCount >= u.RequestLimit),
	}
}

func (u *User) HasPermission(permission string) bool {
	if u.Role == RoleAdmin {
		return true
	}
	return false
}

func (u *User) SetQuota(trafficLimit, requestLimit int64) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.TrafficLimit = trafficLimit
	u.RequestLimit = requestLimit
}

func (u *User) SetRateLimit(limit int) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.RateLimit = limit
}

func (u *User) SetRole(role UserRole) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.Role = role
}

func (u *User) SetEnabled(enabled bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.Enabled = enabled
}

type UserRecord struct {
	Timestamp  time.Time `json:"timestamp"`
	Username   string    `json:"username"`
	Method     string    `json:"method"`
	URL        string    `json:"url"`
	StatusCode int       `json:"status_code"`
	BytesSent  int64     `json:"bytes_sent"`
	BytesRecv  int64     `json:"bytes_recv"`
	Latency    int64     `json:"latency"` // 毫秒
	Success    bool      `json:"success"`
	Region     string    `json:"region"`
}

type UserQuota struct {
	Username     string    `json:"username"`
	TrafficLimit int64     `json:"traffic_limit"`
	TrafficUsed  int64     `json:"traffic_used"`
	RequestLimit int64     `json:"request_limit"`
	RequestCount int64     `json:"request_count"`
	ResetAt      time.Time `json:"reset_at"`
	PlanType     string    `json:"plan_type"`
	PlanName     string    `json:"plan_name"`
	PricePerGB   float64   `json:"price_per_gb"`
	PricePerReq  float64   `json:"price_per_req"`
	CurrentBill  float64   `json:"current_bill"`
	mu           sync.RWMutex
}

func (q *UserQuota) AddTraffic(bytes int64) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.TrafficLimit > 0 && q.TrafficUsed+bytes > q.TrafficLimit {
		return ErrTrafficLimitExceeded
	}
	q.TrafficUsed += bytes
	q.CurrentBill += float64(bytes) / (1024 * 1024 * 1024) * q.PricePerGB
	return nil
}

func (q *UserQuota) AddRequest() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.RequestLimit > 0 && q.RequestCount >= q.RequestLimit {
		return ErrRequestLimitExceeded
	}
	q.RequestCount++
	q.CurrentBill += q.PricePerReq
	return nil
}

func (q *UserQuota) GetQuota() UserQuotaInfo {
	q.mu.RLock()
	defer q.mu.RUnlock()

	trafficRemaining := int64(0)
	if q.TrafficLimit > 0 {
		trafficRemaining = q.TrafficLimit - q.TrafficUsed
	}

	requestRemaining := int64(0)
	if q.RequestLimit > 0 {
		requestRemaining = q.RequestLimit - q.RequestCount
	}

	return UserQuotaInfo{
		TrafficLimit:     q.TrafficLimit,
		TrafficUsed:      q.TrafficUsed,
		TrafficRemaining: trafficRemaining,
		RequestLimit:     q.RequestLimit,
		RequestUsed:      q.RequestCount,
		RequestRemaining: requestRemaining,
		PlanType:         q.PlanType,
		PlanName:         q.PlanName,
		CurrentBill:      q.CurrentBill,
		ResetAt:          q.ResetAt,
	}
}

type UserQuotaInfo struct {
	TrafficLimit     int64     `json:"traffic_limit"`
	TrafficUsed      int64     `json:"traffic_used"`
	TrafficRemaining int64     `json:"traffic_remaining"`
	RequestLimit     int64     `json:"request_limit"`
	RequestUsed      int64     `json:"request_used"`
	RequestRemaining int64     `json:"request_remaining"`
	PlanType         string    `json:"plan_type"`
	PlanName         string    `json:"plan_name"`
	CurrentBill      float64   `json:"current_bill"`
	ResetAt          time.Time `json:"reset_at"`
}

var (
	ErrTrafficLimitExceeded = &QuotaError{Type: "traffic", Message: "流量限制已超出"}
	ErrRequestLimitExceeded = &QuotaError{Type: "request", Message: "请求限制已超出"}
)

type QuotaError struct {
	Type    string
	Message string
}

func (e *QuotaError) Error() string {
	return e.Message
}
