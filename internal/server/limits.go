package server

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/LoggeL/loggerbin/internal/config"
	"golang.org/x/time/rate"
)

type visitor struct {
	limiter *rate.Limiter
	seen    time.Time
}
type limits struct {
	mu          sync.Mutex
	visitors    map[string]*visitor
	global      *rate.Limiter
	lastCleanup time.Time
	config      config.Config
}

func newLimits(c config.Config) *limits {
	return &limits{visitors: make(map[string]*visitor), global: rate.NewLimiter(100, 200), lastCleanup: time.Now(), config: c}
}
func (l *limits) allow(ip string, create bool) bool {
	if !l.global.Allow() {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if now.Sub(l.lastCleanup) > time.Minute {
		for key, v := range l.visitors {
			if now.Sub(v.seen) > 5*time.Minute {
				delete(l.visitors, key)
			}
		}
		l.lastCleanup = now
	}
	key := "read:" + ip
	r, b := rate.Limit(10), 40
	if create {
		key = "create:" + ip
		r, b = rate.Limit(l.config.CreateRate), l.config.CreateBurst
	}
	v := l.visitors[key]
	if v == nil {
		if len(l.visitors) >= 10000 {
			return false
		}
		v = &visitor{limiter: rate.NewLimiter(r, b)}
		l.visitors[key] = v
	}
	v.seen = now
	return v.limiter.AllowN(now, 1)
}
func (s *Server) trusted(ip netip.Addr) bool {
	for _, p := range s.config.TrustedProxies {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}
func (s *Server) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return "unknown"
	}
	peer = peer.Unmap()
	if !s.trusted(peer) {
		return peer.String()
	}
	header := r.Header.Get("X-Forwarded-For")
	if len(header) > 4096 {
		return peer.String()
	}
	parts := strings.Split(header, ",")
	if len(parts) > 32 {
		return peer.String()
	}
	current := peer
	for i := len(parts) - 1; i >= 0; i-- {
		if !s.trusted(current) {
			break
		}
		ip, err := netip.ParseAddr(strings.TrimSpace(parts[i]))
		if err != nil {
			return peer.String()
		}
		current = ip.Unmap()
	}
	return current.String()
}
