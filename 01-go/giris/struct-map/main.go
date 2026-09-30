package main

import "fmt"

type Ogrenci struct {
	Ad   string
	Puan int
}

// Pointer alıcısı, özgün öğrencinin puanını değiştirir.
func (o *Ogrenci) Bonus(ek int) { o.Puan += ek }
func main() {
	ogrenci := Ogrenci{Ad: "Deniz", Puan: 75}
	ogrenci.Bonus(10)
	dersler := map[string]int{"Go": ogrenci.Puan, "Kubernetes": 80}
	fmt.Printf("%s: Go=%d, Kubernetes=%d\n", ogrenci.Ad, dersler["Go"], dersler["Kubernetes"])
	puan, varMi := dersler["Python"]
	fmt.Printf("Python kaydı var mı? %t (değer=%d)\n", varMi, puan)
}
