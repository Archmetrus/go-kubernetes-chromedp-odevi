package main

import (
	"fmt"
	"sort"
	"sync"
)

type sonuc struct{ sayi, kare int }

func main() {
	isler := make(chan int)
	sonuclar := make(chan sonuc)
	var wg sync.WaitGroup
	// Üç goroutine aynı iş kuyruğunu tüketir.
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := range isler {
				sonuclar <- sonuc{n, n * n}
			}
		}()
	}
	go func() {
		for n := 1; n <= 8; n++ {
			isler <- n
		}
		close(isler)
	}()
	go func() { wg.Wait(); close(sonuclar) }()
	var liste []sonuc
	for s := range sonuclar {
		liste = append(liste, s)
	}
	// İşlerin bitiş sırası belirsiz; sunum için sıralıyoruz.
	sort.Slice(liste, func(i, j int) bool { return liste[i].sayi < liste[j].sayi })
	for _, s := range liste {
		fmt.Printf("%d² = %d\n", s.sayi, s.kare)
	}
}
