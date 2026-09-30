# Go, Kubernetes ve chromedp çalışmaları

## Görev Atölyesi

Yeni hesaplı sürüm: [giriş, PostgreSQL, canlı güncelleme, arşiv, JSON içe aktarma ve yayın hazırlığı](gorev-atolyesi/docs/CANLIYA-HAZIRLIK.md). Aşağıdaki hızlı komutlar kişisel geliştirme modudur.

Uygulamanın tüm kaynakları, Go modülü, betikleri, dağıtım dosyaları ve belgeleri **[gorev-atolyesi](gorev-atolyesi/README.md)** klasöründe toplandı.

```fish
cd ~/PROJE/go-kubernetes-chromedp-odevi/gorev-atolyesi
and source scripts/env.fish
and sudo systemctl start docker
and docker info
and bash scripts/proje-k8s.sh
and bash scripts/proje-ac.sh
```

Tarayıcı: **http://127.0.0.1:8090**. Son komut çalışırken terminal açık kalır. Bu akış güncel kodu derleyip Kubernetes'e dağıtır. Kapatmak için aynı klasörde `bash scripts/proje-kapat.sh` çalıştırın.

Docker olmadan yerelde açmak için:

```fish
cd ~/PROJE/go-kubernetes-chromedp-odevi/gorev-atolyesi
and env ADDR=127.0.0.1:8091 bash scripts/proje-baslat.sh
```

Yerel adres **http://127.0.0.1:8091**; kapatmak için Ctrl+C. Yerel ve Kubernetes görev veritabanları ayrıdır.

[Sıralı açma, güncelleme, test ve kapatma komutları](gorev-atolyesi/README.md) · [Go ve Kubernetes için yaptıklarımızın ayrıntılı raporu](gorev-atolyesi/docs/GO-VE-KUBERNETES.md)

## Giriş ve orta seviye alıştırmalar

`01-go`, `02-kubernetes`, `03-chromedp` ve bu dizinin `scripts` klasörü ilk alıştırmalara aittir.

[Alıştırma rehberi](docs/ALISTIRMALAR.md) · [Alıştırmaların doğrulama kaydı](DOGRULAMA.md)
