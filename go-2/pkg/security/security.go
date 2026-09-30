package security

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"golang.org/x/time/rate"
	"k8s-chromedp-synthetics/pkg/types"
)

// SSRFGuard provides IP and domain checks to prevent Server-Side Request Forgery
type SSRFGuard struct {
	AllowLocalhost bool
	AllowedHosts   map[string]bool
	BlockedHosts   map[string]bool
}

// NewSSRFGuard initializes SSRF Protection with default RFC 1918 and cloud metadata blocks
func NewSSRFGuard(cfg types.SecurityConfig) *SSRFGuard {
	allowed := make(map[string]bool)
	for _, h := range cfg.AllowedHosts {
		allowed[strings.ToLower(h)] = true
	}
	blocked := make(map[string]bool)
	for _, h := range cfg.BlockedHosts {
		blocked[strings.ToLower(h)] = true
	}

	return &SSRFGuard{
		AllowLocalhost: false,
		AllowedHosts:   allowed,
		BlockedHosts:   blocked,
	}
}

// ValidateURL checks if target URL is safe to navigate
func (s *SSRFGuard) ValidateURL(targetURL string) error {
	parsed, err := url.Parse(targetURL)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("forbidden protocol scheme: %s (only http and https are allowed)", scheme)
	}

	host := parsed.Hostname()
	if host == "" {
		return fmt.Errorf("empty target hostname")
	}

	hostLower := strings.ToLower(host)

	// Check explicit blocked list
	if s.BlockedHosts[hostLower] {
		return fmt.Errorf("access to host %s is explicitly blocked", host)
	}

	// Check explicit allowed list
	if s.AllowedHosts[hostLower] {
		return nil
	}

	// Block localhost & loopback domain strings
	if !s.AllowLocalhost {
		if hostLower == "localhost" || strings.HasSuffix(hostLower, ".local") || strings.HasSuffix(hostLower, ".internal") {
			return fmt.Errorf("access to internal hostname %s is forbidden (SSRF protection)", host)
		}
	}

	// Resolve IP addresses
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("failed to resolve DNS for host %s: %w", host, err)
	}

	for _, ip := range ips {
		if err := s.ValidateIP(ip); err != nil {
			return fmt.Errorf("SSRF violation for host %s (%s): %w", host, ip.String(), err)
		}
	}

	return nil
}

// ValidateIP verifies if an IP address belongs to private/banned CIDRs
func (s *SSRFGuard) ValidateIP(ip net.IP) error {
	if s.AllowLocalhost && ip.IsLoopback() {
		return nil
	}

	// Loopback
	if ip.IsLoopback() {
		return fmt.Errorf("loopback IP address forbidden: %s", ip)
	}

	// Link-local / Cloud Metadata (169.254.169.254)
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return fmt.Errorf("link-local / cloud metadata IP address forbidden: %s", ip)
	}

	// Private IP Ranges (RFC 1918 & IPv6 Unique Local)
	privateCIDRs := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"169.254.0.0/16", // AWS/GCP Metadata
		"127.0.0.0/8",   // IPv4 Loopback
		"::1/128",       // IPv6 Loopback
		"fc00::/7",      // IPv6 Private
		"fe80::/10",     // IPv6 Link-local
	}

	for _, cidr := range privateCIDRs {
		_, block, err := net.ParseCIDR(cidr)
		if err == nil && block.Contains(ip) {
			return fmt.Errorf("private/internal network IP forbidden: %s (matches %s)", ip, cidr)
		}
	}

	return nil
}

// RateLimiterManager manages per-IP rate limiters
type RateLimiterManager struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	r        rate.Limit
	b        int
}

func NewRateLimiterManager(requestsPerSec float64, burst int) *RateLimiterManager {
	if requestsPerSec <= 0 {
		requestsPerSec = 10.0
	}
	if burst <= 0 {
		burst = 20
	}
	return &RateLimiterManager{
		limiters: make(map[string]*rate.Limiter),
		r:        rate.Limit(requestsPerSec),
		b:        burst,
	}
}

func (m *RateLimiterManager) GetLimiter(ip string) *rate.Limiter {
	m.mu.Lock()
	defer m.mu.Unlock()

	limiter, exists := m.limiters[ip]
	if !exists {
		limiter = rate.NewLimiter(m.r, m.b)
		m.limiters[ip] = limiter
	}
	return limiter
}

// SecurityMiddleware wraps HTTP handlers with Auth, Rate Limiting, and Security Headers
type SecurityMiddleware struct {
	Config      types.SecurityConfig
	SSRF        *SSRFGuard
	RateLimiter *RateLimiterManager
}

func NewSecurityMiddleware(cfg types.SecurityConfig) *SecurityMiddleware {
	return &SecurityMiddleware{
		Config:      cfg,
		SSRF:        NewSSRFGuard(cfg),
		RateLimiter: NewRateLimiterManager(cfg.RateLimitRequests, cfg.RateLimitBurst),
	}
}

// WrapMiddleware applies security headers, rate limiting, and optional API key auth
func (s *SecurityMiddleware) WrapMiddleware(next http.Handler, requireAuth bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Security Headers
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// 2. Rate Limiting
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}
		limiter := s.RateLimiter.GetLimiter(ip)
		if !limiter.Allow() {
			http.Error(w, `{"error":"Rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}

		// 3. API Key Auth Check
		if requireAuth && s.Config.APIKey != "" {
			clientKey := r.Header.Get("X-API-Key")
			if clientKey == "" {
				authHeader := r.Header.Get("Authorization")
				if strings.HasPrefix(authHeader, "Bearer ") {
					clientKey = strings.TrimPrefix(authHeader, "Bearer ")
				}
			}

			if clientKey != s.Config.APIKey {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"Unauthorized: Invalid or missing API Key"}`))
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}
