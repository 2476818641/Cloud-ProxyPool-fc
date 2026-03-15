package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"sync"
	"time"

	"cloud-proxy-pool/models"
)

type UserManager struct {
	users map[string]*models.User
	mu    sync.RWMutex
}

type AuthConfig struct {
	AdminUsername      string
	AdminPasswordHash  string
	EnableRegistration bool
}

func NewUserManager(config AuthConfig) *UserManager {
	mgr := &UserManager{
		users: make(map[string]*models.User),
	}

	if config.AdminUsername != "" {
		admin := &models.User{
			Username:     config.AdminUsername,
			PasswordHash: config.AdminPasswordHash,
			Role:         models.RoleAdmin,
			Enabled:      true,
			CreatedAt:    time.Now(),
		}
		mgr.users[admin.Username] = admin
	}

	return mgr
}

func (m *UserManager) HashPassword(password string) string {
	hash := sha256.Sum256([]byte(password))
	return base64.StdEncoding.EncodeToString(hash[:])
}

func (m *UserManager) Create(username, password string, role models.UserRole, enabled bool) (*models.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.users[username]; exists {
		return nil, ErrUserAlreadyExists
	}

	user := &models.User{
		Username:     username,
		PasswordHash: m.HashPassword(password),
		Role:         role,
		Enabled:      enabled,
		CreatedAt:    time.Now(),
	}

	m.users[username] = user
	return user, nil
}

func (m *UserManager) Get(username string) (*models.User, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	user, exists := m.users[username]
	if !exists || !user.Enabled {
		return nil, false
	}
	return user, true
}

func (m *UserManager) Authenticate(username, password string) (*models.User, error) {
	user, exists := m.Get(username)
	if !exists {
		return nil, ErrInvalidCredentials
	}

	if !user.CheckPassword(password) {
		return nil, ErrInvalidCredentials
	}

	return user, nil
}

func (m *UserManager) UpdatePassword(username, oldPassword, newPassword string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	user, exists := m.users[username]
	if !exists {
		return ErrUserNotFound
	}

	if !user.CheckPassword(oldPassword) {
		return ErrInvalidCredentials
	}

	user.PasswordHash = m.HashPassword(newPassword)
	return nil
}

func (m *UserManager) Delete(username string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.users[username]; !exists {
		return ErrUserNotFound
	}

	delete(m.users, username)
	return nil
}

func (m *UserManager) List() []models.User {
	m.mu.RLock()
	defer m.mu.RUnlock()

	users := make([]models.User, 0, len(m.users))
	for _, user := range m.users {
		users = append(users, models.User{
			Username:     user.Username,
			PasswordHash: user.PasswordHash,
			Role:         user.Role,
			Enabled:      user.Enabled,
			CreatedAt:    user.CreatedAt,
			TrafficLimit: user.TrafficLimit,
			TrafficUsed:  user.TrafficUsed,
			RequestLimit: user.RequestLimit,
			RequestCount: user.RequestCount,
			RateLimit:    user.RateLimit,
			LastLoginAt:  user.LastLoginAt,
			LastLoginIP:  user.LastLoginIP,
		})
	}
	return users
}

func (m *UserManager) UpdateUser(username string, updates func(*models.User)) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	user, exists := m.users[username]
	if !exists {
		return ErrUserNotFound
	}

	updates(user)
	return nil
}

func (m *UserManager) SetQuota(username string, trafficLimit, requestLimit int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	user, exists := m.users[username]
	if !exists {
		return ErrUserNotFound
	}

	user.SetQuota(trafficLimit, requestLimit)

	return nil
}

type RateLimiter struct {
	requests map[string][]time.Time
	mu       sync.Mutex
	limit    int
	window   time.Duration
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
}

func (r *RateLimiter) Check(username string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	userRequests, exists := r.requests[username]

	if !exists {
		r.requests[username] = []time.Time{now}
		return true
	}

	cutoff := now.Add(-r.window)
	var validRequests []time.Time
	for _, reqTime := range userRequests {
		if reqTime.After(cutoff) {
			validRequests = append(validRequests, reqTime)
		}
	}

	if len(validRequests) >= r.limit {
		return false
	}

	validRequests = append(validRequests, now)
	r.requests[username] = validRequests
	return true
}

func (r *RateLimiter) Clear(username string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.requests, username)
}

var (
	ErrUserAlreadyExists  = &AuthError{Type: "user_exists", Message: "用户已存在"}
	ErrUserNotFound       = &AuthError{Type: "user_not_found", Message: "用户不存在"}
	ErrInvalidCredentials = &AuthError{Type: "invalid_credentials", Message: "无效的用户名或密码"}
	ErrRateLimitExceeded  = &AuthError{Type: "rate_limit", Message: "请求速率限制已超出"}
)

type AuthError struct {
	Type    string
	Message string
}

func (e *AuthError) Error() string {
	return e.Message
}
