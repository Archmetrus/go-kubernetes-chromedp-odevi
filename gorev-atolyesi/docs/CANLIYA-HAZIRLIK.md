# Hesaplı kullanım ve canlıya hazırlık

Bu sürüm e-posta/parola ile giriş, kullanıcıya ait görevler, PostgreSQL, migration, arşiv, çöp kutusu, önizlemeli JSON içe aktarma ve 5 saniyelik otomatik yenileme ekler. SQLite kişisel geliştirme için desteklenir. Aşağıdaki komutlar fish içindir; `.sh` dosyaları Bash ile çalışır.

## 1. Yerelde yeni özellikleri aç

Önce varsa çalışan yerel sunucuyu kendi terminalinde Ctrl+C ile kapatın. Mevcut verileri yedeklemek için sunucu kapalıyken tüm `data` klasörünü ayrı bir konuma kopyalayın. Yeni şema eski görevleri silmeden sahiplik, arşiv ve silinme alanları ekler.

```fish
cd ~/PROJE/go-kubernetes-chromedp-odevi/gorev-atolyesi
and source scripts/env.fish
and bash scripts/hesap-olustur.sh
```

Script e-posta ve parolayı sorar. Parola terminale yazılmaz, dosyada saklanmaz. Hesabı oluşturduktan sonra mevcut kişisel görevlerinizi o hesaba bağlamak isterseniz bir kez çalıştırın:

```fish
read --prompt-str 'Hesabın e-postası: ' USER_EMAIL
and env USER_EMAIL="$USER_EMAIL" go run ./cmd/gorev-atolyesi claim-local
```

Bu işlem `owner_id=local` olan görevlerin sahipliğini seçtiğiniz hesaba taşır; yeni kayıt veya görev kopyası oluşturmaz. Kayıt açan ilk kullanıcıya eski veriler otomatik verilmez.

Hesaplı sunucuyu açın:

```fish
env AUTH_ENABLED=true ADDR=127.0.0.1:8091 bash scripts/proje-baslat.sh
```

Adres: **http://127.0.0.1:8091**. E-posta/parola ile giriş yapın. Başka hesaplar `hesap-olustur.sh` ile eklenir. Geliştirmede kayıt ekranını denemek için `ALLOW_SIGNUP=true` eklenebilir. Canlı manifestlerde açık kayıt kapalıdır; e-posta doğrulama ve parola sıfırlama e-postası bu sürümde yoktur. İlk canlı kullanım yöneticinin oluşturduğu hesaplarla sınırlıdır.

Eski komut `bash scripts/proje-baslat.sh`, `AUTH_ENABLED` verilmezse kişisel yerel modda çalışır ve yalnızca `local` sahibinin görevlerini gösterir. Hesaba taşınmış görevlerin bu modda görünmemesi normaldir. Yerel mod internete açılmamalıdır.

Hesapsız sunucu varsayılan olarak yalnızca loopback adresine bağlanabilir. Yerel kind YAML'i container ağı için açıkça `ALLOW_LOCAL_NETWORK=true` kullanır; canlı YAML bu istisnayı kullanmaz.

## 2. Uygulamadaki yeni davranışlar

- **Canlı kontrol:** Açık sayfa 5 saniyede bir yeni liste ister. Arka plan sekmesinde, açık formda veya görev durum menüsü odaktayken otomatik yenileme bekler. Formdaki yazılar kaybolmaz. Aynı veriler geldiyse pano yeniden çizilmez. Sekmeye dönünce yeniden kontrol edilir.
- **Arşiv:** Düzenleme formundaki “Arşivde sakla” ile görev aktif panodan ayrılır. “Görev alanı → Arşiv” üzerinden açılıp arşiv işareti kaldırılabilir.
- **Çöp kutusu:** Silme kayıt satırını kaldırmaz. “Görev alanı → Çöp kutusu → Geri al” ile geri döner. Arşivli görev geri alınırsa arşive döner. Otomatik kalıcı silme yoktur.
- **JSON içe aktarma:** En fazla 4 MiB ve 1000 görev; önce sunucu doğrular ve önizleme gösterir, sonra kullanıcı onayıyla tek transaction içinde ekler. Geçersiz dosyada kısmi kayıt yapılmaz. Yeni kimlik ve sürüm verilir; tekrar içe aktarma kopya üretir.
- **JSON dışa aktarma:** Yalnızca oturumdaki kişinin aktif ve arşivlenmiş görevleri döner. Çöp kutusu dahil değildir. Bu dosya hesap/parola/oturum yedeği değildir; tüm veritabanı yedeği ayrıca alınır.

## 3. DBeaver ve verinin yeri

Veritabanını port belirlemez; sunucunun `DATABASE_URL` veya `DB_PATH` ayarı belirler. Bu rehberde yerel uygulama 8091, kind port yönlendirmesi 8090 kullanılır. Yerelde varsayılan dosya `data/gorevler.db`; eski kind uygulamasında PVC içindeki `/data/gorevler.db`; PostgreSQL modunda `DATABASE_URL` hedefidir.

DBeaver değişikliğini önce Save/Commit ile tamamlayın ve aynı veritabanına bağlı olduğunuzu doğrulayın. WAL modunda SQL istemcisiyle commit edilmiş değişikliği görmek için Go sunucusunu kapatmak gerekmez. Güncel sürüm açık form yokken değişikliği sonraki otomatik kontrolde alır. Kullanıcı hesabı açıksa görev aynı `owner_id` değerine ait olmalıdır.

Elle görev güncellerken uygulamanın eski sekme korumasını sürdürmek için `version=version+1` ve `updated_at` alanını da güncelleyin. Önceki konuşmadaki “version değiştirmeyin” önerisi doğru değildi; dışarıdan yazma da bir sürüm değişikliğidir. Yeni kayıt `owner_id` verilmeden eklenirse varsayılan `local` sahibine gider ve hesaplı kullanıcıya görünmez.

## 4. PostgreSQL'e geçiş

Yönetilen PostgreSQL bağlantısını sağlayıcıdan alın. Staging ve production için ayrı veritabanı/kullanıcı kullanın. TLS için `sslmode=verify-full` ve sağlayıcının CA ayarlarını kullanın; URL/parolayı kaynak dosyasına yazmayın.

```fish
cd ~/PROJE/go-kubernetes-chromedp-odevi/gorev-atolyesi
and source scripts/env.fish
and read --silent --prompt-str 'PostgreSQL URL: ' DATABASE_URL
and set -gx DATABASE_URL "$DATABASE_URL"
and go run ./cmd/gorev-atolyesi migrate
and bash scripts/hesap-olustur.sh
```

Migration, `schema_migrations` tablosunu kullanır; PostgreSQL advisory lock eşzamanlı migration işlerini sıraya alır. Go sunucusu PostgreSQL şemasını kendiliğinden değiştirmez; önce `migrate` gerekir. SQLite açılışta eklemeli migration uygular.

Eski SQLite görevlerini yeni, henüz görevi olmayan PostgreSQL hesabına kopyalamak için:

```fish
read --prompt-str 'Hedef hesabın e-postası: ' USER_EMAIL
and env USER_EMAIL="$USER_EMAIL" SOURCE_SQLITE="$PWD/data/gorevler.db" go run ./cmd/gorev-atolyesi import-sqlite
```

Kaynak salt okunur açılır, commit edilmiş WAL verileri de okunur. Kaynak değiştirilmez. Yalnızca eski/`local` sahibinin silinmemiş görevleri alınır; başka kullanıcıların kayıtları tek hesaba toplanmaz. Hedef hesapta görev varsa tekrar taşıma reddedilir. En fazla 1000 kayıt taşınır. Hesaplara zaten ayrılmış veriyi taşımak için her hesap kendi JSON dosyasını yeni hesabına aktarabilir.

PostgreSQL'i yerel HTTP ile denemek için aynı terminalde:

```fish
env AUTH_ENABLED=true ADDR=127.0.0.1:8091 bash scripts/proje-baslat.sh
```

SQLite'a geri dönmek için önce sunucuyu durdurup `set -e DATABASE_URL` çalıştırın. Bu sadece bağlantıyı değiştirir; verileri geri kopyalamaz.

## 5. Oturumlar ve korumalar

Parolalar bcrypt cost 12 ile hashlenir; minimum 12 karakter, maksimum 72 bayt kabul edilir. 32 rastgele bayttan türeyen oturum kimliği tarayıcıda HttpOnly/SameSite=Strict cookie'de, yalnızca SHA-256 özeti veritabanında tutulur. Oturum süresi 12 saattir. Production cookie `__Host-atolye`, Secure ve Path=/ kullanır. Çıkış veritabanındaki oturumu siler; diğer replikalar da çıkışı görür.

Görev CRUD, arşiv, çöp kutusu, geri alma, dışa/içe aktarma sorguları kullanıcı kimliğiyle sınırlandırılır. Başka kullanıcının kimliğiyle yapılan istek 404 döner. Anonim görev API isteği 401 döner. Cookie tabanlı yazma istekleri tam origin kontrolü ve Go çapraz kaynak korumasından geçer; CORS ile herkese açılmaz.

Ortak veritabanındaki hız sınırı replikalar arasında paylaşılır: kullanıcı başına API 300/dakika, giriş/kayıt/çıkış için bağlantı IP'si başına 20/dakika, e-posta başına giriş 10/15 dakika. Proxy başlıklarına güvenilmez; reverse proxy arkasında IP sınırı proxy için ortak olabilir. Küçük davetli kullanım varsayımı budur; büyük kullanıcı kitlesinde ingress üzerinde güvenilir istemci IP sınırlaması ayrıca yapılandırılmalıdır.

Production/staging başlatması PostgreSQL, `AUTH_ENABLED=true`, HTTPS `APP_ORIGIN`, `APP_VERSION` ve `METRICS_TOKEN` olmadan reddedilir. Uygulama TLS'i ingress'te sonlandırır. HTTPS yönlendirmesi Traefik entrypoint ayarıdır; yalnızca uygulamanın güvenlik başlığı HTTP'yi HTTPS'e yönlendirmez.

## 6. Yayın dosyaları ve sıralama

`deploy/live` yerel kind YAML'inden ayrıdır. İki replika, RollingUpdate, PDB, Secret bağlantısı, migration Job, TLS Ingress ve günlük yedek CronJob şablonları içerir. `scripts/canli-hazirla.sh` yalnızca incelemeye uygun YAML üretir. Gerçek yayın için alan adı, erişilebilir küme, Traefik, geçerli sertifika, registry ve yönetilen PostgreSQL hesabı gerekir; bu kaynaklar otomatik satın alınmaz/oluşturulmaz.

Küme yöneticisinin bir kere hazırlaması gerekenler:

1. `gorev-staging` ve `gorev-production` namespace'leri; her ortamda ayrı veritabanı.
2. `gorev-secrets` Secret'ında `DATABASE_URL` ve rastgele `METRICS_TOKEN`. Secret değerlerini Git'e koymayın; platform secret yöneticisiyle oluşturun. `APP_ORIGIN` gibi gizli olmayan bilgiler ConfigMap'tedir.
3. Traefik ingress class `traefik`, web→websecure yönlendirmesi (`deploy/live/traefik-values.yaml`), alan adına ait `gorev-tls` TLS Secret'ı ve DNS kaydı. TLS Secret'ını cert-manager veya sağlayıcının sertifika sistemi yenilemelidir.
4. Registry imajına erişim; özel registry kullanılırsa namespace'in imagePullSecret/service account ayarı.
5. Yedek için `gorev-backup` service account'una bucket yazma yetkili workload identity; aynı adlı ConfigMap'te `BACKUP_S3_URI=s3://bucket/ortam`. Bucket şifreleme, sürümleme ve 30 günlük saklama politikası sağlayıcı tarafında yapılandırılmalıdır.

Uygulama imajını `deploy/gorev-atolyesi/Dockerfile`, yedek imajını `deploy/live/backup.Dockerfile` ile derleyin. `.github/workflows/release.yml` testlerden sonra GHCR'a commit etiketiyle ikisini yayımlamak için hazırdır. Manuel çalışır; `release` GitHub Environment koruması ayarlanmalıdır. Deployment için tercihen değişmez `@sha256` imaj referansı kullanın.

Fish örneği (örnek alan adı/imajları gerçek değerlerle değiştirin):

```fish
set -gx DEPLOY_ENV staging
set -gx DEPLOY_CONTEXT hedef-kume
set -gx APP_DOMAIN atolye.example.com
set -gx APP_IMAGE ghcr.io/hesap/gorev-atolyesi:v2
set -gx BACKUP_IMAGE ghcr.io/hesap/gorev-atolyesi-backup:v2
set -gx RELEASE_VERSION v2-001
bash scripts/canli-hazirla.sh
```

Üretilen dosyaları `deploy/rendered/staging` altında inceleyin. Altyapı ve Secret'lar hazır olduğunda:

```fish
kubectl --context "$DEPLOY_CONTEXT" -n gorev-staging apply -f deploy/rendered/staging/backup.yaml
and bash scripts/canli-dagit.sh
```

Dağıtım scripti migration Job'unun bitmesini bekler, sonra uygulamayı günceller, rollout'u bekler ve HTTPS `/readyz` kontrolü yapar. Production için ortam ve alan adı değerlerini production karşılıklarına çevirin, manifestleri yeniden üretin ve backup CronJob'unu önce uygulayın. Production dağıtımı migration'dan önce bu CronJob'dan bir yedek işi başlatır ve başarılı bitmesini bekler. Her denemede benzersiz `RELEASE_VERSION` kullanın; başarılı bir migration Job'unu başka imaj için yeniden kullanmayın.

GitHub `.github/workflows/deploy.yml` aynı sırayı çalıştırır. `staging`/`production` Environment içinde `KUBECONFIG_BASE64` secret'ı, `DEPLOY_CONTEXT` ve `APP_DOMAIN` değişkenleri gerekir. Production Environment'a zorunlu reviewer ekleyin. Workflow önceden test edilmiş imaj referansı ister; henüz gerçek bir repository/ortama bağlanıp çalıştırılmadı.

Bu klasör bağımsız repository yapılırsa workflow'lar doğrudan kökte çalışır. Üst ödev klasörü repository olursa workflow dosyalarını repository kökündeki `.github/workflows` içine taşıyıp `APP_DIRECTORY=gorev-atolyesi` repository değişkenini ayarlayın. Sadece alt klasörde duran workflow GitHub tarafından bulunmaz.

## 7. Yedek, geri yükleme ve geri dönüş

`postgres-yedek.sh` PostgreSQL istemci araçları (`pg_dump`, `pg_restore`) gerektirir. Araç ana sürümü sunucudan eski olmamalıdır. Canlı veritabanından tutarlı custom-format yedek alır. `BACKUP_S3_URI` varsa AWS CLI ile bucket'a gönderir; CronJob her gün 02:00 UTC'de çalışır. Yedek başarısızsa job başarısız olur; dosyanın sadece oluşması başarı sayılmaz.

```fish
bash scripts/postgres-yedek.sh
```

Geri yükleme scripti yalnızca ayrı ve boş hedefte çalışır, dolu veritabanının üzerine yazmaz. Önce sağlayıcıda boş bir doğrulama veritabanı oluşturun, yedeği indirin ve:

```fish
read --silent --prompt-str 'Boş geri yükleme hedefi URL: ' RESTORE_DATABASE_URL
and set -gx RESTORE_DATABASE_URL "$RESTORE_DATABASE_URL"
and bash scripts/postgres-geri-yukle.sh backups/yedek-dosyasi
```

Script transaction içinde geri yükler; görev sayısını ve şema sürümünü gösterir. Ardından bu hedefe bağlanan izole uygulamada giriş/JSON indirme deneyin. Üretim bağlantısını otomatik değiştirmez. Ayda bir geri yükleme tatbikatı yapın; günlük yedek varsayımıyla en fazla yaklaşık 24 saatlik veri kaybı hedeflenir, daha kısa hedef için sağlayıcı PITR açılmalıdır.

Uygulama güncellemesinde sorun çıkarsa, aynı şemayla uyumlu son hesaplı production sürümüne `kubectl --context ... -n gorev-production rollout undo deployment/gorev-atolyesi` ve ardından `rollout status` ile dönün. Eski hesapsız v1 binary'sine dönmeyin: kullanıcı izolasyonunu uygulamaz. Migration otomatik geri alınmaz. Veri sorunu varsa önce ayrı veritabanına yedek geri yüklenir, doğrulanır ve kontrollü bağlantı değişimi yapılır.

## 8. İzleme ve kabul kontrolleri

JSON loglarda istek kimliği, HTTP durum kodu, süre ve sürüm bulunur; parola/gövde/oturum kimliği loglanmaz. `/healthz` sürümü de gösterir. `/metrics` Bearer `METRICS_TOKEN` ile korunur; istek, sunucu hatası ve toplam süre sayaçları vardır. Prometheus her Pod'u ayrı hedef olarak scrape etmelidir. Token'ı secret dosyasından okuyacak şekilde `authorization.credentials_file` kullanın.

`deploy/live/alerts.yaml` örnek Prometheus kurallarıdır. Metrik scrape'i, kube-state-metrics ve Alertmanager alıcıları mevcut izleme sistemine bağlanmalıdır. Hata logları mevcut merkezi log sistemine gönderilebilir; harici hata izleme hesabı otomatik kurulmadı.

```fish
cd ~/PROJE/go-kubernetes-chromedp-odevi/gorev-atolyesi
and bash scripts/proje-kontrol.sh
```

PostgreSQL testleri için yalnızca ayrı test veritabanının URL'sini kullanın; testler kullanıcı/görev oluşturur:

```fish
source scripts/env.fish
and read --silent --prompt-str 'Test veritabanı URL: ' TEST_DATABASE_URL
and set -gx TEST_DATABASE_URL "$TEST_DATABASE_URL"
and go test -race ./...
and go vet ./...
```

Kabul kapsamı: eski SQLite şemasını koruma, dış SQL commit'ini okuma, kullanıcılar arasında CRUD/export/restore izolasyonu, paylaşılan oturum, logout/expiry, CSRF, hız sınırı, atomik import, sürüm çakışması; Chrome'da kayıt/giriş, otomatik yenileme, arşiv/geri alma ve gerçek dosya içe/dışa aktarma. Staging'de ayrıca gerçek TLS sertifikası/yönlendirme, iki Pod, registry çekimi, yedek bucket'ı, uyarılar ve rollout geri dönüşü kontrol edilmeden production yayını tamamlanmış sayılmaz.

Teknik kaynaklar: [bcrypt](https://pkg.go.dev/golang.org/x/crypto/bcrypt), [OWASP oturum yönetimi](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html), [pg_dump](https://www.postgresql.org/docs/current/app-pgdump.html), [Traefik entrypoint yönlendirmesi](https://doc.traefik.io/traefik/reference/install-configuration/entrypoints/).
