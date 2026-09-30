# Go ve Kubernetes için neler yaptık?

> Bu rapor ilk kişisel sürümün geliştirme aşamasını anlatır. Sonraki hesaplı sürümde PostgreSQL, oturumlar, kullanıcı sahipliği, migration, arşiv/çöp kutusu, JSON içe aktarma ve otomatik yenileme eklendi. Güncel kurulum ve kapsam için [Canlıya hazırlık](CANLIYA-HAZIRLIK.md) rehberini kullanın; aşağıdaki “henüz yok” ifadeleri ilk sürüme aittir.

Bu belge, projedeki kaynak kod ve dağıtım dosyalarına göre yapılan çalışmaları açıklar. Önce giriş ve orta seviye alıştırmalar hazırlandı; ardından bu konular SQLite kullanan bir görev takip uygulamasında birleştirildi. Çalıştırma sırası için [uygulama README'sini](../README.md) kullanın.

## 1. Çalışmanın iki aşaması

| Aşama | Konum | Amaç |
|---|---|---|
| İlk alıştırmalar | Ana dizindeki `01-go`, `02-kubernetes`, `03-chromedp` | Dil, HTTP, eşzamanlılık, container ve tarayıcı otomasyonunu küçük örneklerle öğrenmek |
| Görev Atölyesi | `gorev-atolyesi/` | Aynı konuları kalıcı veri, arayüz, testler ve dağıtımla gerçek bir uygulamada birleştirmek |

İki aşama ayrı Go modülleridir. Komutları doğru klasörde çalıştırmak gerekir. Kubernetes tarafında aynı `go-odev` kind kümesi içinde alıştırmalar `odev`, uygulama `gorev-atolyesi` namespace'inde bulunur.

## 2. Go giriş alıştırmaları

### Not ortalaması

[temeller/main.go](../../01-go/giris/temeller/main.go) içinde değişken, `[]int` slice, `range` döngüsü, koşul, tür dönüşümü ve birden fazla değer döndüren fonksiyon kullanıldı.

`ortalama` fonksiyonu `(float64, error)` döndürür. Liste boşsa hata üretir; doluysa toplamı eleman sayısına böler. Böylece hata, işlemin normal sonucundan ayrı taşınır. Örnekteki 70, 85 ve 95 notları `83.33` ortalama verir; geçme koşulu `sonuc >= 60` olarak hesaplanır.

### Struct, pointer ve map

[struct-map/main.go](../../01-go/giris/struct-map/main.go) içinde `Ogrenci` tipi oluşturuldu. `Bonus` metodunun alıcısı `*Ogrenci` olduğu için puan değişikliği özgün öğrenci üzerinde yapılır. Deniz'in puanı 75'ten 85'e çıkar.

Ders puanları `map[string]int` ile tutulur. `puan, varMi := dersler["Python"]` kullanımı, kaydın bulunmamasıyla gerçek bir sıfır değerini ayırt etmeyi gösterir.

## 3. Go orta seviye alıştırmaları

### Worker pool: goroutine ve kanallar

[worker-pool/main.go](../../01-go/orta/worker-pool/main.go) üç goroutine ile 1–8 arasındaki sayıların karelerini hesaplar.

1. Üretici sayıları `isler` kanalına gönderir ve kanalı kapatır.
2. Üç çalışan aynı kanaldan iş alır ve sonuçları `sonuclar` kanalına yollar.
3. `sync.WaitGroup` çalışanların bitmesini takip eder.
4. Tüm çalışanlar bitince sonuç kanalı kapatılır.
5. Ana goroutine sonuçları toplar ve gösterim için sayıya göre sıralar.

Çalışma sırası eşzamanlılık nedeniyle değişebilir; çıktı sıralaması özellikle sabitlenmiştir. Kanalın doğru zamanda kapatılması, bitişin tüketiciye bildirilmesini sağlar. Bu örnek ayrı bir öğrenme çalışmasıdır; görev API'sinde aynı worker pool'un kullanıldığı anlamına gelmez.

### İlk HTTP API

[http-api](../../01-go/orta/http-api/main.go) ve [internal/webapp](../../internal/webapp) ile `net/http` tabanlı bir sunucu hazırlandı. `/api/selam?ad=Deniz` JSON cevabı verir; `/healthz` sağlık kontrolüdür. `MESAJ` ortam değişkeniyle cevap metni değiştirilebilir. Tarayıcıdaki form aynı API'yi çağırır.

Bu örnek, Go fonksiyonlarını HTTP üzerinden kullanılabilir hale getirmenin temelini oluşturdu. Sonraki uygulamada bu yapı görev oluşturma, güncelleme ve silme işlemlerine genişletildi.

## 4. Go ile Görev Atölyesi

### Paket ve sorumluluk ayrımı

| Dosya | Yapılan iş |
|---|---|
| [cmd/gorev-atolyesi/main.go](../cmd/gorev-atolyesi/main.go) | Ortam ayarlarını okuma, veritabanını açma, HTTP sunucusu ve düzgün kapanma |
| [internal/tasks/store.go](../internal/tasks/store.go) | Görev modeli, doğrulama, SQL işlemleri ve sürüm çakışmaları |
| [internal/tasks/schema.sql](../internal/tasks/schema.sql) | Tablo, alanlar ve veri kısıtları |
| [internal/tasks/http.go](../internal/tasks/http.go) | HTTP yolları, JSON kodlama, hata cevapları, indirme ve statik dosyalar |
| [internal/tasks/static](../internal/tasks/static) | HTML, CSS, JavaScript, ikonlar ve yerel yazı tipleri |

Arayüz dosyaları Go binary'sine gömülür. Böylece arayüz ve API tek sunucudan sunulur; ayrı Node.js sunucusu gerekmez. Arayüz dosyası değiştiğinde yerelde sunucuyu yeniden başlatmak, Kubernetes'te yeni imaj derlemek gerekir.

### Veri modeli ve doğrulama

Bir görev `id`, `title`, `description`, `category`, `priority`, `status`, `due_date`, `version`, `created_at` ve `updated_at` alanlarını taşır.

- Başlık 3–120 karakter, açıklama en fazla 2000 karakterdir.
- Kategori Go, Kubernetes, chromedp veya Genel olabilir.
- Öncelik `low`, `medium`, `high`; durum `todo`, `doing`, `done` değerlerinden biridir.
- Teslim tarihi boş veya `YYYY-MM-DD` biçimindedir.
- Kimlik, sürüm ve zaman bilgileri sunucu tarafından yönetilir.

Go tarafında doğrulama ve normalleştirme yapılır; SQLite şemasındaki kısıtlar da temel alanları korur. Kullanıcı girdileri SQL metnine eklenmez, parametre olarak gönderilir.

### Kalıcılık ve sürüm kontrolü

`database/sql` ve modernc SQLite sürücüsü kullanıldı. Yerel kayıtlar varsayılan olarak `data/gorevler.db` içindedir. WAL modu ve 5 saniyelik `busy_timeout` ayarı vardır. Bağlantıya özgü ayarların tutarlı uygulanması için bağlantı havuzu tek açık bağlantıyla sınırlandırılmıştır.

Güncelleme ve silme, görevin son okunan `version` değerini ister. Örneğin iki sekme sürüm 1'i açarsa ilk kaydetme sürümü 2 yapar; ikinci sekmenin sürüm 1 ile kaydetmesi 409 çakışma cevabı alır. Kullanıcı panoyu yenileyip güncel kayıt üzerinde çalışır. Bu, eski bir sekmenin yeni değişikliği sessizce ezmesini önler.

### API ve tek tıkla JSON indirme

| Metot ve yol | Görev |
|---|---|
| `GET /api/tasks` | Tüm görevleri listeleme |
| `POST /api/tasks` | Görev oluşturma |
| `PUT /api/tasks/{id}` | Sürüm kontrollü güncelleme |
| `DELETE /api/tasks/{id}?version=...` | Sürüm kontrollü silme |
| `GET /api/export/tasks` | Güncel tüm görevleri dosya olarak indirme |
| `GET /healthz` | Sürecin HTTP cevap verebildiğini gösterme |
| `GET /readyz` | Veritabanına erişimi kontrol etme |

JSON indirme tarayıcıda mevcut veriden Blob üretmek yerine sunucuda uygulanmıştır. Bağlantıya tıklanınca veritabanı yeniden okunur; `Content-Disposition: attachment` ile tarih-saat içeren dosya adı gönderilir. Dosyada `format`, `exported_at`, `count` ve `tasks` bulunur. Sayfadaki filtre veya eski liste indirme sonucunu değiştirmez. Yanıt önbelleğe alınmaz. JSON içe aktarma henüz yoktur.

Ayrıntılı istek ve cevap örnekleri [API sözleşmesinde](API.md) bulunur.

### Sunucu yaşam döngüsü

Sunucu `ADDR` ve `DB_PATH` ortam değişkenlerini okur. HTTP başlık, okuma, yazma ve boşta bekleme süreleri sınırlıdır. `slog` ile JSON günlükleri üretilir.

Ctrl+C veya SIGTERM geldiğinde `signal.NotifyContext` kapanmayı başlatır; `http.Server.Shutdown` için 10 saniyelik süre verilir. Böylece aktif isteklerin tamamlanmasına fırsat tanınır ve veritabanı kapatılır. Kubernetes'te Pod için tanımlanan 20 saniyelik kapanma süresi bu uygulama davranışıyla birlikte çalışır.

### Arayüz ve doğrulama

Üç aşamalı pano, liste, arama, kategori/öncelik filtreleri, tarih filtresi, sıralama, tema, şablonlar ve klavye kısayolları eklendi. Görevler sunucuda, tema ve görünüm tercihi tarayıcıda tutulur. Görev metinleri DOM'a `textContent` ile yazılır; API istek gövdesi 16 KB ile sınırlıdır ve çapraz kaynak koruması uygulanır.

[tasks_test.go](../internal/tasks/tasks_test.go) kalıcılığı, CRUD davranışını, geçersiz veriyi, bulunmayan kaydı, sürüm çakışmasını ve JSON indirmeyi kontrol eder. Sekiz eşzamanlı güncellemede yalnızca birinin kabul edilmesi sınanır. Kontrol scripti ayrıca race detector ve `go vet` çalıştırır.

[chromedp kabul testi](../cmd/gorev-e2e/main.go) gerçek Chrome ile görev ekleme, düzenleme, yenileme, filtreleme, silme ve dosya indirmeyi dener. Masaüstü, mobil, koyu tema, liste ve form görüntüleri üretilir. Testler geçici veritabanı kullanır; gerçek görevleri örnek verilerle değiştirmez.

## 5. Kubernetes giriş ve orta seviye çalışmaları

Docker container çalıştırma altyapısı olarak, kind yerel Kubernetes kümesini oluşturmak için, kubectl ise kaynakları yönetmek için kullanıldı. Kümenin adı `go-odev` olarak belirlendi.

İlk aşamada Go API imajı `go-odev:1.0` etiketiyle oluşturulup kind düğümüne yüklendi. [Giriş Pod'u](../../02-kubernetes/giris/pod.yaml) `odev` namespace'inde `merhaba-go` adında tek uygulama örneği çalıştırır.

[Orta seviye YAML](../../02-kubernetes/orta/uygulama.yaml) şu kaynakları ekler:

- `go-api` Deployment: iki uygulama replikasını yönetir.
- `go-api` Service: 80 portunu Pod'un 8080 portuna yönlendirir.
- `go-ayarlar` ConfigMap: `MESAJ` değerini container ortamına verir.
- Readiness ve liveness kontrolleri: `/healthz` üzerinden hazır olma ve canlılığı denetler.
- CPU/bellek istekleri, sınırları ve root olmayan kullanıcı ayarları.

Tek Pod ile Deployment farkı burada görünür: Deployment istenen replika sayısını korumak için Pod'ları yönetir. İlk API kalıcı görev veritabanı tutmadığından iki replika örneği uygundur. ConfigMap ortam değişkeni olarak yüklendiği için değişiklik mevcut sürece kendiliğinden geçmez; Pod yenilenmelidir.

## 6. Görev Atölyesi'nin container paketi

[Dockerfile](../deploy/gorev-atolyesi/Dockerfile) çok aşamalı derleme kullanır. İlk aşamada Go modülleri indirilir ve `CGO_ENABLED=0` ile uygulama derlenir. Son `scratch` imajına yalnızca çalıştırılabilir dosya kopyalanır; gömülü arayüz de bu dosyanın içindedir.

Uygulama UID/GID 65532 ile çalışır. Container içinde `ADDR=:8090`, `DB_PATH=/data/gorevler.db` kullanılır. İmaj uygulamanın kodunu taşır; görev verileri imajın içine yazılmaz.

## 7. Görev Atölyesi Kubernetes kaynakları

Tüm kaynaklar [kubernetes.yaml](../deploy/gorev-atolyesi/kubernetes.yaml) içinde tanımlıdır.

| Kaynak/ayar | Projedeki karşılığı | Neden kullanıldı? |
|---|---|---|
| Namespace | `gorev-atolyesi` | Uygulamayı ilk alıştırmalardan ayırmak |
| Deployment | `gorev-atolyesi`, 1 replika | Uygulama Pod'unu yönetmek |
| Strateji | `Recreate` | Güncellemede eski Pod'u kapatıp yenisini açmak |
| ConfigMap | `gorev-ayarlar` | Dinleme adresi ve veritabanı yolunu koddan ayırmak |
| PVC | `gorev-veri`, 1 GiB, ReadWriteOnce | `/data` altındaki veriyi Pod ömründen ayırmak |
| Service | 80 → 8090 | Pod'a servis adı üzerinden erişmek |
| Readiness | `/readyz`, 3 saniyede bir | Veritabanına erişebilen Pod'u hazır saymak |
| Liveness | `/healthz`, 10 saniyede bir | Canlılık denetimi; ilk kontrol 10 saniye sonra |
| Kaynaklar | İstek 50m CPU / 64Mi; sınır 500m / 256Mi | Kaynak beklentisi ve üst sınır tanımlamak |

SQLite için tek replika ve Recreate birlikte seçildi. Amaç, bu yerel dosya tabanlı uygulamada güncelleme sırasında eski ve yeni uygulama Pod'larının birlikte çalışmamasıdır. Bunun karşılığında güncellemede kısa kesinti olur. ReadWriteOnce tek başına yalnızca bir Pod'un yazmasını garanti eden bir uygulama kilidi değildir; replika stratejisinin yerine geçmez.

Root olmayan kullanıcı, `fsGroup: 65532`, varsayılan seccomp profili, salt okunur kök dosya sistemi, yetki yükseltmenin kapatılması ve Linux capability'lerinin kaldırılması tanımlandı. Yazılabilir görev verileri PVC üzerindeki `/data` yoluna gider.

## 8. Dağıtım, erişim ve kapatma akışı

`scripts/proje-k8s.sh` sırasıyla Docker erişimini kontrol eder, küme yoksa oluşturur, proje `.kubeconfig` dosyasını yeniler, imajı derler, kind içine yükler, kaynakları uygular ve Deployment'ın hazır olmasını bekler. İmaj etiketi zaman ve süreç kimliği içerdiğinden kod değişikliği yeni bir rollout başlatır. `imagePullPolicy: Never` nedeniyle imajın kind içine yüklenmesi zorunludur.

`scripts/proje-ac.sh` replikayı 1 yapar, rollout'u bekler ve `8090:80` port yönlendirmesini açık tutar. Akış şöyledir:

```text
Tarayıcı localhost:8090
  → kubectl port-forward service/gorev-atolyesi
  → Service port 80
  → uygulama Pod'u port 8090
  → SQLite /data/gorevler.db → gorev-veri PVC
```

Port yönlendirmesi terminali açık kalmalıdır. Bu yöntem yerel erişim içindir; internete yayın veya kalıcı Ingress kurulumu yapılmış değildir.

`scripts/proje-kapat.sh` Docker ve küme durumunu kontrol eder, eski API portunu kubeconfig üzerinden yeniler, API hazırsa replikayı 0 yapıp Pod'un silinmesini bekler. Küme bulunmuyorsa veya düğüm durmuşsa durumu bildirir. Erişim veya yetki hatasında kapatma doğrulanamadığını belirtir; yanıltıcı başarı mesajı vermez. PVC'yi silmez, Docker'ı veya tüm kümeyi kapatmaz. Kapanmış bir kümede replika ayarı değiştirilemediği için küme tekrar açıldığında uygulama yeniden çalışabilir.

## 9. Verinin nerede olduğu ve mevcut sınırlar

Yerel sunucu `data/gorevler.db`, Kubernetes ise PVC üzerindeki `/data/gorevler.db` dosyasını kullanır. Bunlar ayrı veritabanlarıdır; birindeki görevler diğerine otomatik taşınmaz. Yerel sunucu Ctrl+C ile, Kubernetes uygulaması kapatma scriptiyle durdurulur.

Pod'un yenilenmesi veya replikanın 0 olması PVC'yi silmez. PVC ya da kind kümesinin silinmesi veri kaybına yol açabilir; PVC yedek değildir. Yerel yedek için sunucuyu durdurduktan sonra tüm `data` klasörü kopyalanmalıdır; çalışan WAL veritabanının yalnızca `.db` dosyasını kopyalamak yeterli değildir.

Uygulama kişisel yerel kullanım içindir. Kullanıcı hesabı, çok kullanıcılı yetkilendirme, JSON içe aktarma, sayfalama, PostgreSQL, Ingress/TLS ve otomatik veritabanı yedekleme uygulanmadı. Uzak CI yapılandırması hazırlandı ancak GitHub üzerinde çalıştığı doğrulanmadı. Bunlar yapılan işlerden ayrı, sonraki geliştirme alanlarıdır.

## 10. Doğrulama ile kurulumun farkı

Go testleri ve Chrome kabul testleri yerel olarak başarılı çalıştırıldı. Önceki Kubernetes kurulumunda Pod, Deployment ve PVC durumları kontrol edildi. Son arayüz yenilemesi yerelde doğrulandı; Docker erişilemediği için bu yenilemenin Kubernetes dağıtımı o turda yapılmadı. Güncel tasarımı kümeye geçirmek için README'deki güncelleme komutları çalıştırılmalıdır.

Tarihsel kontrol kayıtları: [DOGRULAMA.md](../DOGRULAMA.md). Bir testin geçmişte başarılı olması, kümenin şu anda açık olduğu anlamına gelmez; güncel durum `kubectl get` ve rollout çıktısından görülür.
