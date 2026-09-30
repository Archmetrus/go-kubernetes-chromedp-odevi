# Go, Kubernetes ve chromedp ödevi

Girişten orta seviyeye ilerleyen, Türkçe yorumlu küçük örnekler. Komutları bu klasörde çalıştırın. Go örneklerini okuyup değiştirerek öğrenmeniz için her bölümde ek çalışma vardır.

## Başlangıç (fish)

```fish
cd /home/ykk/PROJE/go-kubernetes-chromedp-odevi
source scripts/env.fish
go version
kubectl version --client
kind version
go mod download
bash scripts/kontrol.sh
```

Go, kubectl ve kind bu bilgisayarda `~/.local/share/odev-tools` dizinindedir. `source scripts/env.fish` yalnızca açık terminalin PATH değerini ayarlar; her yeni terminalde tekrar çalıştırın. chromedp sürümü `go.mod`, bağımlılık doğrulama kayıtları `go.sum` dosyasındadır. Kurulu Google Chrome kullanılır; farklı tarayıcı için `set -gx CHROME_PATH /tarayicinin/tam/yolu` ayarlayın. Otomasyon geçici bir profil kullanır.

Aynı araçları başka bir Linux x86_64 ortamında kurmak için `bash scripts/kurulum.sh` kullanılabilir. Bu betik Go, kubectl, kind ve Go modüllerini kurar; Chrome ve çalışan Docker ayrıca gereklidir.

Komutlar fish içindir. `.sh` betiklerini fish terminalinden `bash scripts/betik-adi.sh` biçiminde çalıştırın; bu betikler kendi Bash ortamını açar. Fish oturumuna ortam ayarlarını yüklemek için `source scripts/env.fish` kullanın.

## 1. Go Tour konuları

| Seviye | Örnek | Konu | Çalıştırma |
|---|---|---|---|
| Giriş | Not ortalaması | Slice, döngü, koşul, fonksiyon, hata dönüşü | `go run ./01-go/giris/temeller` |
| Giriş | Öğrenci kaydı | Struct, pointer alıcısı, map, bulunma kontrolü | `go run ./01-go/giris/struct-map` |
| Orta | İş kuyruğu | Goroutine, channel, WaitGroup, sıralama | `go run ./01-go/orta/worker-pool` |
| Orta | HTTP API | Handler, JSON, ortam değişkeni, düzgün kapanma | `go run ./01-go/orta/http-api` |

İlk örnek `83.33` ortalama üretir. İkinci örnekte Deniz'in Go notu `85` olur. İş kuyruğu 1–8 sayılarının karelerini hesaplar; üç çalışan işleri paylaşır. Kanalları gönderen taraf kapatır; sonuç kanalı bütün çalışanlar tamamlanınca kapanır.

HTTP API çalışırken başka terminalde:

```fish
curl 'http://127.0.0.1:8080/api/selam?ad=Deniz'
curl http://127.0.0.1:8080/healthz
```

İlk cevap `{"ad":"Deniz","mesaj":"Merhaba"}`, sağlık cevabı `ok` olur. Tarayıcıda `http://127.0.0.1:8080` adresinde form da vardır. Sunucuyu Ctrl+C ile durdurun. `env MESAJ='Selam' go run ./01-go/orta/http-api` mesajı değiştirir.

**Ek çalışmalar:** Notlara 0–100 aralığı kontrolü ekleyin; map'e bir ders ekleyin; çalışan sayısını değiştirin; API'ye `/api/topla?a=3&b=4` ekleyip geçersiz sayıları 400 koduyla reddedin.

## 2. Kubernetes

Container, uygulamanın çalıştırılabilir paketidir. Pod bu container'ı çalıştırır. Deployment istenen Pod sayısını korur. Service Pod'lara sabit erişim sağlar. ConfigMap uygulama ayarlarını taşır.

Docker kurulu ve servisi erişilebilir olmalıdır. `docker info` başarısızsa önce sisteminizde Docker servisini başlatın ve kullanıcı erişimini düzeltin. Proje betiği bu kontrol geçmezse durur.

```fish
source scripts/env.fish
bash scripts/k8s-baslat.sh
set -gx KUBECONFIG "$PWD/.kubeconfig"
kubectl --context kind-go-odev -n odev get pods,deployments,services
```

Betik ayrı `go-odev` kind kümesi oluşturur, Go API imajını derleyip kümeye yükler. `.kubeconfig` kullanıldığı için varsayılan küme ayarlarınız değiştirilmez. İmaj yerel olduğu için `imagePullPolicy: Never` kullanılır; başka kümelerde önce imajı yüklemelisiniz.

### Giriş: tek Pod

`02-kubernetes/giris/pod.yaml` tek uygulama örneği çalıştırır.

```fish
kubectl --context kind-go-odev -n odev port-forward pod/merhaba-go 8081:8080
```

Başka terminalde `curl 'http://127.0.0.1:8081/api/selam?ad=Deniz'` çalıştırın. Port yönlendirmesini Ctrl+C ile kapatın.

### Orta: iki replika, Service, ConfigMap ve sağlık kontrolleri

`02-kubernetes/orta/uygulama.yaml` iki replika ve bir Service oluşturur. Readiness trafiğe hazır olmayı, liveness uygulamanın canlılığını kontrol eder. CPU/bellek sınırları kaynak kullanımını sınırlar.

```fish
kubectl --context kind-go-odev -n odev port-forward service/go-api 8082:80
```

Başka terminalde `curl 'http://127.0.0.1:8082/api/selam?ad=Deniz'` çalıştırın. Mesaj `Kubernetes'ten merhaba` olur. Port-forward erişimi gösterir; yük dağıtımını ölçmek için tasarlanmamıştır.

```fish
# Replika sayısını artırın ve izleyin.
kubectl --context kind-go-odev -n odev scale deployment/go-api --replicas=3
kubectl --context kind-go-odev -n odev get pods
# ConfigMap dosyasındaki MESAJ'ı değiştirip yeniden uyguladıktan sonra:
kubectl --context kind-go-odev apply -f 02-kubernetes/orta/uygulama.yaml
kubectl --context kind-go-odev -n odev rollout restart deployment/go-api
kubectl --context kind-go-odev -n odev rollout status deployment/go-api
```

ConfigMap ortam değişkeni olarak okunduğundan mevcut Pod'lara otomatik yansımaz; yeniden başlatma gerekir. YAML'i tekrar uygulamak replika sayısını dosyadaki 2 değerine getirir.

**Ek çalışmalar:** Deployment Pod'larından birini silip yenisinin oluşmasını izleyin; tek Pod ile farkını açıklayın; mesajı değiştirin; readiness adresini bilerek hatalı yapıp `kubectl describe pod` çıktısını inceleyin ve düzeltin.

Yalnızca bu ödevin yerel kümesini kaldırmak için:

```fish
kind delete cluster --name go-odev
```

## 3. chromedp

Örnekler kendi geçici yerel HTTP sunucularını açar. Harici siteye veya ayrıca API başlatılmasına ihtiyaç yoktur.

```fish
go run ./03-chromedp/giris
go run ./03-chromedp/orta
```

Giriş örneği sayfa başlığını ve H1 metnini okur: `Go Ödev Atölyesi`.
Orta örnek forma `Deniz` yazar, butona tıklar, asenkron cevabı bekler, `Merhaba, Deniz!` sonucunu doğrular ve `outputs/form-sonucu.png` üretir. Sabit sleep yerine DOM koşulu beklenir. İşlem 30 saniyeyle sınırlandırılır; bitince tarayıcı ve sunucu kapanır.

**Ek çalışmalar:** Girilen adı değiştirip beklenen sonucu güncelleyin; ikinci alan ekleyin; ekran görüntüsünü sadece form alanından alın. İlgili eylemler: `Navigate`, `Title`, `Text`, `SendKeys`, `Click`, `Poll`, `FullScreenshot`.

## Kontrol ve öğrenme sırası

1. Go giriş örneklerini çalıştırın ve değerleri değiştirin.
2. Worker pool'da kanalların neden kapatıldığını açıklayın.
3. HTTP API'yi curl ile deneyin; `internal/webapp/app_test.go` testlerini okuyun.
4. chromedp ile aynı API'nin kullanıcı arayüzünü otomatik deneyin.
5. API'yi Kubernetes'e taşıyıp replika ve ConfigMap davranışını gözlemleyin.

`bash scripts/kontrol.sh` Go testlerini (race detector dahil), vet kontrolünü ve kısa örnekleri çalıştırır. Kubernetes kümesi ayrı betikle kurulur. Makinede yapılan kontrollerin sonucu `../DOGRULAMA.md` dosyasındadır.

## Resmî kaynaklar

- [A Tour of Go](https://go.dev/tour/welcome/1): dil konuları ve alıştırmalar.
- [chromedp kaynak kodu ve belgeleri](https://github.com/chromedp/chromedp): Go ile tarayıcı otomasyonu.
- [Kubernetes araç kurulumu](https://kubernetes.io/docs/tasks/tools/).
- [kind başlangıç rehberi](https://kind.sigs.k8s.io/docs/user/quick-start/).
