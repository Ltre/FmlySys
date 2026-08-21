package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const LocalConfigFilename = "config.env"

type Config struct {
	Addr                   string
	DataDir                string
	ConfigFile             string
	DevMember              string
	DevAuthEnabled         bool
	WeChatAppID            string
	WeChatAppSecret        string
	AdminUsername          string
	AdminBootstrapPassword string
	MasterKey              string
}

func Load() (Config, error) {
	dataDir := getenvEnvOnly("FMLYSYS_DATA_DIR", "data")
	abs, err := filepath.Abs(dataDir)
	if err == nil {
		dataDir = abs
	}
	configFile := filepath.Join(dataDir, LocalConfigFilename)
	fileValues, err := loadConfigFile(configFile)
	if err != nil {
		return Config{}, err
	}

	value := func(key, fallback string) string {
		if v, ok := os.LookupEnv(key); ok {
			return v
		}
		if v, ok := fileValues[key]; ok {
			return v
		}
		return fallback
	}
	nonEmpty := func(key, fallback string) string {
		if v := strings.TrimSpace(value(key, "")); v != "" {
			return v
		}
		return fallback
	}

	return Config{
		Addr:                   nonEmpty("FMLYSYS_ADDR", "127.0.0.1:8080"),
		DataDir:                dataDir,
		ConfigFile:             configFile,
		DevMember:              nonEmpty("FMLYSYS_DEV_MEMBER", "Dev Admin"),
		DevAuthEnabled:         parseBool(value("FMLYSYS_DEV_AUTH_ENABLED", ""), false),
		WeChatAppID:            strings.TrimSpace(value("FMLYSYS_WECHAT_APP_ID", "")),
		WeChatAppSecret:        strings.TrimSpace(value("FMLYSYS_WECHAT_APP_SECRET", "")),
		AdminUsername:          nonEmpty("FMLYSYS_ADMIN_USERNAME", "admin"),
		AdminBootstrapPassword: value("FMLYSYS_ADMIN_BOOTSTRAP_PASSWORD", ""),
		MasterKey:              value("FMLYSYS_MASTER_KEY", ""),
	}, nil
}

func (c Config) WeChatConfigured() bool {
	return c.WeChatAppID != "" && c.WeChatAppSecret != ""
}

func loadConfigFile(path string) (map[string]string, error) {
	values := map[string]string{}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return values, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取本地配置文件 %s 失败：%w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "\ufeff"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("本地配置文件 %s 第 %d 行缺少 '='", path, lineNo)
		}
		key := strings.TrimSpace(parts[0])
		if key == "" {
			return nil, fmt.Errorf("本地配置文件 %s 第 %d 行配置名为空", path, lineNo)
		}
		value, err := parseConfigValue(parts[1])
		if err != nil {
			return nil, fmt.Errorf("本地配置文件 %s 第 %d 行：%w", path, lineNo, err)
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取本地配置文件 %s 失败：%w", path, err)
	}
	return values, nil
}

func parseConfigValue(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if strings.HasPrefix(raw, "\"") {
		v, err := strconv.Unquote(raw)
		if err != nil {
			return "", fmt.Errorf("双引号配置值无效：%w", err)
		}
		return v, nil
	}
	if strings.HasPrefix(raw, "'") {
		if len(raw) < 2 || !strings.HasSuffix(raw, "'") {
			return "", errors.New("单引号配置值未闭合")
		}
		return raw[1 : len(raw)-1], nil
	}
	return raw, nil
}

func getenvEnvOnly(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func parseBool(v string, fallback bool) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return fallback
	}
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}
