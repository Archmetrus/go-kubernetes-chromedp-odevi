package webapp

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestSelam(t *testing.T) {
	for _, tc := range []struct{ path, ad string }{{"/api/selam", "Dünya"}, {"/api/selam?ad=Deniz", "Deniz"}} {
		w := httptest.NewRecorder()
		Handler("Merhaba").ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
		var got map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if w.Code != 200 || got["ad"] != tc.ad || got["mesaj"] != "Merhaba" {
			t.Fatalf("beklenmeyen cevap: %d %v", w.Code, got)
		}
	}
}
func TestRoutes(t *testing.T) {
	for _, tc := range []struct {
		method, path string
		code         int
	}{{"GET", "/healthz", 200}, {"POST", "/api/selam", 405}, {"GET", "/olmayan", 404}} {
		w := httptest.NewRecorder()
		Handler("Merhaba").ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != tc.code {
			t.Fatalf("%s %s: %d", tc.method, tc.path, w.Code)
		}
	}
}
