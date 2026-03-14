package config

import (
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Client ClientConfig `toml:"client"`
	Cloud  CloudConfig  `toml:"cloud"`
}

type ClientConfig struct {
	ListenAddr    string   `toml:"listen_addr"`
	ListenAddrs   []string `toml:"listen_addrs"`
	SocksAddr     string   `toml:"socks_addr"`
	User          string   `toml:"user"`
	Password      string   `toml:"password"`
	Dump          bool     `toml:"dump"`
	DumpFile      string   `toml:"dump_file"`
	DashboardAddr string   `toml:"dashboard_addr"`
	Debug         bool     `toml:"debug"`
}

type CloudConfig struct {
	FunctionURLs []string    `toml:"function_urls"`
	Region       string      `toml:"region"`
	Token        string      `toml:"token"`
	Redis        RedisConfig `toml:"redis"`
}

type RedisConfig struct {
	Addr            string `toml:"addr"`
	Password        string `toml:"password"`
	DB              int    `toml:"db"`
	KeyPrefix       string `toml:"key_prefix"`
	LeaseTTLSeconds int    `toml:"lease_ttl_seconds"`
	CooldownSeconds int    `toml:"cooldown_seconds"`
	AcquireRetries  int    `toml:"acquire_retries"`
	RetryDelayMs    int    `toml:"retry_delay_ms"`
}

func (c ClientConfig) HTTPListenAddrs() []string {
	addrs := normalizeListenAddrs(c.ListenAddrs)
	if len(addrs) > 0 {
		return addrs
	}

	if addr := strings.TrimSpace(c.ListenAddr); addr != "" {
		return []string{addr}
	}

	return []string{"0.0.0.0:10800"}
}

func LoadConfig(path string) (*Config, error) {
	var conf Config
	if _, err := toml.DecodeFile(path, &conf); err != nil {
		return nil, err
	}
	return &conf, nil
}

func CreateDefaultConfig(path string) error {
	defaultConf := Config{
		Client: ClientConfig{
			ListenAddr: "0.0.0.0:10800",
			Debug:      false,
		},
		Cloud: CloudConfig{
			FunctionURLs: []string{
				"https://your-function.cn-shanghai.fc.aliyuncs.com",
				"https://your-function.cn-shenzhen.fc.aliyuncs.com",
			},
			Region: "multi-region",
			Redis: RedisConfig{
				Addr:            "redis:6379",
				DB:              0,
				KeyPrefix:       "cloud_proxy_pool",
				LeaseTTLSeconds: 120,
				CooldownSeconds: 120,
				AcquireRetries:  3,
				RetryDelayMs:    200,
			},
		},
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := toml.NewEncoder(f)
	return enc.Encode(defaultConf)
}

func normalizeListenAddrs(addrs []string) []string {
	seen := make(map[string]struct{}, len(addrs))
	normalized := make([]string, 0, len(addrs))

	for _, addr := range addrs {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			continue
		}
		if _, exists := seen[addr]; exists {
			continue
		}
		seen[addr] = struct{}{}
		normalized = append(normalized, addr)
	}

	return normalized
}
