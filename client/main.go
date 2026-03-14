package main

import (
	"cloud-proxy-pool/cloud"
	"cloud-proxy-pool/config"
	"cloud-proxy-pool/dashboard"
	"cloud-proxy-pool/proxy"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/fatih/color"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "C", "config.toml", "Path to configuration file")
	flag.Parse()

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		color.Yellow("Config file not found, creating default config: %s...", configPath)
		if err := config.CreateDefaultConfig(configPath); err != nil {
			log.Fatalf("failed to create default config: %v", err)
		}
		color.Yellow("Edit %s and fill your function URLs before restarting.", configPath)
		return
	}

	conf, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	if len(conf.Cloud.FunctionURLs) == 0 {
		color.Red("error: set valid function URLs in %s", configPath)
		return
	}

	provider, err := cloud.NewProvider(conf.Cloud.FunctionURLs, cloud.RedisOptions{
		Addr:            conf.Cloud.Redis.Addr,
		Password:        conf.Cloud.Redis.Password,
		DB:              conf.Cloud.Redis.DB,
		KeyPrefix:       conf.Cloud.Redis.KeyPrefix,
		LeaseTTLSeconds: conf.Cloud.Redis.LeaseTTLSeconds,
		CooldownSeconds: conf.Cloud.Redis.CooldownSeconds,
		AcquireRetries:  conf.Cloud.Redis.AcquireRetries,
		RetryDelayMs:    conf.Cloud.Redis.RetryDelayMs,
	})
	if err != nil {
		log.Fatalf("failed to initialize provider: %v", err)
	}

	color.Cyan("Running health check...")
	ip, err := provider.HealthCheck()
	if err != nil {
		color.Red("health check failed: %v", err)
		color.Red("check your function URLs, Redis config, and network connectivity.")
		os.Exit(1)
	}

	showBanner(conf, ip, provider.UsingRedisLease())
	listenAddrs := conf.Client.HTTPListenAddrs()

	srv := proxy.NewProxyServer(
		listenAddrs,
		conf.Client.SocksAddr,
		conf.Client.User,
		conf.Client.Password,
		conf.Client.Dump,
		conf.Client.DumpFile,
		provider,
		conf.Client.Debug,
	)

	if conf.Client.DashboardAddr != "" {
		go dashboard.StartDashboard(conf.Client.DashboardAddr, srv)
	}

	if err := srv.Start(); err != nil {
		log.Fatal(err)
	}
}

func showBanner(conf *config.Config, ip string, redisLease bool) {
	listenAddrs := conf.Client.HTTPListenAddrs()
	banner := `
   ________                __   ____                        ____             __
  / ____/ /___  __  ______/ /  / __ \_________  ____  __  _/ __ \____  ____ / /
 / /   / / __ \/ / / / __  /  / /_/ / ___/ __ \/ __ \/ / / / /_/ / __ \/ __ \/ /
/ /___/ / /_/ / /_/ / /_/ /  / ____/ /  / /_/ / /_>  </ /_/ / ____/ /_/ / /_/ / /
\____/_/\____/\__,_/\__,_/  /_/   /_/   \____/\___/\__, /_/     \____/\____/_/
                                                  /____/
`
	color.HiBlue(banner)
	fmt.Println("================================================================")
	color.Green(" [Client] Listen Addr : %s", strings.Join(listenAddrs, ", "))
	color.Green(" [Cloud]  Node Count  : %d", len(conf.Cloud.FunctionURLs))
	color.Green(" [Health] Check       : PASS")
	color.Green(" [ExitIP] Current IP  : %s", ip)
	if redisLease {
		color.Green(" [Lease]  Scheduler   : Redis distributed lease")
	} else {
		color.Yellow(" [Lease]  Scheduler   : Local round robin")
	}
	fmt.Println("================================================================")
	fmt.Println("Configure your tools to use this proxy.")
	fmt.Println("Example: export http_proxy=http://" + listenAddrs[0] + " https_proxy=http://" + listenAddrs[0])
	fmt.Println("Install the CA certificate in client/certs before using HTTPS MITM.")
}
