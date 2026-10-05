package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/LoggeL/loggerbin/internal/config"
	"github.com/LoggeL/loggerbin/internal/store"
	"github.com/LoggeL/loggerbin/internal/web"
)

type Server struct {
	config  config.Config
	store   *store.Store
	logger  *slog.Logger
	limits  *limits
	version string
}

func New(c config.Config, db *store.Store, log *slog.Logger, version string) *Server {
	return &Server{config: c, store: db, logger: log, limits: newLimits(c), version: version}
}
func (s *Server) HTTP() *http.Server {
	return &http.Server{Addr: s.config.Address(), Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
}
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /api/config", s.publicConfig)
	mux.HandleFunc("POST /api/pastes", s.create)
	mux.HandleFunc("GET /api/pastes/{id}", s.get)
	mux.HandleFunc("GET /{$}", s.page)
	mux.HandleFunc("GET /p/{id}", s.page)
	mux.HandleFunc("GET /static/{asset}", s.asset)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		securityHeaders(w)
		rec := &response{ResponseWriter: w, status: 200}
		defer func() {
			if recover() != nil {
				s.logger.Error("request panic recovered")
				if !rec.wrote {
					fail(rec, 500, "internal_error", "Request failed")
				}
			}
			// Route patterns never contain paste IDs, fragments, queries or user-supplied headers.
			s.logger.Info("request", "method", r.Method, "route", r.Pattern, "status", rec.status)
		}()
		if r.URL.Path != "/healthz" && !s.authenticate(rec, r) {
			return
		}
		if r.Method == http.MethodPost && !sameOrigin(r) {
			fail(rec, 403, "forbidden", "Cross-origin creation is disabled")
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") && !s.limits.allow(s.clientIP(r), r.Method == http.MethodPost) {
			rec.Header().Set("Retry-After", "5")
			fail(rec, 429, "rate_limited", "Too many requests. Try again shortly.")
			return
		}
		mux.ServeHTTP(rec, r)
	})
}
func securityHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Cache-Control", "no-store")
	h.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'; object-src 'none'")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
	h.Set("Cross-Origin-Opener-Policy", "same-origin")
	h.Set("Cross-Origin-Resource-Policy", "same-origin")
}

type response struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (r *response) WriteHeader(code int) {
	if r.wrote {
		return
	}
	r.status = code
	r.wrote = true
	r.ResponseWriter.WriteHeader(code)
}
func (r *response) Write(b []byte) (int, error) {
	if !r.wrote {
		r.WriteHeader(200)
	}
	return r.ResponseWriter.Write(b)
}
func respond(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
func fail(w http.ResponseWriter, status int, code, message string) {
	respond(w, status, map[string]string{"code": code, "message": message})
}
func sameOrigin(r *http.Request) bool {
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host == r.Host && u.User == nil && u.Path == "" && u.RawQuery == "" && u.Fragment == ""
}
func (s *Server) authenticate(w http.ResponseWriter, r *http.Request) bool {
	if s.config.BasicUser == "" {
		return true
	}
	u, p, ok := r.BasicAuth()
	a, b := sha256.Sum256([]byte(u)), sha256.Sum256([]byte(s.config.BasicUser))
	c, d := sha256.Sum256([]byte(p)), sha256.Sum256([]byte(s.config.BasicPassword))
	valid := subtle.ConstantTimeCompare(a[:], b[:]) & subtle.ConstantTimeCompare(c[:], d[:])
	if !ok || valid != 1 {
		w.Header().Set("WWW-Authenticate", `Basic realm="Loggerbin", charset="UTF-8"`)
		fail(w, 401, "unauthorized", "Authentication required")
		return false
	}
	return true
}
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if s.store.Check() != nil {
		fail(w, 503, "unavailable", "Storage unavailable")
		return
	}
	respond(w, 200, map[string]string{"status": "ok", "version": s.version})
}
func (s *Server) publicConfig(w http.ResponseWriter, r *http.Request) {
	respond(w, 200, map[string]any{"name": "Loggerbin", "version": s.version, "max_paste_bytes": s.config.MaxPasteBytes, "default_ttl": s.config.DefaultTTL, "max_ttl": s.config.MaxTTL})
}
func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	if id := r.PathValue("id"); id != "" && !store.ValidID(id) {
		http.NotFound(w, r)
		return
	}
	s.serveAsset(w, "index.html", "text/html; charset=utf-8")
}
func (s *Server) asset(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("asset")
	types := map[string]string{"app.js": "text/javascript; charset=utf-8", "crypto.js": "text/javascript; charset=utf-8", "style.css": "text/css; charset=utf-8", "logo.svg": "image/svg+xml"}
	kind, ok := types[name]
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.serveAsset(w, name, kind)
}
func (s *Server) serveAsset(w http.ResponseWriter, name, kind string) {
	b, err := web.Files.ReadFile("static/" + name)
	if err != nil {
		fail(w, 500, "internal_error", "Asset unavailable")
		return
	}
	w.Header().Set("Content-Type", kind)
	w.Write(b)
}
func decodeBase64(value string, min, max int) bool {
	if len(value) > base64.RawURLEncoding.EncodedLen(max) {
		return false
	}
	b, err := base64.RawURLEncoding.Strict().DecodeString(value)
	return err == nil && len(b) >= min && len(b) <= max && base64.RawURLEncoding.EncodeToString(b) == value
}
func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	kind, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || kind != "application/json" {
		fail(w, 415, "unsupported_type", "Use application/json")
		return
	}
	bodyMax := int64(base64.RawURLEncoding.EncodedLen(s.config.MaxPasteBytes+16) + 1024)
	r.Body = http.MaxBytesReader(w, r.Body, bodyMax)
	var req struct {
		store.Envelope
		Expiration int64 `json:"expiration"`
	}
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(&req); err != nil {
		var size *http.MaxBytesError
		if errors.As(err, &size) {
			fail(w, 413, "too_large", "Request exceeds the size limit")
		} else {
			fail(w, 400, "invalid_request", "Invalid encrypted paste")
		}
		return
	}
	if err := d.Decode(new(any)); err != io.EOF {
		fail(w, 400, "invalid_request", "Send one JSON object")
		return
	}
	if req.Version != 1 || !decodeBase64(req.Nonce, 12, 12) || !decodeBase64(req.Ciphertext, 17, s.config.MaxPasteBytes+16) {
		fail(w, 400, "invalid_request", "Invalid encrypted paste")
		return
	}
	ttl, err := s.config.Expiration(req.Expiration)
	if err != nil {
		fail(w, 400, "invalid_expiration", "Expiration is outside the allowed range")
		return
	}
	p, err := s.store.Create(req.Envelope, ttl)
	if errors.Is(err, store.ErrQuota) {
		fail(w, 507, "storage_full", "This instance has reached its storage limit")
		return
	}
	if err != nil {
		s.logger.Error("paste storage failed")
		fail(w, 500, "internal_error", "Could not store paste")
		return
	}
	w.Header().Set("Location", "/p/"+p.ID)
	respond(w, 201, map[string]any{"id": p.ID, "created_at": p.CreatedAt, "expires_at": p.ExpiresAt})
}
func (s *Server) get(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.Get(r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		fail(w, 404, "not_found", "Paste does not exist or has expired")
		return
	}
	if err != nil {
		s.logger.Error("paste read failed")
		fail(w, 500, "internal_error", "Could not read paste")
		return
	}
	respond(w, 200, p)
}
