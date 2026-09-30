# Görev Atölyesi

## Excel/JSON toplu atama ve otomasyon

Ekip ekranındaki **Dosyadan görev ata** ile dosyayı önizle, hataları düzelt ve görevleri tek işlemde ata. Aynı ekranda grup kapsamlı, süreli erişim anahtarı oluşturabilir ve iptal edebilirsin. Web, terminal ve harici otomasyon aynı API'yi kullanır; tekrar denemeler aynı istek anahtarıyla görev çoğaltmaz.

```fish
cd (git rev-parse --show-toplevel)/gorev-atolyesi
and bash scripts/ekip-demo.sh restart
```

Demo: http://127.0.0.1:8092 → **Ekip ağacı ve görevler → Dosyadan görev ata**. Örnek dosya `examples/toplu-atama-demo.json`. Anahtarı web'den oluşturduktan sonra terminalde:

```fish
bash scripts/ekip-demo.sh assign setup
and bash scripts/ekip-demo.sh assign preview --file examples/toplu-atama-demo.json
```

Önizleme çıktısını inceledikten sonra:

```fish
bash scripts/ekip-demo.sh assign commit
and bash scripts/ekip-demo.sh assign results
```

TUI: `bash scripts/ekip-demo.sh tui`; dosyalar `data/imports/` içinde listelenir. [Dosya biçimi, güvenlik, Robomotion HTTP sözleşmesi ve tüm komutlar](docs/TOPLU-ATAMA-VE-OTOMASYON.md). En fazla 200 görev / 2 MiB; tek sayfalı `.xlsx`, formülsüz ve tarihleri metin olarak kullanılır. Migration 7 SQLite'ta yeniden başlatmada uygulanır; PostgreSQL'de dağıtım `migrate` adımı gerekir.

Go ile yazılmış, SQLite kullanan kişisel görev takip uygulaması. Görevleri oluşturun, düzenleyin, silin; üç sütunlu panoda durumlarını değiştirin. Kategori, öncelik ve teslim tarihi belirleyin; başlık ve açıklamada arayın. İlerleme satırı toplam, devam eden, tamamlanan ve geciken işleri gösterir.

## Ekip ağacı ve görev atama demosu

Ana yönetici → grup liderleri → üyeler hiyerarşisi, rol bazlı görev atama/takip, üye/yetki yönetimi ve görev geçmişi eklendi. Ayrı demo DB'sinde 10 test hesabı, 3 grup ve 12 görev hazırdır. Mevcut kişisel veriler korunur.

```fish
cd (git rev-parse --show-toplevel)/gorev-atolyesi
and bash scripts/ekip-demo.sh seed
and bash scripts/ekip-demo.sh up
and bash scripts/ekip-demo.sh accounts
```

**http://127.0.0.1:8092** adresinde giriş yap; **Ekip ağacı ve görevler** düğmesini seç. `accounts` komutu yerel demo parolalarını ve yönetici/lider için geçerli MFA kodunu gösterir. Ana yönetici: `yonetici@atolye.example.test`. Kod tüketildiyse yeni 30 saniyelik aralığı bekle.

Güncelle: `bash scripts/ekip-demo.sh restart`. Kapat: `bash scripts/ekip-demo.sh down`. Terminal menüsü: `bash scripts/ekip-demo.sh tui` (local profilde kal).

[Ağaç, roller, DB konumları ve ayrıntılı kullanım](docs/EKIP-AGACI-DEMO.md). Demo DB `data/demo/ekip.db`; giriş bilgileri/MFA anahtarı `.control/demo/` altındadır. Bu klasörü koru. Canlı yayın engeli devam eder.

**[Go ve Kubernetes için neler yaptık? Ayrıntılı proje raporu](docs/GO-VE-KUBERNETES.md)**

## Ok tuşlarıyla terminal kontrolü

```fish
cd (git rev-parse --show-toplevel)/gorev-atolyesi
and bash scripts/atolye.sh control tui
```

↑↓ ile seçin, Enter ile çalıştırın; Tab ile `local`/`kind` değiştirin. `q` panelden çıkar; sunucuyu durdurmaz. Menüde başlatma, durdurma, yeniden derleyerek yerel güncelleme, durum, loglar ve yedek listesi bulunur.

Menüsüz yerel başlatma ve durum:

```fish
bash scripts/atolye.sh system up --profile local
and bash scripts/atolye.sh system status --profile local --json
```

Güncelleme: `bash scripts/atolye.sh system restart --profile local`.
Kapatma: `bash scripts/atolye.sh system down --profile local`.

Web üzerinden kayıt ve konsoldan ilk yönetici için menüde **Hesaplı kaydı etkinleştir → Başlat** seçin. Sitede kayıt olup giriş yapın; terminalde **Doğrulama iletilerini göster** ile kodu alın ve sitede doğrulayın. Web'de **İki adımlı doğrulamayı kur / yönet** ile MFA kurup kurtarma kodlarını saklayın. Sonra terminalde **İlk yöneticiyi seç** ile kendi hesabınızı belirleyin. [Kayıt/yönetici rehberi](docs/HESAP-ONAYI.md) · [MFA ve kurtarma rehberi](docs/MFA-VE-KURTARMA.md).

Yeni hesap onaylı mod geliştirme ortamına açıktır. Doğrulama iletileri yerel posta kutusuna bırakılır; gerçek e-posta gönderilmez. MFA eklendi; SMTP teslimatı ve parola sıfırlama tamamlanmadan staging/production'da başlamaz. **`.control/mfa.key` şifreleme anahtarını DB yedeğiyle birlikte koruyun; `.control/` klasörünü silmeyin.** Önceden başka scriptle açılmış sunucular otomatik devralınmaz. [Profiller, veri konumu, gereksinimler ve tüm komutlar](docs/TERMINAL-KONTROL.md).

## Hesaplı sürüm ve yayın hazırlığı

**[Yeni özellikler, hesap oluşturma, PostgreSQL taşıma ve canlıya hazırlık rehberi](docs/CANLIYA-HAZIRLIK.md)**

Giriş/çıkış, kullanıcıya ait görevler, arşiv, çöp kutusundan geri alma, önizlemeli JSON içe aktarma ve 5 saniyelik otomatik yenileme eklendi. PostgreSQL migration, yedek/geri yükleme scriptleri, staging/production manifestleri ve CI/release/deploy workflow'ları hazırlandı. Gerçek alan adı, TLS, registry ve bulut hesapları yapılandırılmadan internete yayın yapılmış olmaz.

Önceki, konsoldan hesap oluşturulan kullanım (yeni web kayıt akışı için yukarıdaki rehberi kullanın):

```fish
cd (git rev-parse --show-toplevel)/gorev-atolyesi
and bash scripts/hesap-olustur.sh
and env AUTH_ENABLED=true ADDR=127.0.0.1:8091 bash scripts/proje-baslat.sh
```

İlk komut hesabı bir kere oluşturur; sonraki açılışlarda yalnızca son komutu çalıştırın. Eski SQLite görevlerini bu hesaba bağlamak için rehberdeki `claim-local` adımını kullanın. Aşağıdaki eski yerel/kind komutları kişisel geliştirme modunu açar; canlı dağıtım için `deploy/live` ve yeni rehberi kullanın.

## Yeni çalışma alanı

- **Pano ve liste:** Sağ üstte görünüm değiştirin; seçiminiz bu tarayıcıda hatırlanır.
- **Bugünün odağı:** Tamamlanmamış görevlerden devam eden, yüksek öncelikli veya teslim tarihi bugün/geçmiş olanları gösterir.
- **Yaklaşan işler:** Bugün dahil önümüzdeki yedi günün açık görevleri.
- **Yedi günlük takvim:** Bir güne tıklayarak teslim tarihine göre filtreleyin. Diğer filtreler de birlikte uygulanır.
- **Sıralama:** En yeni, öncelik, teslim tarihi veya başlık. Tarihsiz işler teslim sıralamasında sonda kalır.
- **Şablonlar:** Go, Kubernetes ve chromedp şablonları düzenlenebilir form açar; Kaydet demeden görev oluşturmaz.
- **Koyu görünüm:** Sol menüden değişir, tarayıcıda hatırlanır.
- **JSON dışa aktarma:** “Tüm görevleri indir”, mevcut kullanıcının aktif ve arşivlenmiş görevlerini güncel veritabanından indirir. Filtreler sonucu etkilemez; çöp kutusu dahil değildir. “JSON içe aktar” dosyayı önce doğrular ve önizletir, onaydan sonra yeni görevler ekler.
- **Klavye:** `N` yeni görev açar, `/` aramaya odaklanır. Yazı alanları ve açık formlarda kısayollar devre dışıdır.

Görevler SQLite'ta kalır; yalnızca görünüm ve tema tercihi tarayıcıda saklanır.

## Hemen çalıştır (fish)

Bu belgedeki `and`, önceki komut başarılıysa sonraki komutu çalıştırır. `.sh` dosyaları fish içinde `bash` ile çalıştırılır. Aşağıdaki yerel ve Kubernetes akışlarından kullanmak istediğinizi seçin.

### 1. Yerel sunucuyu aç

```fish
cd (git rev-parse --show-toplevel)/gorev-atolyesi
and source scripts/env.fish
and env ADDR=127.0.0.1:8091 bash scripts/proje-baslat.sh
```

Tarayıcı: **http://127.0.0.1:8091**. Terminal açık kalır; Ctrl+C ile durdurun. Sunucu günlükleri bu terminalde görünür. Görev başlığına tıklayarak düzenleyin, karttaki durum menüsünden taşıyın. Veriler `data/gorevler.db` dosyasında kalır. Pano ilk açılışta boştur; test verileri gerçek veritabanına eklenmez.

### 2. Yerel kod değişikliklerini yükle

Sunucunun çalıştığı terminalde önce **Ctrl+C**, ardından yukarıdaki açma bloğunu tekrar çalıştırın. Go, gömülü HTML/CSS/JavaScript dosyalarını yeniden derler. Otomatik yeniden yükleme yoktur. Tarayıcıda gerekirse **Ctrl+Shift+R** kullanın. `address already in use` çıkarsa aynı portta önceki sunucu hâlâ çalışıyordur.

Başka terminalde çıktıyı kontrol etmek için:

```fish
curl -fsS http://127.0.0.1:8091/readyz
and curl -fsS http://127.0.0.1:8091/api/tasks
```

Bu komut kişisel geliştirme modudur. Hesaplı kullanım için `AUTH_ENABLED=true` gerekir; internet yayını için yeni canlıya hazırlık rehberindeki ek yapılandırmaları tamamlayın.

## Kubernetes'te çalıştır (fish)

### 1. Docker'ı aç, güncel kodu dağıt ve siteye bağlan

```fish
cd (git rev-parse --show-toplevel)/gorev-atolyesi
and source scripts/env.fish
and sudo systemctl start docker
and docker info
and bash scripts/proje-k8s.sh
and bash scripts/proje-ac.sh
```

Tarayıcı: **http://127.0.0.1:8090**. Son komut `Forwarding from 127.0.0.1:8090` çıktısıyla bekler; bu normaldir, terminal açık kalmalıdır. Dağıtım betiği mevcut `go-odev` kümesini kullanır, yoksa oluşturur. Yeni uygulama `gorev-atolyesi` namespace'indedir; önceki `odev` örnekleri ayrı çalışır. Her derleme ayrı imaj etiketi alır. `docker info` başarısızsa `and` zinciri dağıtıma geçmez.

Bu README yerel sunucuyu 8091'de açar. `proje-baslat.sh` adres verilmeden çalıştırılırsa varsayılan 8090'ı kullanır; Kubernetes bağlantısını açmadan önce o sunucuyu kapatın. Yerel veritabanı ve Kubernetes PVC'si ayrıdır; görevler otomatik eşitlenmez.

### 2. Daha sonraki kod değişikliklerini güncelle

Önce eski port yönlendirme terminalinde **Ctrl+C** yapın. Ardından yukarıdaki dağıtım bloğunu tekrar çalıştırın. Yalnızca `proje-ac.sh` çalıştırmak yeni kodu derlemez. Tarayıcıda gerekirse **Ctrl+Shift+R** kullanın.

### 3. Durumu ve günlükleri gör

Başka terminalde durum ve günlükler:

```fish
cd (git rev-parse --show-toplevel)/gorev-atolyesi
and source scripts/env.fish
and set -gx KUBECONFIG "$PWD/.kubeconfig"
and kubectl --context kind-go-odev -n gorev-atolyesi get pods,deployments,pvc,services
and kubectl --context kind-go-odev -n gorev-atolyesi logs deployment/gorev-atolyesi --tail=30
```

Beklenen durum: Pod `1/1 Running`, Deployment `1/1`, PVC `Bound`. Sürekli günlük izlemek için son komuta `-f` ekleyin; izlemeyi Ctrl+C ile bırakabilirsiniz.

SQLite için **tek replika + Recreate** stratejisi kullanılır. Güncellemede kısa kesinti olur. Veritabanı 1 GiB PVC üzerinde tutulur; Pod değişince korunur. kind kümesini veya PVC'yi silmek verileri kaybettirebilir. Yatay ölçekleme için sonraki adım PostgreSQL gibi paylaşımlı bir veritabanına geçmektir.

## Kapat ve yeniden aç (fish)

Kubernetes'teki uygulamayı durdurmak için:

```fish
cd (git rev-parse --show-toplevel)/gorev-atolyesi
and bash scripts/proje-kapat.sh
```

Bu betik uygulamanın replika sayısını sıfıra indirir ve Pod kapanana kadar bekler. Kayıtlı görevler ve kalıcı disk korunur. Pod kapanınca ona bağlı port yönlendirmesi de sonlanır. Docker, Kubernetes kümesi ve önceki alıştırmalar çalışmaya devam eder.

Docker/API erişilemiyorsa kısa bir açıklama verir; doğrulanamayan kapatmayı başarılı saymaz. Küme yoksa veya kontrol düğümü durmuşsa yapılacak işlem olmadığını bildirir. Çalışan kümede bağlantı bilgilerini yenileyerek eski API portunu düzeltir. Bu script 8091'deki yerel Go sunucusunu kapatmaz.

Yeniden açmak için (imajı tekrar derlemez):

```fish
cd (git rev-parse --show-toplevel)/gorev-atolyesi
and source scripts/env.fish
and sudo systemctl start docker
and docker info
and kind export kubeconfig --name go-odev --kubeconfig "$PWD/.kubeconfig"
and bash scripts/proje-ac.sh
```

Bu yeniden açma akışı daha önce kurulmuş, çalışan `go-odev` kümesi içindir. Küme veya uygulama yoksa ilk dağıtım bloğunu kullanın.

Yerel `proje-baslat.sh` sunucusunu ise çalıştığı terminalde **Ctrl+C** ile durdurun.

## Testler

```fish
cd (git rev-parse --show-toplevel)/gorev-atolyesi
and bash scripts/proje-kontrol.sh
```

Test için uygulamayı veya Kubernetes'i önceden açmanız gerekmez. Chrome kurulu olmalıdır. Sonuçlar terminale yazılır; testler başarılıysa sonda `E2E başarılı` mesajı görünür.

- API doğrulama, CRUD, 404, çakışma ve çapraz kaynak istek kontrolü.
- SQLite kapatılıp açıldığında kalıcılık.
- Sekiz eşzamanlı güncellemeden yalnızca birinin kabul edilmesi.
- Go race detector ve vet.
- chromedp: gerçek Chrome ile oluşturma, yenileme, düzenleme, durum değiştirme, filtreleme ve silme.
- 1440 px masaüstü ve 390 px mobil görünümde yatay taşma kontrolü.

```fish
xdg-open outputs/gorev-desktop.png
and xdg-open outputs/gorev-mobile.png
```

Diğer görseller: `outputs/gorev-dark.png`, `outputs/gorev-list.png`, `outputs/gorev-form.png`.

Ekran görüntülerindeki örnek görevler yalnızca geçici test veritabanında oluşturulur. `.github/workflows/ci.yml` GitHub'a taşındığında Go testlerini ve container derlemesini otomatik çalıştıracak şekilde hazırlandı. Uzak CI henüz çalıştırılmadı.

## Proje yapısı

| Konum | Sorumluluk |
|---|---|
| `cmd/gorev-atolyesi` | Ayarlar, sunucu ve SIGTERM ile düzgün kapanma |
| `internal/tasks/store.go` | SQLite erişimi, doğrulama ve sürüm kontrolü |
| `internal/tasks/schema.sql` | İlk veritabanı şeması |
| `internal/tasks/http.go` | JSON API, hata cevapları, sağlık kontrolleri |
| `internal/tasks/static` | Go binary içine gömülü HTML/CSS/JavaScript |
| `internal/tasks/tasks_test.go` | Entegrasyon ve eşzamanlılık testleri |
| `cmd/gorev-e2e` | İzole chromedp kabul testi |
| `deploy/gorev-atolyesi` | Çok aşamalı Dockerfile ve Kubernetes kaynakları |
| `scripts/proje-*.sh` | Fish içinden `bash` ile çalışan yardımcı komutlar |

Arayüz ve API aynı Go sunucusundan sunulur. Node.js veya ayrı frontend sunucusu gerektirmez. SQL sorguları parametrelidir. Görev metinleri DOM'a `textContent` ile yazılır. API gövdesi 16 KB ile sınırlıdır. Güncelleme ve silmede `version` kontrolü eski sekmelerin yeni verinin üzerine yazmasını önler. Çakışmada panoyu Yenile ile yükleyip işlemi tekrar yapın.

## Ayarlar

| Değişken | Varsayılan | Açıklama |
|---|---|---|
| `ADDR` | `127.0.0.1:8090` | HTTP dinleme adresi |
| `DB_PATH` | `data/gorevler.db` | SQLite dosyası |
| `CHROME_PATH` | PATH üzerinden Chrome/Chromium | Test tarayıcısı |

```fish
env ADDR=127.0.0.1:8091 bash scripts/proje-baslat.sh
```

Yeni bilgisayara araç kurulumu: `bash scripts/kurulum.sh`. Docker ve Chrome ayrıca gereklidir.

## API ve ödev sunumu

[API sözleşmesi](docs/API.md) · [Ödev sunum akışı](docs/SUNUM.md) · [Doğrulama sonuçları](DOGRULAMA.md)

## Veriyi yedekleme

Yerel sunucuyu durdurduktan sonra tüm `data` klasörünü kopyalayın. SQLite WAL modunda çalıştığı için açık veritabanının yalnızca `.db` dosyasını kopyalamayın. Kubernetes PVC'si yedek değildir; küme silinmeden önce ayrı yedek planı gerekir.

## Kaynaklar

[A Tour of Go](https://go.dev/tour/welcome/1) · [chromedp](https://github.com/chromedp/chromedp) · [SQLite sürücüsü](https://pkg.go.dev/modernc.org/sqlite) · [Kubernetes Deployment](https://kubernetes.io/docs/concepts/workloads/controllers/deployment/)

## Tasarım referansları

[Todoist masaüstü çalışma alanı](https://www.todoist.com/features) ve [Linear ürün ekranları](https://linear.app/build) incelendi. Kompakt gezinme, görev odaklı yerleşim, ince ayırıcılar ve sınırlı vurgu rengi kullanıldı. Referans ürünlerin logo veya görselleri uygulamaya kopyalanmadı.

Son tasarım `frontend-design` rehberiyle bir çalışma tezgâhı olarak düzenlendi: sekmeli görev tepsileri, kâğıt mavisi zemin, petrol/kehribar vurgular ve yerel Bricolage Grotesque / Manrope yazı tipleri. Pano, liste, koyu görünüm ve mobil form aynı tasarım dilini kullanır. [Tasarım kararları](docs/DESIGN.md).
