package config

import (
	"math"
	"testing"
	"time"
)

func TestExpirationRejectsOverflowAndNegative(t *testing.T) {
	c := Defaults()
	for _, seconds := range []int64{-1, -9223372036, 9223372037, math.MaxInt64, c.MaxTTL + 1} {
		if _, err := c.Expiration(seconds); err == nil {
			t.Errorf("accepted invalid expiration %d", seconds)
		}
	}
	for _, seconds := range []int64{1, c.MaxTTL} {
		got, err := c.Expiration(seconds)
		if err != nil || got != time.Duration(seconds)*time.Second {
			t.Errorf("valid expiration failed: %d", seconds)
		}
	}
	got, err := c.Expiration(0)
	if err != nil || got != time.Duration(c.DefaultTTL)*time.Second {
		t.Fatal("default expiration failed")
	}
}
func TestEnvironmentNamesAndValidation(t *testing.T) {
	t.Setenv("LOGGERBIN_MAX_PASTE_BYTES", "1234")
	t.Setenv("LOGGERBIN_DEFAULT_TTL_SECONDS", "60")
	t.Setenv("LOGGERBIN_TRUSTED_PROXIES", "127.0.0.1/32,::1/128")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.MaxPasteBytes != 1234 || c.DefaultTTL != 60 || len(c.TrustedProxies) != 2 {
		t.Fatal("environment mapping failed")
	}
	t.Setenv("LOGGERBIN_CREATE_RATE", "NaN")
	if _, err := Load(); err == nil {
		t.Fatal("accepted NaN limiter rate")
	}
}
