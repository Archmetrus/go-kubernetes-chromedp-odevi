package security

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"k8s-chromedp-synthetics/pkg/types"
)

func TestSSRFGuard(funcTest *testing.T) {
	cfg := types.SecurityConfig{
		EnableSSRFGuard: true,
	}
	guard := NewSSRFGuard(cfg)

	// Tests for forbidden IPs & internal hosts
	forbidden := []string{
		"http://127.0.0.1/admin",
		"http://localhost:8080",
		"http://169.254.169.254/latest/meta-data/",
		"http://10.0.0.1/api",
		"http://192.168.1.1",
	}

	for _, u := range forbidden {
		err := guard.ValidateURL(u)
		if err == nil {
			funcTest.Errorf("expected SSRF error for URL %s, but got nil", u)
		}
	}

	// Test for valid public domain (if network available)
	validURL := "https://example.com"
	err := guard.ValidateURL(validURL)
	if err != nil {
		funcTest.Logf("Notice: DNS resolution check for %s returned: %v", validURL, err)
	}
}

func TestAuthMiddleware(t *testing.T) {
	cfg := types.SecurityConfig{
		APIKey: "secret-key-123",
	}
	sec := NewSecurityMiddleware(cfg)

	handler := sec.WrapMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}), true)

	// Test 1: No Key -> 401
	req1 := httptest.NewRequest("GET", "/api/test", nil)
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized, got %d", rec1.Code)
	}

	// Test 2: Valid Key -> 200
	req2 := httptest.NewRequest("GET", "/api/test", nil)
	req2.Header.Set("X-API-Key", "secret-key-123")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rec2.Code)
	}
}
