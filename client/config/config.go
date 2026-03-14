package config

import (
	"os"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Client ClientConfig `toml:"client"`
	Cloud  CloudConfig  `toml:"cloud"`
}

type ClientConfig struct {
	ListenAddr    string `toml:"listen_addr"`
	SocksAddr     string `toml:"socks_addr"`
	User          string `toml:"user"`
	Password      string `toml:"password"`
	Dump          bool   `toml:"dump"`
	DumpFile      string `toml:"dump_file"`
	DashboardAddr string `toml:"dashboard_addr"`
	Debug         bool   `toml:"debug"`
}

type CloudConfig struct {
	FunctionURLs []string `toml:"function_urls"`
	Region       string   `toml:"region"`
	Token        string   `toml:"token"`
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
			ListenAddr: "127.0.0.1:10800",
			Debug:      false,
		},
		Cloud: CloudConfig{
			FunctionURLs: []string{
				"https://your-function.cn-shanghai.fc.aliyuncs.com",
				"https://your-function.cn-shenzhen.fc.aliyuncs.com",
			},
			Region: "multi-region",
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
