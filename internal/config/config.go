package config

import (
	"os"
	"path/filepath"
)

type Config struct {
	Addr      string
	DataDir   string
	DevMember string
}

func Load() Config {
	addr := getenv("FMLYSYS_ADDR", ":8080")
	dataDir := getenv("FMLYSYS_DATA_DIR", "data")
	devMember := getenv("FMLYSYS_DEV_MEMBER", "开发管理员")
	abs, err := filepath.Abs(dataDir)
	if err == nil {
		dataDir = abs
	}
	return Config{Addr: addr, DataDir: dataDir, DevMember: devMember}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
