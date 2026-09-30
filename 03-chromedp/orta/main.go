package main

import (
	"example.com/odev/internal/browser"
	"example.com/odev/internal/webapp"
	"fmt"
	"github.com/chromedp/chromedp"
	"log"
	"net/http/httptest"
	"os"
)

func main() {
	srv := httptest.NewServer(webapp.Handler("Merhaba"))
	defer srv.Close()
	ctx, cancel := browser.New()
	defer cancel()
	var sonuc string
	var png []byte
	err := chromedp.Run(ctx,
		chromedp.Navigate(srv.URL),
		chromedp.WaitVisible("#ad", chromedp.ByQuery),
		chromedp.SendKeys("#ad", "Deniz", chromedp.ByQuery),
		chromedp.Click("#gonder", chromedp.ByQuery),
		// Sabit bekleme yerine asenkron API cevabının DOM'a yansımasını bekle.
		chromedp.Poll(`document.querySelector('#sonuc').textContent === 'Merhaba, Deniz!'`, nil),
		chromedp.Text("#sonuc", &sonuc, chromedp.ByQuery),
		chromedp.FullScreenshot(&png, 90))
	if err != nil {
		log.Fatal(err)
	}
	if sonuc != "Merhaba, Deniz!" {
		log.Fatalf("Beklenmeyen sonuç: %q", sonuc)
	}
	if err = os.MkdirAll("outputs", 0755); err != nil {
		log.Fatal(err)
	}
	if err = os.WriteFile("outputs/form-sonucu.png", png, 0644); err != nil {
		log.Fatal(err)
	}
	fmt.Println(sonuc)
	fmt.Println("Ekran görüntüsü: outputs/form-sonucu.png")
}
