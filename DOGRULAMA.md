# Bu makinedeki doğrulama

- Go: `go1.27.1 linux/amd64`.
- chromedp: `v0.16.0` (go.mod/go.sum ile sabit).
- kubectl: `v1.37.0`; kind: `v0.33.0`.
- Go ve kubectl indirmelerinin SHA-256 değerleri doğrulandı.
- `go test -race ./...`: başarılı. API varsayılan ad, verilen ad, sağlık, 404 ve 405 davranışları denetlendi.
- `go vet ./...`: başarılı.
- İki Go giriş örneği ve worker pool çalıştırıldı; beklenen çıktılar alındı.
- chromedp giriş örneği gerçek Google Chrome ile başlık ve H1 okudu.
- chromedp orta örneği gerçek Chrome ile form doldurdu, API sonucunu doğruladı, `outputs/form-sonucu.png` oluşturdu.
- `CGO_ENABLED=0 go build -trimpath -o /tmp/odev-api ./01-go/orta/http-api`: başarılı.
- Shell betiklerinin `bash -n` kontrolü başarılı.
- kubeconform v0.8.0 ile `kubeconform -strict -summary 02-kubernetes/`: 5 kaynak geçerli, 0 hata, 0 atlanan kaynak. Bu şema kontrolüdür; kümede çalıştığını kanıtlamaz.

## Tamamlanamayan çalışma zamanı kontrolü

Docker komutu kurulu, ancak Docker servisi `inactive`; Docker soketi yok. `sudo -n true` yönetici parolası gerektirdiğini bildirdi. Bu nedenle Docker imajı oluşturma, kind kümesi açma ve Kubernetes üzerinde canlı çalıştırma doğrulanamadı.

Sistemde Docker servisi başlatılıp `docker info` erişimi sağlandıktan sonra proje kökünde `bash scripts/k8s-baslat.sh` çalıştırılmalıdır. Betik yalnızca `go-odev` adlı yerel kümeyi hedefler. Çalıştırma ve temizleme adımları README.md içindedir.

## Fish uyumluluğu

- `fish -n scripts/env.fish`: başarılı.
- Fish içinde ortam dosyası yüklenerek Go, kubectl ve kind sürümleri kontrol edildi; Go giriş örneği çalıştı.
- Tekrar source edildiğinde PATH kaydının çoğalmadığı ve özel CHROME_PATH değerinin korunduğu doğrulandı.
- README komutları fish sözdizimine uyarlandı; Bash yardımcı betiklerinin sözdizimi kontrolü başarılı.
