# Toplu görev atama ve otomasyon API'si

11 Eylül 2026 — migration 7. Web, terminal ve harici otomasyon aynı önizleme/onay API'sini kullanır. Görevler mevcut grup/lider/atama yetkileriyle oluşturulur. Bu adım canlı yayın engelini kaldırmaz.

## Web kullanımı

Demo kodunu güncelle ve aç:

```fish
cd (git rev-parse --show-toplevel)/gorev-atolyesi
and bash scripts/ekip-demo.sh restart
and bash scripts/ekip-demo.sh accounts
```

http://127.0.0.1:8092 adresinde yönetici veya atama yetkili hesapla giriş yap. **Ekip ağacı ve görevler → Dosyadan görev ata** seç.

1. JSON veya `.xlsx` dosyasını seç. **Grubuma uygun JSON örneği indir** ile gerçek grup ve sorumlu bilgileriyle örnek alabilirsin.
2. **Dosyayı kontrol et** ile her satırın başlığını, sorumlusunu ve hatasını gör. Bu adım görev oluşturmaz.
3. Hatalı satır varsa dosyayı düzelt ve tekrar önizle. Bütün dosya geçerli olmadan onay açılamaz.
4. Onay kutusunu işaretleyip **Görevleri ata** seç. Bütün görevler ve geçmiş kayıtları tek transaction içinde oluşturulur.
5. **İşlem sonuçları** sekmesinde iş numarası, görev kimlikleri ve sorumlularını gör. Ekip panosu açıkken 5 saniyelik yenilemeyle işler görünür.

Önizleme 15 dakika geçerlidir. Onay anında hesap, anahtar, grup yetkisi ve sorumlunun üyeliği yeniden kontrol edilir. Bir koşul değişmişse bütün işlem reddedilir. Satır hataları önizlemede gösterilir; başarılı atamalar kalıcı işlem kaydına yazılır. Kısmi atama yapılmaz.

## Dosya biçimi

JSON, `tasks` dizisini içeren bir nesnedir. Çalışan demo örneği: `examples/toplu-atama-demo.json`.

```json
{"tasks":[{"team_id":"demo-team-1","assignee_email":"go-ece@atolye.example.test","title":"API kabul testini tamamla","category":"Go","priority":"medium","status":"todo","due_date":""}]}
```

XLSX'te tek çalışma sayfası ve ilk satırda şu sütun başlıklarını kullan:

| Başlık | Kural |
|---|---|
| `team_id` | Zorunlu; grubun API kimliği |
| `assignee_email` | Zorunlu; grubun aktif ve doğrulanmış üyesi |
| `title` | Zorunlu; 3–120 karakter |
| `description` | İsteğe bağlı; en fazla 2000 karakter |
| `category` | Go / Kubernetes / chromedp / Genel; boşsa Genel |
| `priority` | low / medium / high; boşsa medium |
| `status` | todo / doing / done; boşsa todo |
| `due_date` | YYYY-MM-DD **metni** veya boş |

Sütunlar A–H içinde olmalı. İsteğe bağlı sütunlar tamamen atlanabilir. **Eski `.xls`, formül, birleştirilmiş hücre, makro, dış bağlantı ve birden fazla sayfa desteklenmez.** Excel'in sayısal tarih seri numarası yerine tarih sütununu Metin olarak biçimlendir. `.xlsx` okuması ortak metin tablosunu ve hücre içi metni destekler; formül sonucu hesaplamaz. [Microsoft SpreadsheetML açıklaması](https://learn.microsoft.com/en-us/office/open-xml/spreadsheet/working-with-sheets).

En fazla 200 görev / 2 MiB yükleme. ZIP içinde en fazla 128 parça, toplam 16 MiB açılmış içerik ve parça başına 8 MiB sınırı vardır. Dosya sunucu diskine açılmaz. Bilinmeyen JSON alanları ve XLSX başlıkları reddedilir.

## Erişim anahtarı

**Erişim anahtarları** sekmesinde grup, açıklayıcı ad, 1–30 gün ve mevcut parolanı gir. Son 5 dakika içinde yapılmış MFA gerekir; atama yetkili üyeler de anahtar oluşturmak için MFA kurmalıdır. Kullanıcı başına en fazla 20 etkin anahtar tutulur.

Anahtar yalnızca bir kez gösterilir. DB'de SHA-256 hash'i bulunur. Web'de anahtar listesi ham değeri göstermez. Anahtar **tek grup** için önizleme, onay ve kendi işlem sonuçlarını okuma yetkisi verir. Kullanıcı/grup yönetimi, kişisel görev API'si veya yeni anahtar oluşturma yetkisi vermez.

Anahtar sahibi aktif, e-postası doğrulanmış ve MFA etkin olmalıdır. Hesap sürümü değişimi/MFA sıfırlama anahtarı geçersiz kılar. Üyelik veya atama izni kaldırıldığında ve liderlik devredildiğinde ilgili anahtarlar iptal edilir. Süre dolması/iptal her istekte ve atama transaction'ında yeniden kontrol edilir. Web'deki **Anahtarı iptal et** işlemi sonraki kullanımları engeller.

## Terminal: anahtar saklama, önizleme, onay

Anahtarı argümana, shell geçmişine veya görev JSON'una yazma. Demo için:

```fish
bash scripts/ekip-demo.sh assign setup
```

Gizli isteme web'deki anahtarı yapıştır. `.control/demo/automation.token` dosyası `0600` izinle oluşturulur. Var olan dosya sessizce ezilmez. Kaybolan anahtar web'den yeniden gösterilmez; eskisini iptal edip yeni anahtar oluştur.

Önce önizle ve çıktıyı incele:

```fish
bash scripts/ekip-demo.sh assign preview --file examples/toplu-atama-demo.json
```

İnceledikten sonra ayrı olarak onayla:

```fish
bash scripts/ekip-demo.sh assign commit
and bash scripts/ekip-demo.sh assign results
```

Aynı `commit` tekrar çalıştırıldığında aynı iş sonucu döner, görev çoğalmaz. Yeni `preview` yeni bir atama işlemi başlatır; aynı dosyayı bilinçli olarak yeniden önizleyip onaylamak yeni görevler oluşturur. Başarısız önizleme eski yerel onay dosyasını geçersiz kılar.

Genel sunucu için:

```fish
bash scripts/atolye.sh assign setup --token-file .control/automation.token
bash scripts/atolye.sh assign preview --url https://atolye.example.com --file gorevler.xlsx --preview-file .control/assignment-preview.json
```

Onay/sonuç komutlarına da aynı `--url`, `--token-file`, `--preview-file` değerlerini ver. HTTPS zorunludur; yalnızca loopback adreslerinde HTTP kabul edilir. İstemci yönlendirme takip etmez, anahtar dosyasında symlink ve açık dosya izinlerini reddeder. TLS doğrulaması kapatılmaz. Çıkış kodları: başarılı `0`, dosyada satır hataları `2`, bağlantı/yetki/diğer hata `1`.

## Oklarla terminal menüsü

Dosyalarını `data/imports/` klasörüne koy; anahtar kurulumunu yukarıdaki `assign setup` ile yap. Demo için `bash scripts/ekip-demo.sh tui` aç.

- **Dosyadan görev / önizle:** Dosyayı oklarla seç, Enter ile doğrulat; sonuçları PgUp/PgDn ile incele.
- Ana menüye dön, **Son önizlemeyi onayla / ata:** Enter ile işlemi seçip ikinci Enter ile onayla.
- **Toplu atama sonuçları:** Aynı anahtarla yapılmış sonuçları görüntüle.

Panelde parolayla otomatik giriş veya DB üzerinden atama yoktur; aynı API istemcisi çalışır. Genel panel `ATOLYE_API_URL` ve `ATOLYE_API_TOKEN_FILE` ortam değişkenlerini de destekler.

## Robomotion / harici HTTP akışı

1. Anahtarı otomasyon aracının gizli değer deposuna kaydet. Her istekte `Authorization: Bearer <anahtar>` başlığını kullan; URL'ye ekleme.
2. `POST /api/automation/imports`: Gövde JSON dosyası veya XLSX'in ham baytlarıdır (multipart değil). Content-Type JSON için `application/json`, XLSX için `application/vnd.openxmlformats-officedocument.spreadsheetml.sheet`.
3. Yanıttaki `ready`, `rows[].error`, `id`, `expires_at` değerlerini kontrol et. `ready=false` ise dur; görev oluşmamıştır.
4. Bir UUID üret ve kalıcı iş kaydında sakla. `POST /api/automation/imports/{id}/commit`, JSON gövde `{}`, `Idempotency-Key: <UUID>` başlığıyla onayla.
5. Ağ hatasında **aynı önizleme ID'si ve aynı Idempotency-Key** ile tekrar dene. Başka önizleme için aynı anahtarı veya işlenmiş önizleme için başka anahtarı kullanmak `409` verir.
6. `GET /api/automation/results` aynı anahtarın son 50 başarılı işini gösterir. Web oturumu ve her anahtar ayrı işlem kapsamıdır. Anahtar başka anahtarın/oturumun önizlemesini onaylayamaz.

Geçersiz/iptal edilmiş anahtar `401`; yetersiz yetki `403`; sürüm/istek çakışması `409`; boyut aşımı `413`; hız sınırı `429`. Dosya biçimi hatası `400`; satır hataları `200` içinde `ready=false` ve açıklamalarla döner. Önizleme 20/dakika, bearer erişimi IP başına 120/dakika ve anahtar oluşturma 5/dakika ile sınırlıdır.

## Doğrulama ve kalan sınırlar

Testler: XLSX ortak/hücre içi metin, formül/makro/dış bağlantı ve boyut kuralları; anahtar hash/izin/kapsam/iptal/MFA; oturumlar arası izolasyon; eşzamanlı aynı anahtarla tek atama; geçmiş kaydı hatasında rollback; süre aşımı; web önizleme/onay; gerçek terminal preview/commit/results ve tekrar deneme.

Testler geçici SQLite DB ve geçici sunucu kullanır. Gerçek PostgreSQL entegrasyonu henüz çalıştırılmadı. Robomotion uygulamasına bağlanılmadı veya mesaj gönderilmedi; entegrasyon HTTP sözleşmesi ve gerçek CLI istemcisiyle doğrulandı. SMTP, parola sıfırlama, grup mesajlaşması ve dosya paylaşımı bu teslimin dışındadır.
