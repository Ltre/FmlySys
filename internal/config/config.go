package config

import (
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Addr                   string
	DataDir                string
	DevMember              string
	DevAuthEnabled         bool
	WeChatAppID            string
	WeChatAppSecret        string
	WeChatRedirectURL      string
	AdminUsername          string
	AdminBootstrapPassword string
	MasterKey              string
}

func Load() Config {
	addr := getenv("FMLYSYS_ADDR", "127.0.0.1:8080")
	dataDir := getenv("FMLYSYS_DATA_DIR", "data")
	abs, err := filepath.Abs(dataDir)
	if err == nil {
		dataDir = abs
	}
	return Config{
		Addr:                   addr,
		DataDir:                dataDir,
		DevMember:              getenv("FMLYSYS_DEV_MEMBER", "Dev Admin"),
		DevAuthEnabled:         envBool("FMLYSYS_DEV_AUTH_ENABLED", false),
		WeChatAppID:            strings.TrimSpace(os.Getenv("FMLYSYS_WECHAT_APP_ID")),
		WeChatAppSecret:        strings.TrimSpace(os.Getenv("FMLYSYS_WECHAT_APP_SECRET")),
		WeChatRedirectURL:      strings.TrimSpace(os.Getenv("FMLYSYS_WECHAT_REDIRECT_URL")),
		AdminUsername:          getenv("FMLYSYS_ADMIN_USERNAME", "admin"),
		AdminBootstrapPassword: os.Getenv("FMLYSYS_ADMIN_BOOTSTRAP_PASSWORD"),
		MasterKey:              os.Getenv("FMLYSYS_MASTER_KEY"),
	}
}

func (c Config) WeChatConfigured() bool {
	return c.WeChatAppID != "" && c.WeChatAppSecret != "" && c.WeChatRedirectURL != ""
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if v == "" {
		return fallback
	}
	return v == "1" || v == "true" || v == "yes" || v == "on"
}
