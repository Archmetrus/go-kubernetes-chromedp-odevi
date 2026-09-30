# Görev Atölyesi doğrulama sonuçları

## Hesaplı sürüm ve yayın hazırlığı — 10 Eylül 2026

- SQLite ve gerçek PostgreSQL 18 üzerinde kullanıcı ayrımı, CRUD/export/restore izolasyonu, migration tekrarı, oturum iptali/süresi, CSRF, hız sınırı ve eşzamanlı sürüm çakışmaları test edildi. PostgreSQL oturumu ayrı Store/bağlantı havuzuyla da doğrulandı.
- Go testleri race detector ile ve `go vet ./...` başarılı. Salt okunur SQLite→PostgreSQL taşıması, kaynak korunması ve tekrar taşımanın reddi doğrulandı.
- Chrome'da hesap oluşturma/giriş, ikinci kullanıcının boş panosu, commit edilmiş dış verinin otomatik görünmesi, arşiv, çöp kutusu/geri alma ve gerçek JSON içe/dışa aktarma başarılı.
- Yeni giriş ekranı ve mobil pano görsel olarak incelendi; önceki masaüstü/mobil/koyu/liste/form ekranları yeniden üretildi.
- Uygulama imajı derlendi ve geçici PostgreSQL'e bağlanarak `/readyz` ve `/api/session` cevapları doğrulandı. Kullanıcı veritabanına dokunulmadı.
- Yedek imajı root olmayan kullanıcı, salt okunur kök ve geçici `/tmp` ile çalıştı. Custom-format PostgreSQL yedeği ayrı boş hedefe geri yüklendi (görev sayısı ve şema sürümü doğrulandı); dolu hedefe ikinci geri yükleme reddedildi.
- Staging manifestleri Kubernetes server-side dry-run kontrolünden geçti: ConfigMap, Deployment, Service, Ingress, PDB, migration Job ve yedek CronJob. Gerçek dağıtım yapılmadı.
- `actionlint v1.7.12` ile üç GitHub workflow dosyası, Bash scriptlerinin sözdizimi, 23 fish komut bloğu ve belge bağlantıları doğrulandı.
- `govulncheck v1.8.0`: uygulamanın çağırdığı kodda bilinen açık yok; kullanılmayan modül kodunda bir kayıt bildirildi. `v1.1.4` Go 1.27 ile uyumsuz olduğundan güncellendi. Uygulama imajı Trivy 0.69.3 HIGH/CRITICAL taramasından bulgusuz geçti.
- Docker soketini tarama container'ına bağlama denemesi otomatik onay denetimince reddedildi. Tarama, Docker erişimi olmadan yalnızca dışa aktarılan imaj dosyasının salt okunur bağlanmasıyla tamamlandı.

Gerçek bulut TLS/DNS, GHCR yayın/deploy workflow çalışması, bucket'a yükleme, uyarı alıcıları ve production rollback henüz dış ortamda doğrulanmadı. Kurulum ve kabul sırası [canlıya hazırlık rehberinde](docs/CANLIYA-HAZIRLIK.md). Önceki başlıklar tarihsel kayıtlardır.

## Başarılı kontroller

- `bash scripts/proje-kontrol.sh`: tüm Go testleri, race detector ve vet başarılı.
- SQLite yeniden açılma, CRUD, veri doğrulama, eski sürümle güncelleme/silme, çapraz kaynak engeli ve 8 eşzamanlı güncelleme senaryoları doğrulandı.
- Gerçek Chrome ile oluşturma, sayfa yenileme, düzenleme, durum değiştirme, filtreleme ve silme başarılı.
- Masaüstü 1440 px / mobil 390 px ekran görüntüleri üretildi ve incelendi; yatay taşma yok.
- Kubernetes manifesti: 5 kaynak geçerli, 0 hata, 0 atlanan kaynak.
- Yeni Docker imajı oluşturuldu ve mevcut `go-odev` kind kümesine yüklendi.
- `gorev-atolyesi` namespace'inde Deployment ve Service oluşturuldu; PVC bağlandı.
- Kubernetes canlı kalıcılık testi: geçici görev oluşturuldu, Deployment yeniden başlatıldı, kayıt tekrar okundu ve test kaydı silindi. Pod `Running 1/1`, PVC `Bound`.
- Fish ortam dosyası ve Bash yardımcı betikleri sözdizimi kontrolünden geçti.

## Sınırlar ve notlar

- GitHub Actions yapılandırması hazır; uzak CI çalıştırılmadı.
- Docker mevcut legacy builder için kullanım dışı kalma uyarısı veriyor; imaj derlemesi başarılı.
- Chrome, dialog açılışlarında chromedp'nin işlemediği `dom.EventTopLayerElementsUpdated` olayını raporluyor; oluşturma/düzenleme/silme akışları ve sonuç kontrolleri başarılı.
- Bu kişisel uygulamada kimlik doğrulama yok. İnternet yayını ve çok kullanıcılı kullanım doğrulanmadı.
- Ekran görüntülerinin örnek kayıtları geçici test veritabanına aittir.

## Bağımsız klasöre taşıma

- Uygulama `gorev-atolyesi/` altında `example.com/gorev-atolyesi` Go modülüne ayrıldı.
- Yeni konumdan Go testleri, race detector, vet ve chromedp kabul testi başarılı.
- Yeni klasördeki Kubernetes bağlantısı aynı Deployment (`1/1`) ve PVC (`Bound`) ile doğrulandı.
- İlk alıştırmaların ayrı Go modülünde test ve vet kontrolleri başarılı.

## Arayüz yenilemesi

- Görev CRUD kabul testi yeni arayüzde başarılı.
- Odak, yaklaşan tarih ve gün filtresi; liste görünümü; öncelik sıralaması; tema/görünüm kalıcılığı; şablon ön doldurma ve JSON dışa aktarma içeriği gerçek Chrome ile doğrulandı.
- Masaüstü ve mobil ekran görüntüleri güncellendi; yatay taşma kontrolü başarılı.
- Go testleri, race detector ve vet başarılı.

## Ürün referansları ve sunucudan indirme

- Todoist ve Linear resmî ürün ekranları görsel olarak incelendi; arayüzde kompakt çalışma alanı düzeni uygulandı.
- Export uç noktası için dosya başlıkları, boş liste, güncel kayıt, sürüm bilgisi, önbellek ve hata davranışları test edildi.
- Chrome'da gerçek JSON dosyası geçici klasöre indirildi ve okundu: sayfada hiçbir görev görünmeyen bir filtre açıkken, sayfa yüklendikten sonra DB'ye eklenen altıncı görev de dosyadaydı.
- Go testleri/race/vet ve tarayıcı kabul testi başarılı. Masaüstü ve mobil görüntüler incelendi.

## Kesinti sonrası kontrol — 10 Eylül 2026

- Kaynak kodda referans ürünlere göre düzenlenen arayüz ve sunucu tabanlı JSON indirme mevcut.
- Docker servisi kapalı; yönetici parolası gerektiği için Kubernetes dağıtımının son durumu henüz doğrulanamadı.
- Yerel uygulama `127.0.0.1:8091` üzerinde açıldı. Bu çalışma modu `data/gorevler.db` kullanır; Kubernetes PVC'sinden ayrıdır.
- Yerel HTTP kontrolü başarılı: yeni indirme bağlantısı mevcut; export cevabı HTTP 200, JSON içerik türü, dosya indirme başlığı ve `no-store` ile dönüyor.

## Çalışma tezgâhı tasarımı — 10 Eylül 2026

- Arayüz ve yerel yazı tipleri yenilendi; mevcut API ve kayıt yapısı korundu.
- Go testleri, race detector, vet ve gerçek Chrome kabul testi başarılı.
- Masaüstü, mobil, koyu görünüm, liste ve mobil form ekran görüntüleri `outputs/gorev-*.png` olarak üretildi ve incelendi. Bu beş görünümde yatay taşma kontrolü başarılı.
- Görev işlemleri, filtreler, tema kalıcılığı, şablonlar ve gerçek JSON dosyası indirme yeniden doğrulandı. Görsel testler geçici veritabanı kullanır.
- Kubernetes dağıtımı Docker kapalı olduğundan bu tasarım turunda yenilenmedi; yerel sunucu 8091 portunda kullanılır.
