package config

import (
	"fmt"
	"math"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Host             string
	Port             int
	DataDir          string
	MaxPasteBytes    int
	DefaultTTL       int64
	MaxTTL           int64
	MaxStorageBytes  int64
	MaxDatabaseBytes int64
	MaxPastes        int64
	CreateRate       float64
	CreateBurst      int
	TrustedProxies   []netip.Prefix
	BasicUser        string
	BasicPassword    string
}

func Defaults() Config {
	return Config{Host: "127.0.0.1", Port: 8080, DataDir: "data", MaxPasteBytes: 65536, DefaultTTL: 86400, MaxTTL: 2592000, MaxStorageBytes: 64 << 20, MaxDatabaseBytes: 256 << 20, MaxPastes: 10000, CreateRate: 0.2, CreateBurst: 5}
}

func Load() (Config, error) {
	c := Defaults()
	for key, dst := range map[string]*string{"HOST": &c.Host, "DATA_DIR": &c.DataDir, "BASIC_AUTH_USER": &c.BasicUser, "BASIC_AUTH_PASSWORD": &c.BasicPassword} {
		if v, ok := os.LookupEnv("LOGGERBIN_" + key); ok {
			*dst = v
		}
	}
	for key, dst := range map[string]*int{"PORT": &c.Port, "MAX_PASTE_BYTES": &c.MaxPasteBytes, "CREATE_BURST": &c.CreateBurst} {
		if v, ok := os.LookupEnv("LOGGERBIN_" + key); ok {
			n, err := strconv.Atoi(v)
			if err != nil {
				return c, fmt.Errorf("LOGGERBIN_%s must be an integer", key)
			}
			*dst = n
		}
	}
	for key, dst := range map[string]*int64{"DEFAULT_TTL_SECONDS": &c.DefaultTTL, "MAX_TTL_SECONDS": &c.MaxTTL, "MAX_STORAGE_BYTES": &c.MaxStorageBytes, "MAX_DATABASE_BYTES": &c.MaxDatabaseBytes, "MAX_PASTES": &c.MaxPastes} {
		if v, ok := os.LookupEnv("LOGGERBIN_" + key); ok {
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return c, fmt.Errorf("LOGGERBIN_%s must be an integer", key)
			}
			*dst = n
		}
	}
	if v, ok := os.LookupEnv("LOGGERBIN_CREATE_RATE"); ok {
		n, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return c, fmt.Errorf("LOGGERBIN_CREATE_RATE must be a number")
		}
		c.CreateRate = n
	}
	if v := os.Getenv("LOGGERBIN_TRUSTED_PROXIES"); v != "" {
		for _, raw := range strings.Split(v, ",") {
			p, err := netip.ParsePrefix(strings.TrimSpace(raw))
			if err != nil {
				return c, fmt.Errorf("LOGGERBIN_TRUSTED_PROXIES must contain CIDR prefixes")
			}
			c.TrustedProxies = append(c.TrustedProxies, p.Masked())
		}
	}
	return c, c.Validate()
}

func (c Config) Validate() error {
	if c.Host == "" || c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("a host and a port between 1 and 65535 are required")
	}
	if strings.TrimSpace(c.DataDir) == "" {
		return fmt.Errorf("LOGGERBIN_DATA_DIR must not be empty")
	}
	if c.MaxPasteBytes < 1 || c.MaxPasteBytes > 1<<20 {
		return fmt.Errorf("LOGGERBIN_MAX_PASTE_BYTES must be between 1 and 1048576")
	}
	if c.DefaultTTL < 1 || c.MaxTTL < c.DefaultTTL || c.MaxTTL > 31536000 {
		return fmt.Errorf("TTL values must satisfy 1 <= default <= maximum <= 31536000 seconds")
	}
	if c.MaxStorageBytes < int64(c.MaxPasteBytes)*2 || c.MaxStorageBytes > 1<<40 {
		return fmt.Errorf("storage quota must fit a paste and be at most 1 TiB")
	}
	if c.MaxDatabaseBytes < int64(c.MaxPasteBytes)*4+(1<<20) || c.MaxDatabaseBytes > 1<<40 {
		return fmt.Errorf("database file quota must allow a paste plus overhead and be at most 1 TiB")
	}
	if c.MaxPastes < 1 || c.MaxPastes > 1000000 {
		return fmt.Errorf("LOGGERBIN_MAX_PASTES must be between 1 and 1000000")
	}
	if math.IsNaN(c.CreateRate) || math.IsInf(c.CreateRate, 0) || c.CreateRate <= 0 || c.CreateRate > 1000 || c.CreateBurst < 1 || c.CreateBurst > 1000 {
		return fmt.Errorf("creation rate and burst must be positive and at most 1000")
	}
	if (c.BasicUser == "") != (c.BasicPassword == "") {
		return fmt.Errorf("both Basic authentication settings must be supplied")
	}
	if c.BasicPassword != "" && len(c.BasicPassword) < 12 {
		return fmt.Errorf("Basic authentication password must contain at least 12 bytes")
	}
	return nil
}

func (c Config) Address() string      { return net.JoinHostPort(c.Host, strconv.Itoa(c.Port)) }
func (c Config) DatabasePath() string { return filepath.Join(c.DataDir, "loggerbin.db") }
func (c Config) Expiration(seconds int64) (time.Duration, error) {
	if seconds == 0 {
		seconds = c.DefaultTTL
	}
	if seconds < 1 || seconds > c.MaxTTL {
		return 0, fmt.Errorf("expiration is outside allowed range")
	}
	return time.Duration(seconds) * time.Second, nil
}
