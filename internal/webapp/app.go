package webapp

import (
	"encoding/json"
	"net/http"
)

// Handler hem yerel sunucuda hem Kubernetes örneğinde kullanılır.
func Handler(mesaj string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	mux.HandleFunc("GET /api/selam", func(w http.ResponseWriter, r *http.Request) {
		ad := r.URL.Query().Get("ad")
		if ad == "" {
			ad = "Dünya"
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(map[string]string{"mesaj": mesaj, "ad": ad})
	})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(page))
	})
	return mux
}

const page = `<!doctype html><html lang="tr"><meta charset="utf-8"><title>Go Ödev Atölyesi</title>
<body><h1>Go Ödev Atölyesi</h1><p>Yerel chromedp deneme sayfası</p>
<form id="form"><label>Adınız <input id="ad" required></label><button id="gonder">Selamla</button></form>
<p id="sonuc" role="status"></p>
<script>document.querySelector('#form').addEventListener('submit',async e=>{
e.preventDefault();const ad=document.querySelector('#ad').value;
const r=await fetch('/api/selam?ad='+encodeURIComponent(ad));const d=await r.json();
document.querySelector('#sonuc').textContent=d.mesaj+', '+d.ad+'!';});</script></body></html>`
