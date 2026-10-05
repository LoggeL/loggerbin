package server

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/LoggeL/loggerbin/internal/config"
	"github.com/LoggeL/loggerbin/internal/store"
)

func setup(t *testing.T, edit func(*config.Config)) (*Server, *bytes.Buffer) {
	t.Helper()
	c := config.Defaults()
	c.DataDir = t.TempDir()
	if edit != nil {
		edit(&c)
	}
	db, err := store.Open(c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	var logs bytes.Buffer
	return New(c, db, slog.New(slog.NewTextHandler(&logs, nil)), "test"), &logs
}
func envelope() map[string]any {
	block, _ := aes.NewCipher(bytes.Repeat([]byte{42}, 32))
	gcm, _ := cipher.NewGCM(block)
	nonce := bytes.Repeat([]byte{17}, 12)
	return map[string]any{"version": 1, "nonce": base64.RawURLEncoding.EncodeToString(nonce), "ciphertext": base64.RawURLEncoding.EncodeToString(gcm.Seal(nil, nonce, []byte("SYNTHETIC PRIVATE TEXT"), []byte("loggerbin:v1"))), "expiration": 60}
}
func request(h http.Handler, method, path string, data any) *httptest.ResponseRecorder {
	var body bytes.Buffer
	if data != nil {
		json.NewEncoder(&body).Encode(data)
	}
	r := httptest.NewRequest(method, path, &body)
	r.Header.Set("Content-Type", "application/json")
	r.RemoteAddr = "198.51.100.4:1234"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func TestOpaqueRoundTripAndNoSensitiveLogs(t *testing.T) {
	s, logs := setup(t, nil)
	h := s.Handler()
	r := request(h, "POST", "/api/pastes", envelope())
	if r.Code != 201 {
		t.Fatalf("create: %d %s", r.Code, r.Body)
	}
	var created struct {
		ID string `json:"id"`
	}
	json.Unmarshal(r.Body.Bytes(), &created)
	for i := 0; i < 3; i++ {
		r = request(h, "GET", "/api/pastes/"+created.ID+"?key=DO_NOT_LOG_THIS", nil)
		if r.Code != 200 {
			t.Fatal("repeated read failed")
		}
	}
	for _, secret := range []string{created.ID, "DO_NOT_LOG_THIS", "SYNTHETIC PRIVATE TEXT", envelope()["ciphertext"].(string)} {
		if strings.Contains(logs.String(), secret) {
			t.Fatalf("sensitive data in logs: %q", secret)
		}
	}
	if strings.Contains(r.Body.String(), "SYNTHETIC PRIVATE TEXT") {
		t.Fatal("server returned plaintext")
	}
	for _, header := range []string{"Cache-Control", "Content-Security-Policy", "Referrer-Policy", "X-Frame-Options", "X-Content-Type-Options"} {
		if r.Header().Get(header) == "" {
			t.Errorf("missing %s", header)
		}
	}
	if s.HTTP().ReadHeaderTimeout == 0 || s.HTTP().ReadTimeout == 0 || s.HTTP().WriteTimeout == 0 || s.HTTP().IdleTimeout == 0 {
		t.Fatal("unbounded server timeout")
	}
}
func TestStrictInputAndExpiration(t *testing.T) {
	for _, seconds := range []int64{-1, 9223372037, -9223372036, 9223372036854775807, 2592001} {
		t.Run("expiration", func(t *testing.T) {
			s, _ := setup(t, nil)
			data := envelope()
			data["expiration"] = seconds
			r := request(s.Handler(), "POST", "/api/pastes", data)
			if r.Code != 400 {
				t.Fatalf("invalid expiry accepted: %d", r.Code)
			}
		})
	}
	for _, field := range []string{"content", "language", "key", "nonce", "ciphertext", "version"} {
		t.Run(field, func(t *testing.T) {
			s, _ := setup(t, nil)
			data := envelope()
			data[field] = "INVALID"
			r := request(s.Handler(), "POST", "/api/pastes", data)
			if r.Code != 400 {
				t.Fatalf("invalid field accepted: %d", r.Code)
			}
		})
	}
	s, _ := setup(t, nil)
	r := request(s.Handler(), "POST", "/api/pastes", map[string]any{"ciphertext": strings.Repeat("A", 2<<20)})
	if r.Code != 413 {
		t.Fatalf("body limit status %d", r.Code)
	}
	r = request(s.Handler(), "POST", "/internal/pastes/create", nil)
	if r.Code != 404 {
		t.Fatal("legacy plaintext route exists")
	}
}
func TestCreationLimitAndUntrustedHeaders(t *testing.T) {
	s, _ := setup(t, func(c *config.Config) { c.CreateBurst = 1; c.CreateRate = 0.0001 })
	h := s.Handler()
	for i, want := range []int{201, 429, 429} {
		body, _ := json.Marshal(envelope())
		r := httptest.NewRequest("POST", "/api/pastes", bytes.NewReader(body))
		r.RemoteAddr = "198.51.100.4:1234"
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Real-IP", []string{"10.0.0.1", "10.0.0.1", "10.0.0.2"}[i])
		r.Header.Set("X-Forwarded-For", []string{"10.0.0.1", "10.0.0.1", "10.0.0.2"}[i])
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("header spoof changed limiter: %d", w.Code)
		}
	}
}
func TestTrustedProxyBoundary(t *testing.T) {
	s, _ := setup(t, func(c *config.Config) { c.TrustedProxies = []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")} })
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "198.51.100.4:1234"
	r.Header.Set("X-Forwarded-For", "10.0.0.9")
	if s.clientIP(r) != "198.51.100.4" {
		t.Fatal("untrusted peer spoofed IP")
	}
	r.RemoteAddr = "127.0.0.1:1234"
	r.Header.Set("X-Forwarded-For", "10.0.0.9, 198.51.100.4")
	if s.clientIP(r) != "198.51.100.4" {
		t.Fatal("proxy did not select nearest untrusted hop")
	}
}
func TestOriginAndAuthenticationProtectAllPasteRoutes(t *testing.T) {
	s, _ := setup(t, nil)
	body, _ := json.Marshal(envelope())
	r := httptest.NewRequest("POST", "/api/pastes", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "https://attacker.invalid")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("cross-origin create accepted")
	}
	s, _ = setup(t, func(c *config.Config) { c.BasicUser = "test-user"; c.BasicPassword = "synthetic-test-password" })
	for _, path := range []string{"/", "/api/config", "/api/pastes/AAAAAAAAAAAAAAAAAAAAAA", "/static/app.js"} {
		if w := request(s.Handler(), "GET", path, nil); w.Code != 401 {
			t.Errorf("authentication bypass at %s", path)
		}
	}
	r = httptest.NewRequest("GET", "/api/config", nil)
	r.SetBasicAuth("test-user", "synthetic-test-password")
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal("valid credentials rejected")
	}
	if w := request(s.Handler(), "GET", "/healthz", nil); w.Code != 200 {
		t.Fatal("healthcheck requires credentials")
	}
}
func TestInvalidIDsAndStaticTraversal(t *testing.T) {
	s, _ := setup(t, nil)
	for _, path := range []string{"/api/pastes/" + strings.Repeat("A", 1000), "/api/pastes/short", "/static/%2e%2e%2fmain.go", "/static/README.md", "/p/%3Cscript%3E"} {
		w := request(s.Handler(), "GET", path, nil)
		if w.Code != 404 {
			t.Errorf("invalid path status %d at %s", w.Code, path)
		}
		if strings.Contains(w.Body.String(), "package main") {
			t.Fatal("static source disclosure")
		}
	}
}

func TestStorageQuotaHasExplicitAPIStatus(t *testing.T) {
	s, _ := setup(t, func(c *config.Config) { c.MaxPastes = 1 })
	h := s.Handler()
	if r := request(h, "POST", "/api/pastes", envelope()); r.Code != 201 {
		t.Fatal("first write failed")
	}
	if r := request(h, "POST", "/api/pastes", envelope()); r.Code != 507 {
		t.Fatalf("quota status %d", r.Code)
	}
}
