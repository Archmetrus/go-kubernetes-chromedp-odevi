package main

import "fmt"

// Değişken, slice, döngü, koşul ve çoklu dönüş değeri.
func ortalama(notlar []int) (float64, error) {
	if len(notlar) == 0 {
		return 0, fmt.Errorf("not listesi boş olamaz")
	}
	toplam := 0
	for _, not := range notlar {
		toplam += not
	}
	return float64(toplam) / float64(len(notlar)), nil
}
func main() {
	notlar := []int{70, 85, 95}
	sonuc, err := ortalama(notlar)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("Notlar: %v, ortalama: %.2f, geçti: %t\n", notlar, sonuc, sonuc >= 60)
}
