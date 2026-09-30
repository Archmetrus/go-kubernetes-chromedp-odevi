package main

import (
	"example.com/odev/internal/browser"
	"example.com/odev/internal/webapp"
	"fmt"
	"github.com/chromedp/chromedp"
	"log"
	"net/http/httptest"
)

func main() {
	// Dış sitelere ihtiyaç duymayan geçici bir yerel sunucu.
	srv := httptest.NewServer(webapp.Handler("Merhaba"))
	defer srv.Close()
	ctx, cancel := browser.New()
	defer cancel()
	var baslik, metin string
	if err := chromedp.Run(ctx, chromedp.Navigate(srv.URL), chromedp.Title(&baslik), chromedp.Text("h1", &metin, chromedp.ByQuery)); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Başlık: %s\nH1: %s\n", baslik, metin)
}
