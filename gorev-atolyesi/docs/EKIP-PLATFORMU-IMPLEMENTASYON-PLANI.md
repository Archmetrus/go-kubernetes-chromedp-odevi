# Görev Atölyesi: güvenli ekip platformu uygulama planı

Güncel teslim (11 Eylül 2026): toplu JSON/XLSX atama, grup kapsamlı süreli API anahtarları, idempotency, web/terminal/TUI önizleme-onay-sonuç akışları migration 7 ile uygulandı. [Toplu atama rehberi](TOPLU-ATAMA-VE-OTOMASYON.md). Robomotion HTTP sözleşmesi hazır; gerçek uygulama bağlantısı yapılmadı. Geniş hedef planının bütün aşamaları tamamlanmış değildir.

Tarih: 10 Eylül 2026. Durum: uygulama öncesi teknik plan; bu belgedeki yeni özellikler henüz geliştirilmedi. Mevcut kaynak kod, veritabanı şeması, oturum/ithalat uygulaması ve tasarım rehberi incelenerek hazırlandı.

Güncelleme (11 Eylül 2026): terminal işletim arayüzü ve kimlik aşamasının kayıt/hesap onayı/ilk yönetici parçası uygulandı. E-posta kod doğrulama akışı geliştirme posta kutusuyla eklendi; gerçek e-posta gönderimi değildir. MFA/TOTP, kurtarma kodları ve CLI/TUI operatör sıfırlaması migration 5 ile uygulandı. Gerçek komut `bash scripts/atolye.sh control tui`; kontrol katmanı Python curses, sunucu Go'dur. Aşağıdaki bölümler hedef kapsamı gösterir; mevcut sürümün sınırları [Terminal kontrol rehberinde](TERMINAL-KONTROL.md), [hesap onayı rehberinde](HESAP-ONAYI.md), [MFA rehberinde](MFA-VE-KURTARMA.md) ve [ilerleme kaydında](IMPLEMENTATION-STATUS.md) açıklanır. Gerçek SMTP teslimatı, parola sıfırlama ve sonraki ekip özellikleri henüz tamamlanmadı.

Öncelik güncellemesi (11 Eylül): kullanıcı demo hiyerarşisi ve görev atamayı SMTP öncesine aldı. Migration 6 ile grup/lider/üyelik ve ekip görevleri uygulandı; SQLite üzerinde ayrı, gerçek kayıtlı demo hazırlandı. Canlı ekip dağıtımında PostgreSQL hedefi korunur. Uygulanan kapsam ve komutlar: [Ekip ağacı demo rehberi](EKIP-AGACI-DEMO.md). Aşağıdaki tarihsel mevcut-durum ve hedef bölümleri tamamlanma beyanı değildir.

## 1. Amaç ve karara bağlanan varsayımlar

Kullanıcı kendi hesabını web arayüzünden açacak. Konsol yalnızca ilk ana yöneticiyi belirlemek, acil kurtarma ve sistem işletimi için gerekecek. Ana yönetici tüm grupları görecek; grup lideri kendi grubunun yöneticisi olacak ve kendisine de görev atanabilecek. Yetkilendirilmiş kişiler görevleri web, terminal veya harici otomasyondan aynı API üzerinden atayacak. Grup içinde mesaj ve dosya paylaşımı bulunacak.

İlk iki madde henüz kullanıcı tarafından onaylanmamış **varsayımlardır**. Üçüncü madde kullanıcının açık tercihiyle güncellenmiştir:

1. Kayıt: e-posta doğrulaması + ana yönetici hesap onayı; grup üyeliği ayrıca lider/ana yönetici onayına bağlı.
2. Üye: kendi görevlerini görür. Başkasına atama için grup liderinin verdiği ek yetki gerekir; bu yetkiyle oluşturduğu görevleri de takip edebilir. Grubun bütün görevlerini otomatik göremez.
3. İşletim arayüzü: terminal içinde ok tuşları ve Enter ile kullanılan etkileşimli TUI. TUI ve doğrudan CLI komutları aynı işletim katmanını kullanır.

Diğer başlangıç kararları:

- Tek kurum/kurulum, birden fazla grup. Bir kişi birden fazla gruba üye olabilir; rolü her grupta farklı olabilir.
- Her aktif grubun tam bir lideri vardır. Lider değişimi atomik yapılır; grup lidersiz bırakılamaz.
- Ana yönetici görev, grup mesajı ve dosyaları görebilir; parola, token, MFA sırrı veya silinmiş içeriğin erişilebilir olmayan eski sürümlerini arayüzden göremez.
- Grup lideri bir başka grubun veya üyenin grup dışındaki görevlerini göremez. Global hesap kapatma yetkisi yoktur; yalnızca kendi grubundaki üyeliği kaldırabilir.
- Görevde bir sorumlu bulunur. Aynı işi üç kişiye dağıtmak, üç ayrı görev oluşturur. Çoklu sorumlu bu sürümde yoktur.
- İlk iletişim kapsamı grup kanalları ve görev yorumlarıdır. Özel mesaj, ses/video, herkese açık dosya bağlantısı ve dış webhook gönderimi sonraki sürüme bırakılır.
- Ekip modu PostgreSQL gerektirir. SQLite eski kişisel çalışma ve veri taşıma için korunur; ekip özelliklerinin ikinci bir SQLite uygulaması yazılmaz.
- Tasarım, mevcut çalışma tezgâhının devamıdır. Yeni bir tema veya bağımsız bir görsel ürün oluşturulmaz.

## 2. Mevcut durum ve değişmesi gereken temel

Bugün e-posta/parola kaydı, cookie oturumu, kullanıcıya ait görevler, PostgreSQL desteği, şema sürümü 2, arşiv/çöp kutusu, JSON import/export, 5 saniyelik yenileme ve yayın betikleri var.

Bugünkü `owner_id` kontrolü bir ekip hiyerarşisini ifade etmiyor. `owner(ctx)` kimlik yoksa `local` döndürüyor; ekip modunda bu davranış kullanılamaz. Grup/atama ile mevcut kişisel sahiplik birbirine karıştırılmadan dönüştürülmeli. Cookie dışında otomasyon kimlik doğrulaması, MFA, e-posta doğrulaması, üyelik, mesaj, dosya güvenliği ve denetim kayıtları yeni çalışmalardır.

`internal/tasks` sorumlulukları daraltılarak `identity`, `authorization`, `groups`, `tasks`, `imports`, `messaging`, `files`, `audit`, `jobs` modüllerine ayrılacak. Bu modüller aynı Go uygulamasında kalacak. Ayrı servisler yerine başlangıçta API süreci, arka plan çalışanı ve yerel işletim aracı yeterlidir.

## 3. Hiyerarşi ve yetki matrisi

```text
Ana yönetici
├── Go Ekibi — grup lideri
│   ├── üye
│   └── görev atama yetkili üye
├── Platform Ekibi — grup lideri
│   └── üyeler
└── Kalite Ekibi — grup lideri
    └── üyeler

Otomasyon kimliği → belirli kullanıcı/servis hesabı + izinler + belirli gruplar
Sistem operatörü → yerel işletim aracı; uygulama rolleriyle aynı yetki değildir
```

| İşlem | Üye | Atama yetkili üye | Grup lideri | Ana yönetici |
|---|---|---|---|---|
| Kendi görevini görme/durum değiştirme | Evet | Evet | Evet | Evet |
| Başkasına görev atama | Hayır | İzinli grubunda | Kendi grubunda | Tüm gruplarda |
| Diğer kişinin görevini görme | Hayır | Kendisinin oluşturdukları | Kendi grubundakiler | Tümü |
| Başlık, açıklama, öncelik ve tarih değiştirme | Kendi oluşturduğu ve kendisine ait görev | Kendi oluşturdukları | Kendi grubunda | Tümü |
| Üyelik onaylama/kaldırma | Hayır | Hayır | Kendi grubunda | Tüm gruplarda |
| Atama yetkisi verme | Hayır | Hayır | Kendi grubunun üyelerine | Tüm gruplarda |
| Grup oluşturma/lider belirleme | Hayır | Hayır | Hayır | Evet |
| Global hesap onayı/askıya alma | Hayır | Hayır | Hayır | Evet |
| Grup mesajı/dosyası | Üye olduğu gruplar | Üye olduğu gruplar | Kendi grubu | Tüm gruplar |
| Altyapıyı başlatma/durdurma | Hayır | Hayır | Hayır | Sadece ayrıca OS operatörü ise |

Üye, kendisine atanmış başkasının görevinde durum ve yorum günceller; atayanı, sorumluyu, grubu veya kapsamı değiştiremez. Görev silme/arşivleme, başlık değiştirme yetkisiyle aynı kapsama bağlıdır. Gruptan çıkarılan kişi, geçmiş mesajlara ve dosyalara da erişimini kaybeder. Kendi oluşturduğu atamalar üzerindeki erişimi de grup üyeliğine bağlıdır.

Görevin atandığı kişi gruptan çıkarılırken açık görevler liderine aktarılır; işlem öncesi sayı/önizleme gösterilir ve tek transaction ile tamamlanır. Tamamlanmış görevlerin tarihsel sorumlusu korunur. Lider devri yeni liderin aktif, e-postası doğrulanmış, MFA kurulumu tamamlanmış bir grup üyesi olduğunu doğrular, eski lideri üyeye indirir ve iki işlemi birlikte kaydeder. İlk grup oluşturulurken de lider adayı bu koşulları sağlamalıdır. Global hesap kapatmada tüm gruplardaki açık görevler aynı kurala göre devredilir; lider olan kişi için önce lider değişimi zorunludur.

Ana yönetici de kişisel görevler alabilir; grup görevi almak için ilgili grubun üyesi olur. Sistemde en az bir aktif ana yönetici kalmalıdır. İkinci ana yönetici ataması otomasyon token'ıyla yapılamaz; mevcut yöneticinin yeniden MFA doğrulaması gerekir.

Yetki denetimi rol + üyelik + kaynak ilişkisi + verilmiş özel izinlerden hesaplanacak; arayüzde buton gizlemek yetki denetimi sayılmayacak. Liste, sayaç, arama, export, dosya indirme ve canlı olaylar aynı kurallara tabi olacak. [OWASP Authorization](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html)

## 4. Güvenli kayıt ve ilk yönetici

Normal kayıt akışı:

```text
Kayıt formu → e-posta doğrulama → yönetici onayı bekliyor
           → aktif hesap → grup daveti/üyelik onayı → çalışma alanı
```

- Register isteği yalnızca ad, e-posta, parola ve gerekli kabul alanlarını alır. `role`, `is_admin`, `approved`, `group_id`, `permissions` gibi alanlar reddedilir.
- Kayıt ve parola sıfırlama cevapları hesap varlığını açıklamaz. Var olan hesap için ikinci kayıt oluşturulmaz; mevcut parola veya durum değiştirilmez.
- Doğrulama token'ı tek kullanımlık, hashli saklanan, 24 saat geçerli bir rastgele değerdir. Yeniden gönderme eski kullanılmamış token'ı iptal eder. Parola sıfırlama token'ı 30 dakika geçerlidir.
- E-posta linki trusted `APP_ORIGIN` üzerinden üretilir. Mail tarayıcısının GET isteği token'ı tüketmez; kullanıcı sayfasındaki POST onayı tüketir. Token/referrer/gövde loglanmaz.
- Mail teslimatı transaction outbox üzerinden yapılır; yerelde Mailpit, canlıda TLS'li SMTP adaptörü kullanılır. SMTP yoksa canlı açık kayıt başlatılmaz. Test doğrulama linkleri production cevaplarına konulmaz.
- Bekleyen hesap yalnızca kendi durumunu, doğrulama ve çıkış işlemlerini görebilir. Kullanıcı/grup listesine ve görev API'sine erişemez.
- İlk yönetici önce normal formdan kayıt olur ve e-postasını doğrular. OS operatörü `atolye admin bootstrap --email ...` ile bu hesabı ana yönetici yapar. Parola konsola girilmez.
- Bootstrap ilk yönetici zaten varsa reddedilir; eşzamanlı iki bootstrap transaction kilidiyle tek sonuca iner. Genel bir HTTP bootstrap endpoint'i bulunmaz.
- Ana yönetici ve lider için TOTP MFA zorunlu, üyeler için isteğe bağlıdır. Yönetim işlevleri MFA kurulmadan açılmaz. TOTP sırları uygulama dışında yönetilen anahtarla şifrelenir; kurtarma kodları tek kullanımlık ve hashli saklanır.
- Rol değiştirme, token üretme, hesap kapatma ve MFA sıfırlama için son 5 dakika içinde yeniden doğrulama gerekir. Kurtarma, ayrı OS-yetkili komutla ve denetim kaydıyla yapılır.
- Parola sıfırlama ve global askıya alma oturumları ve otomasyon anahtarlarını iptal eder. Rol/üyelik değişiklikleri sonraki her istekte yeniden değerlendirileceğinden eski token daha geniş yetkiyi sürdüremez.
- Yeni parolalar Argon2id ile saklanır; mevcut bcrypt hashleri başarılı girişte yeniden hashlenir. Parametreler hedef sunucuda ölçülür; başlangıç değeri 64 MiB bellek, 3 tur, paralellik 1; eşzamanlı parola işlemleri sınırlandırılır. Parola minimum 12 karakter, maksimum 256 UTF-8 bayt olur.

Bu akışın testleri link tekrar kullanımı, süresi dolmuş link, e-posta değişimi, eşzamanlı tüketim, hesap enumerasyonu, brute force, MFA tekrar kullanımı ve session fixation durumlarını kapsar. [OWASP Password Reset](https://cheatsheetseries.owasp.org/cheatsheets/Forgot_Password_Cheat_Sheet.html), [OWASP MFA](https://cheatsheetseries.owasp.org/cheatsheets/Multifactor_Authentication_Cheat_Sheet.html)

## 5. Veri modeli ve migration

| Model | Temel alanlar / görev |
|---|---|
| users | ad, e-posta, global rol, hesap durumu, verified_at, auth_version, parola hash'i |
| groups | UUID, sabit slug, ad, leader_id, aktif/arşiv durumu, version |
| group_memberships | group_id + user_id benzersiz; üye durumu, izinler, katılım zamanı |
| invitations / verification_tokens | amaç, hedef, süre, hash, tüketilme durumu |
| tasks | mevcut alanlar + group_id nullable, created_by, assignee_id, assigned_by |
| task_events | atama/devir/durum değişikliği geçmişi; eski/yeni sorumlu |
| api_tokens | token özeti, kimlik, grup kısıtları, scope, bitiş/iptal, son kullanım |
| automation_integrations | servis hesabı, sahibi, izinli gruplar, etkinlik durumu |
| import_batches / import_rows | normalize edilmiş satırlar, digest, durum, hata, üretilen görev kimlikleri |
| idempotency_records | kimlik + işlem + anahtar benzersiz; istek hash'i ve cevap referansı |
| channels / messages | grup kanalı, gönderen, içerik, düzenlenme/silinme, sıra kimliği |
| attachments | grup/kaynak, yükleyen, rastgele object key, boyut, hash, tarama durumu |
| notifications | alıcı, tür, kaynak, okunma durumu |
| outbox / jobs | transaction ile yaratılan, yeniden denenebilir arka plan işleri |
| audit_events | aktör, token kimliği, eylem, kaynak, kapsam, sonuç, istek kimliği |

Yabancı anahtarlar, rol/durum CHECK kısıtları ve birleşik üyelik anahtarları DB seviyesinde kurulacak. Aktif grubun lider üyeliği deferred constraint/trigger ile transaction sonunda doğrulanacak. Son ana yönetici koruması eşzamanlı yönetici işlemlerini kilitleyerek uygulanacak.

Mevcut kullanıcı görevleri başlangıçta `group_id=NULL`, created_by/assignee_id mevcut kullanıcı olacak şekilde korunur. Sahibi `local` olan görevler otomatik bir kullanıcıya verilmez; mevcut açık taşıma komutuyla sahip belirlenir. Ana yönetici tüm görevleri görebildiği için bu alanlara kullanıcı arayüzünde “özel/gizli” denmez. Grup lideri bu grup dışı kişisel görevleri göremez.

Yeni grup görevi için grup ve sorumlu zorunludur. Mevcut görev kimlikleri yeniden numaralanmaz. Eski kişisel JSON formatı kişisel import/export olarak korunur; ekip atama dosyası ayrı v2 sözleşmesidir. Legacy endpoint'ler yeni yetki servisinin üzerinden geçer, eski `owner_id` bypass'ı bırakılmaz.

Migration'lar sıralı SQL dosyalarına ayrılır; mevcut 001/002 geçmişi değiştirilmez. Önce ekleme/backfill, sonra yeni kod, son aşamada gereksiz alanların kaldırılması yapılır. Migration ve uygulama DB kullanıcıları ayrılır; uygulama rolü şema değiştiremez. Kapsamlı geri yükleme DB + dosya nesneleri + şifreleme anahtarlarına erişimi birlikte kapsar.

## 6. Ortak görev ve otomasyon API'si

Web, terminal, küçük import GUI'si ve Robomotion aynı `/api/v1` API'sini kullanır. CLI veya robot doğrudan SQL yazmaz. OpenAPI 3.1 dosyası API, JSON şeması, yetki scope'ları ve örnekler için tek sözleşme olur.

| Alan | Endpoint ailesi |
|---|---|
| Kimlik | `/api/v1/auth/register`, `/verify-email`, `/login`, `/logout`, `/password-reset`, `/mfa/*`, `/session` |
| Kullanıcı onayları | `/api/v1/admin/users`, `/{id}/approve`, `/{id}/suspend` |
| Gruplar | `/api/v1/groups`, `/{id}/members`, `/{id}/invitations`, `/{id}/leadership` |
| Görevler | `/api/v1/tasks`, `/{id}`, `/{id}/assignment`, `/{id}/events`, `/{id}/comments` |
| Otomasyon | `/api/v1/tokens`, `/api/v1/integrations` |
| Toplu atama | `/api/v1/assignment-batches`, `/{id}`, `/{id}/commit` |
| İletişim | `/api/v1/groups/{id}/channels`, `/api/v1/channels/{id}/messages` |
| Dosya | `/api/v1/attachments`, `/{id}/complete`, `/{id}/download` |
| Bildirim ve olaylar | `/api/v1/notifications`, `/api/v1/events` |
| Denetim | `/api/v1/admin/audit-events`; lidere yalnızca kendi grubuna ait filtreli kayıtlar |

Tekil atama örneği:

```http
POST /api/v1/tasks
Authorization: Bearer <atolye-token>
Idempotency-Key: robot-run-2026-001-row-1
Content-Type: application/json
```

```json
{
  "group_id": "group-uuid",
  "assignee_id": "user-id",
  "title": "Dağıtım kontrol listesini tamamla",
  "description": "Staging sonuçlarını görev yorumuna ekle.",
  "category": "Kubernetes",
  "priority": "high",
  "due_date": "2026-09-20"
}
```

Sunucu aktörü token/oturumdan bulur, hedef grubun ve sorumlunun aktif üyeliğini doğrular. `created_by`, `assigned_by`, `owner_id` istekten kabul edilmez. Var olan görevin devri `assignment` endpoint'inden version/If-Match kontrolüyle yapılır; sıradan PUT içinde yetkisiz devir mümkün olmaz.

Tarayıcı cookie + CSRF koruması kullanır. Robot/CLI için Bearer token ayrı doğrulanır; Authorization başlığı geçersizse cookie'ye geri düşülmez. Token URL/query içine konulmaz. Yetkisiz kaynak kimlikleri 404; görünür kaynaktaki yetkisiz işlem 403; eski sürüm 409; dosya büyüklüğü 413; validasyon 422; sınır aşımı 429 olur. Listeleme 50 varsayılan / 100 maksimum cursor sayfalama kullanır.

Otomasyon anahtarları:

- `tasks:read`, `tasks:create`, `tasks:assign`, `imports:preview`, `imports:commit` gibi dar kapsamlar. Sohbet ve dosya erişimi görev robotuna varsayılan verilmez.
- Etkin yetki = kimliğin güncel yetkileri ∩ token scope'ları ∩ token grup listesi. Ana yönetici token'ı da otomatik global olmaz.
- Token yalnızca üretildiği anda gösterilir, DB'de hash/prefix tutulur; varsayılan 7 gün, maksimum 30 gün. İptal sonraki istekte etkili olur. Token üretme/scope genişletme token ile yapılamaz.
- Kullanıcı adına anahtar ve uzun süreli entegrasyon için ayrı servis hesabı desteklenir. Servis hesabı interaktif giriş yapamaz; lider yalnızca kendi grubu için oluşturabilir. Sahibi askıya alınırsa entegrasyon durur, yönetici kontrollü devredebilir.
- Yüksek ayrıcalıklı kişisel token için MFA; ana yönetici atama ve üyelik-yönetimi scope'ları ilk otomasyon sürümünde verilmez.
- Token/IP/kullanıcı/grup tabanlı hız ve kota sınırları birlikte uygulanır; trusted proxy CIDR listesi dışında X-Forwarded-For kabul edilmez.

Robomotion örnek akışı Vault → HTTP Request → durum kodu kontrolü → batch takip adımlarını gösterecek. Kullanılacak token Görev Atölyesi token'ıdır; Robomotion'un kendi yönetim API anahtarı değildir. TLS doğrulaması açık tutulur; geçici hata yeniden denemelerinde aynı idempotency key kullanılır. [Robomotion API çağırma rehberi](https://docs.robomotion.io/how-to-guides/flows/api/)

## 7. Excel/JSON toplu atama

Desteklenen dosyalar `.xlsx` ve `.json`; `.xls`, `.xlsm`, makro, formül, harici bağlantı ve şifreli Excel kabul edilmez. İlk sürüm şablonu tek `Gorevler` sayfası kullanır. Sütunlar:

```text
external_id | group_slug | assignee_email | title | description | category | priority | due_date
```

`external_id` dosyanın kaynağında sabit bir satır kimliğidir. Grup slug'ı ve e-posta yalnızca başlatanın yetkili olduğu gruplar içinde çözülür. Bir insanın görünen adına göre atama yapılmaz. `.xlsx` tarihi ISO metin biçiminde istenir; belirsiz gün/ay veya sayısal Excel tarihleri sessizce yorumlanmaz. JSON aynı alanları `format: gorev-atolyesi-assignments-v2` zarfında taşır.

Akış:

1. Dosyayı yükle; 10 MiB sıkıştırılmış boyut, 100 MiB açılmış içerik, 1000 görev sınırı uygula. Parser süre/bellek ve ZIP parça sayısı limitleriyle izole worker'da çalışır.
2. Alanları ve her satırın yetkisini doğrula; sonuç `ready` veya `invalid` batch olur. Kullanıcı satır numarası, hata kodu ve düzeltme açıklaması görür.
3. Önizlemede hedef grup, kişi, başlık ve toplam atama sayısı gösterilir. Hatalı tek satır varsa bu sürümde batch'in tamamı commit edilemez.
4. Kullanıcı veya yetkili robot `commit` çağırır. Önizleme yetki garantisi sayılmaz; işlem anında üyelik ve izinler yeniden kontrol edilir. Yetki değişim işlemleri ve batch commit aynı grup kilitleme sırasını kullanır.
5. Görevler, atama geçmişi, denetim olayı ve bildirim outbox kayıtları tek transaction içinde yazılır. Bildirim teslimatı transaction dışında tekrar denenebilir.

Batch aktöre/integrasyona bağlıdır; başkasının batch kimliğiyle commit yapılamaz. Önizleme 24 saat geçerlidir. Aynı idempotency key + aynı içerik aynı sonucu döndürür; farklı içerik 409 olur. Kalıcı kaynak kayıtları üzerinden `(source_id, group_id, external_id)` benzersizliği, yeni istek anahtarıyla tekrar gönderilen satırın da kopya üretmesini önler. Her entegrasyonun ayrı kaynak kimliği vardır. UI ve kişisel token kullanan CLI, kullanıcı başına sunucunun oluşturduğu aynı kalıcı kaynak kimliğini kullanır; dosya seçmek veya yeni önizleme başlatmak bu kimliği değiştirmez. İstemci başka kullanıcının kaynak kimliğini seçemez. Bu kayıt fiziksel görev silinse bile tutulur.

Bu ilk sürüm create-only'dir; aynı external_id ile içerik değişmesi mevcut görevi sessizce güncellemez. Güncelleme ayrı, version kontrollü görev endpoint'inden yapılır. Başarılı batch raporu satır→görev kimliği eşlemesini içerir. Excel rapor hücreleri metin olarak yazılır; formül üretecek kullanıcı verileri çalıştırılabilir hücreye dönüştürülmez.

## 8. Grup iletişimi ve canlı olaylar

Her grup oluşturulduğunda `Genel` kanalı açılır. Lider ek grup kanalı oluşturabilir. İlk sürümde kanallar grubun tüm aktif üyelerine görünür; gizli alt kanal yoktur. Kullanıcı yeni katıldığı grubun geçmiş mesajlarını da görebilir; üyelik kaldırılınca tüm grup erişimi kapanır.

Mesajlar düz metin, en fazla 4000 karakterdir. Kullanıcı kendi mesajını 15 dakika içinde düzenleyebilir/silebilir; lider kendi grubunda moderasyon, ana yönetici her grupta moderasyon yapabilir. Arayüz düzenleme/silme durumunu gösterir. Audit, eylemi kaydeder fakat silinen mesaj gövdesini ayrıca kopyalamaz. Görev yorumları, görev okuma yetkisine tabidir; grup kanalına kapalı görev ayrıntıları otomatik yazılmaz.

Atama ve etiketleme bildirimleri alıcıya özeldir; okundu durumu tutulur. İlk sürüm online/typing ve okundu baskısı oluşturan kişi başı sohbet okundu bilgisi içermez.

Mesaj gönderme REST POST, olay alma SSE üzerinden yapılır. Mevcut 5 saniyelik görev yenilemesi bağlantı kesintisinde yedek mekanizma olarak korunur. SSE yalnızca cookie oturumuyla açılır; robotlar REST kullanır.

Dayanıklı olay kaydı PostgreSQL'dedir. NOTIFY yalnızca yeni olay kimliği için uyarıdır; mesaj gövdesi veya kişisel veri taşımaz. Kaçırılan uyarı DB'den telafi edilir. Her alıcıya gönderilmeden önce güncel grup/kaynak yetkisi kontrol edilir; üyelik iptali açık akışı da en geç 5 saniye içinde kapatır. Gönderilmiş veri geri alınamaz; sunucu iptalden sonra yeni veri göndermemeyi garanti eder. Yeniden bağlantıda Last-Event-ID üzerinden yalnızca yetkili olaylar döner. [PostgreSQL NOTIFY](https://www.postgresql.org/docs/current/sql-notify.html)

Mevcut genel 10 saniyelik istek context'i ve 15 saniyelik HTTP yazma timeout'u SSE'ye doğrudan uygulanamaz. Akış handler'ı için ayrı heartbeat/yazma deadline'ı, bağlantı kotası, 15 saniyelik heartbeat ve periyodik oturum doğrulaması tanımlanacak; diğer API timeout'ları korunacak.

## 9. Dosya paylaşımı güvenliği

- İzinli ilk türler: PNG, JPEG, PDF, TXT, CSV, DOCX ve XLSX. Dosya başına 20 MiB, mesaj başına 5 ek, kullanıcı başına 100 MiB/gün, grup başına 1 GiB başlangıç kotası. ZIP/RAR, SVG/HTML, çalıştırılabilir ve makrolu belgeler reddedilir.
- Uzantı, MIME, dosya imzası ve paket yapısı birlikte doğrulanır. Kullanıcının gönderdiği Content-Type tek başına yeterli değildir. XLSX/DOCX için açılmış boyut ve parça limitleri geçerlidir.
- Dosya önce özel karantina alanına gider. Ayrı, root olmayan ve ağ erişimi kısıtlı worker ClamAV ile tarar. Tarama başarısız/ulaşılamazsa dosya paylaşılmaz; durum `scan_failed` olur.
- Object storage herkese kapalıdır; DB yalnızca metadata tutar. Nesne anahtarı rastgele üretilir, dosya adı yol olarak kullanılmaz. Kullanıcıdan harici URL alınıp dosya indirilen özellik eklenmez.
- Yükleme rezervasyonu, tamamlanma ve indirme sırasında grup üyeliği ayrı ayrı doğrulanır. Grup değiştirerek başka grubun yüklemesini tamamlama engellenir.
- İndirme API üzerinden yetki kontrolüyle stream edilir; kalıcı/public object URL verilmez. `Content-Disposition: attachment`, `nosniff` kullanılır. İlk sürümde tarayıcı içinde PDF/Office önizlemesi yoktur.
- Grup mesajı eki grup üyelerine; görev yorumu eki yalnızca görevi görebilenlere açılır. Etkin izin, dosya ve bağlandığı kaynağın izinlerinin kesişimidir.
- Tamamlanmamış/bağlanmamış yüklemeler 24 saat sonra temizlenir. Kullanıcı silmesi dosyayı hemen erişime kapatır; fiziksel nesne 30 gün sonra yaşam döngüsüyle kaldırılır. Yedek saklama süresi ayrıca açıklanır.

Virüs taraması dosyanın zararsız olduğuna mutlak garanti vermez; tür kısıtları, karantina ve yetkili indirme birlikte uygulanır. [OWASP File Upload](https://cheatsheetseries.owasp.org/cheatsheets/File_Upload_Cheat_Sheet.html)

## 10. Web uygulaması, TUI ve terminal işletimi

İki farklı arayüz yapılacak:

**Uygulama GUI'si:** normal kullanıcı, grup lideri ve ana yöneticinin hesap, grup, görev, mesaj, dosya ve otomasyon ekranları. API tabanlı küçük Excel/JSON atama sihirbazı bu arayüzün parçasıdır.

**Terminal kontrol arayüzü (TUI):** Go ile çalışan `atolye control tui`, mevcut terminalde açılır; ayrı HTTP sunucusu veya kontrol portu kullanmaz. Uygulamadan ayrı süreç olduğundan uygulama durmuşken de açılır. Başlat, durdur, yeniden başlat, durum, maskelenmiş log, migration durumu, yedek durumu ve tarayıcıda uygulamayı aç seçeneklerini içerir. Production deploy/restore ilk sürümde bu arayüzden yapılmaz.

Yukarı/aşağı oklar seçenek değiştirir, Enter seçer, Tab paneller arasında ilerler, Esc geri döner ve `q` panelden çıkar. Panelden çıkmak çalışan uygulamayı durdurmaz; durdurma ayrı bir menü işlemidir. Profil adı, çalışma durumu ve son işlem sonucu her zaman görünür. Durdur/yeniden başlat öncesi hedef profil gösterilerek onay alınır. Uzun işlemler arayüzü kilitlemez; ilerleme ve sonuç görüntülenir, aynı profil için çakışan işletim işlemleri kilitlenir. Ctrl+C/çıkış sırasında terminal ayarları geri yüklenir; devam eden işlemin durumu açıkça bildirilir, yarım işlemin geri alındığı varsayılmaz. Etkileşimsiz ortamda TUI açık hata verir ve CLI alternatifini gösterir.

TUI, mevcut petrol/kehribar renklerini terminalin desteklediği ölçüde kullanır; durumlar renk yanında metinle de belirtilir. Dar terminal, yeniden boyutlandırma, renksiz çıktı ve klavye kullanımı desteklenir. Log içindeki güvenilmeyen ANSI/OSC ve kontrol dizileri temizlenir; parola/token alanları maskelenir.

Ortak terminal arayüzü:

```text
atolye admin bootstrap --email ...
atolye control tui
atolye system up --profile dev
atolye system down --profile dev
atolye system status --profile dev --json
atolye system logs --profile dev
atolye login --server https://...
atolye groups list
atolye tasks list --group platform
atolye assignments preview --file gorevler.xlsx
atolye assignments commit --batch <id>
```

Uygulama komutları aynı HTTP API'ye gider. `atolye login`, tarayıcıda kullanıcı + MFA onayı isteyen kısa ömürlü cihaz eşleştirme akışıyla scope'lu CLI token'ı alır; ana parolayı shell history'ye yazdırmaz. Başsız robot secret'ı stdin veya 0600 token dosyasından alır; komut argümanında token önerilmez. Kişisel CLI token'ı OS keyring'de tutulur; keyring yoksa açık onayla 0600 dosya seçeneği sunulur.

Kontrol katmanı yalnızca sabit izin listeli işlemler çalıştırır; gelen metni `sh -c` ile yürütmez. Profil dosyaları 0600, dosya yolları doğrulanmış, executable listesi sabittir. TUI erişimi mevcut OS oturumuna dayanır; uzaktan HTTP kontrol endpoint'i oluşturulmaz. Docker socket public uygulama container'ına bağlanmaz; kontrol aracı mevcut OS kullanıcısının yetkileriyle çalışır ve sessiz `sudo` yapmaz. Web uygulamasının yönetici rolü tek başına yerel işletim yetkisi sağlamaz.

`down` yalnızca bu profilin süreç/container'larını durdurur; port numarasına bakarak başka süreci öldürmez, volume/DB silmez. Kapatma sonunda gerçek süreç durumu yeniden okunur. Hata mesajları yerel dilde, makine çıktıları sabit JSON şemasında olur. CLI: başarı 0, doğrulama 2, kimlik/yetki 3, bağlantı 4, çakışma 5, sunucu hatası 6. Fish uyumlu kullanım komutları ve mevcut `.sh` giriş noktaları sağlanır. TUI ve CLI aynı işlem sonuçlarını ve hata kodlarını kullanır.

Yerel ekip geliştirme profili PostgreSQL, özel S3-uyumlu depolama, Mailpit, ClamAV, API ve worker'ı Docker Compose üzerinden yönetir. Kubernetes, staging/production yolu olarak korunur. Compose ve kind aynı isim/volume'leri paylaşmaz.

## 11. Tasarım dili ve ekran haritası

Mevcut token sistemi korunacak: kâğıt mavisi `#E8EEF3`, mürekkep `#16374A`, porselen `#FAFCFD`, petrol `#19746B`, kehribar `#DEB665`, çelik `#647D8D`. Bricolage Grotesque başlık/marka; Manrope form, görev ve iletişim metni. Koyu tema ve reduced-motion desteği devam eder.

Yerleşim karşılaştırması:

```text
A: Tek büyük yönetim ekranı
menü | kullanıcılar + görevler + sohbet + altyapı kontrolleri

B: Grup odaklı çalışma alanı — seçilen
sol menü | grup seçici + görev / konuşma / dosya sekmeleri
         | görev tepsileri veya konuşma akışı | isteğe bağlı detay paneli
```

A, işletim yetkilerini kullanıcı iş akışına karıştırır ve ekranı sıkıştırır. B'de mevcut dosya tepsisi biçimi panonun ayırt edici öğesi olarak kalır; diğer ekranlar aynı şekli gereksiz yere tekrar etmez. Başlıklar sola hizalı; mobilde detay paneli ayrı görünüm olur. Kullanıcının işi görev atamakken Docker/SQL gibi uygulama ayrıntıları gösterilmez.

Ekranlar:

- Kayıt → e-posta doğrulama → onay bekleme → MFA → ilk grup yönlendirmesi.
- “Görevlerim”, “Gruplarım”, “Konuşmalar”, “Bildirimler”, “Otomasyonlar”.
- Grup alanında “Pano / Üyeler / Konuşmalar / Dosyalar”; lider için üyelik ve izin kontrolleri.
- Ana yöneticide onay kuyruğu, gruplar/liderler, tüm görevler ve denetim kayıtları. Görev yükü tablosu gerçek sayılara dayanır.
- Atama formunda grup → o gruptaki aktif kişi → görev. Grup değişince eski kişi seçimi temizlenir.
- Toplu atama sihirbazı: dosya → kolon/doğrulama → önizleme → onay → sonuç. Hata, satırın yanında görünür.
- Konuşmada okunabilir mesaj akışı, dosya tarama durumu, gönderme hatasında korunmuş taslak ve tekrar deneme.
- Ana yöneticinin erişim kapsamı hesap/grup açıklamasında açıkça belirtilir; özel mesaj veya uçtan uca şifreleme izlenimi verilmez.

Frontend mevcut gömülü HTML/CSS/JS mimarisinde ES modüllerine ayrılacak; bu kapsam için framework geçişi yapılmayacak. Rol değişimi/grup geçişinde eski veriler ve client cache temizlenecek. API'den gelen `capabilities` yalnızca UI yönlendirmesi sağlar; backend yetkisinin yerine geçmez.

Mobil 360/390 px, tablet 768 px ve masaüstü 1440 px; açık/koyu tema; uzun Türkçe isimler; 1000 görev; yüklenme, boş, hata ve yetki iptali durumları incelenecek. Klavye dolaşımı, focus trap/geri dönüş, canlı bölge mesajları ve WCAG 2.2 AA kontrast hedefi test edilecek. Otomatik canlı güncelleme açık formu yeniden kurmayacak.

## 12. Test kişileri ve grupları

Fixture yalnızca ayrı geliştirme/test veritabanına, `atolye dev seed` ile oluşturulacak. Production/staging modunda seed komutu reddedilecek. Örnek hesaplar `.example.test` adresleri kullanacak; gerçek kullanıcı bilgisi kullanılmayacak.

| Grup | Lider | Üyeler | Ek durum |
|---|---|---|---|
| Go Ekibi | Deniz | Ada, Ece, Mert | Ece'ye tasks.assign izni |
| Platform Ekibi | Selin | Bora, Can, İpek | Liderine de görev atanmış |
| Kalite Ekibi | Arda | Duru, Ozan, Nil | Mert burada ek üye, lider değil |

Ana yönetici: Yönetici Demo. Ayrıca onay bekleyen 2 kişi ve askıya alınmış 1 kişi. Toplam 16 benzersiz kullanıcı; Mert'in iki üyeliği ayrıca hiyerarşi testidir.

Her grubun lideri ve üç ana üyesi için kişi başına 3 görev: toplam 36 grup görevi. Ana yönetici için 3 kişisel görev; toplam 39 görev. Her grupta Genel kanalı, 10 örnek mesaj ve zararsız metin/PNG ekler olacak. Gecikmiş görev, arşiv, çöp kutusu, farklı öncelik ve başarısız import örnekleri bulunacak. Zararlı dosya testleri gerçek kullanıcı alanına değil izole güvenlik test fixture'larına gider.

Parolalar rastgele üretilir ve yalnızca yerelde 0600 izinli seed çıktısına yazılır; sabit ortak parola repository'ye konulmaz. Seed ikinci çalıştırmada aynı fixture kimliklerini kullanır, gerçek kayıtları silmez veya mevcut parolaları sıfırlamaz. Fixture reset yalnızca oluşturduğu seed alanına uygulanabilir.

## 13. Güvenlik ve kabul testleri

| Sınama | Beklenen sonuç |
|---|---|
| Register gövdesine admin/leader alanı ekleme | Reddedilir; rol değişmez |
| Aynı anda iki bootstrap | Yalnızca biri başarılı |
| Onaysız/verify edilmemiş hesap | Görev/grup/mesaj erişimi yok |
| Liderin başka grup ID'si kullanması | Liste, detay, export ve dosyada erişim yok |
| Yetkisiz üyenin assignee değiştirmesi | Görev değişmez |
| Mert'in bir gruptaki iznini diğer grupta kullanması | Reddedilir |
| Token scope/grup dışına çıkma | Kullanıcı admin olsa bile reddedilir |
| Token/üyelik iptali, açık SSE | Yeni API işlemleri reddedilir, akış kapanır |
| Batch önizlemesinden sonra yetki kaldırma | Commit başarısız; görev yaratılmaz |
| İki robotun aynı external_id gönderimi | Tek görev oluşur |
| Commit cevabı kaybolup tekrar deneme | Aynı görev kimlikleri döner |
| Formüllü Excel, ZIP bomb, bozuk MIME | Limitli kaynak tüketimiyle reddedilir |
| Tarayıcıda XSS/CSRF/HTML dosya denemesi | Çalıştırılmaz, yetki atlanmaz |
| Dosya tarayıcı kapalıyken yükleme | Dosya indirilemez/paylaşılamaz |
| Başka grubun attachment ID'si | İçerik/metadata sızmaz |
| Kullanıcının sistem kontrol endpoint'ine erişmesi | Uygulamada böyle işletim endpoint'i yok |
| TUI logunda ANSI/OSC kontrol dizileri | Terminal komutu/kaçış etkisi oluşturmadan temizlenir |
| TUI ok/Enter/Esc, yeniden boyutlandırma ve Ctrl+C | Seçim doğru çalışır, terminal ayarları geri yüklenir |
| TUI'den çıkış ve eşzamanlı TUI/CLI işlemi | Çıkış sunucuyu kapatmaz; çakışan işlemler kilitlenir |
| Migration/DB+object restore | Görevler, ilişkiler, mesaj ve ekler korunur |

Test katmanları: politika unit testleri; PostgreSQL transaction/concurrency entegrasyonu; gerçek HTTP API negatif testleri; Chrome ile kayıt/lider/admin/üye akışları; CLI ve robot sözleşme testleri; dosya parser fuzz testleri; iki API replikası + worker kesintisi testleri. Güvenlik tarayıcısının temiz çıkması, nesne bazlı yetki testlerinin yerine geçmez.

Küçük ekip hedefi için başlangıç yük kabulü: 100 aktif hesap, 20 grup, 50 eşzamanlı SSE bağlantısı; 2 vCPU/4 GiB uygulama/worker test ortamında normal görev API'sinde p95 <500 ms hedefi. Import/tarama asenkron iş olarak ölçülür; parola hashleme bu gecikme hedefinin dışında tutulur. Limitler ölçüm raporuna göre değiştirilir, sessizce kaldırılmaz.

## 14. Uygulama sırası ve teslim kapıları

| Aşama | Teslim | Bir sonraki aşamaya geçiş koşulu |
|---|---|---|
| 0. Temel | OpenAPI taslağı, yetki matrisi, tehdit modeli, migration stratejisi, tasarım wireframe'leri | Rol/kaynak sınırları ve varsayımlar net |
| 1. Kimlik | Web register, doğrulama, onay, MFA, ilk admin CLI, kurtarma | Admin yükseltme/kimlik testleri başarılı |
| 2. Ekip ve atama | Gruplar, liderlik, üyelik, task assignee, admin/lider ekranları | Tüm çapraz grup negatif testleri başarılı |
| 3. Otomasyon | Scope'lu anahtar, servis hesabı, API/CLI, JSON/XLSX batch, küçük GUI, Robomotion örneği | Tekrarlı atama ve yetki değişimi testleri başarılı |
| 4. İletişim | Grup sohbeti, yorum, bildirim, SSE/outbox | İptal/reconnect/çoklu replika testleri başarılı |
| 5. Dosyalar | Özel storage, karantina/tarama, yetkili indirme, kota | Dosya güvenliği ve kaynak tüketimi testleri başarılı |
| 6. İşletim | Terminal kontrol TUI/CLI, Compose profili, Kubernetes worker/Secret/kotalar | Klavye/terminal testleri başarılı; aç/kapat başka süreç/veriye dokunmuyor |
| 7. Demo ve yayın hazırlığı | 16 test kişi, 3 grup, tasarım incelemesi, yük/güvenlik/restore raporu | Staging kabul listesi tamam |

Her aşama kod + migration + API belgesi + test + ekran görüntüsü + bilinen sınırlar ile tamamlanır. Yeni ekranlar birden fazla aşamada gelişir; tasarım yalnızca son aşamaya bırakılmaz. Kullanıcı verileri üzerinde test/seed çalıştırılmaz.

## 15. Bağlam ve ilerleme yönetimi

Bu plan sonraki oturumların başvuru belgesidir; tamamlandı iddiası değildir. Uygulamada `docs/IMPLEMENTATION-STATUS.md` aşama durumu, son başarılı kontroller, bekleyen kararlar ve bir sonraki somut işi tutacak. `docs/decisions/` altında yetki, auth, dosya ve işletim sınırı kararları ayrı kaydedilecek.

Oturum uzadığında tamamlanan aşamalar bu kayıtlardan sürdürülecek; konuşma belleğine dayanılarak migration veya test tekrarlanmayacak. Otomatik bağlam sıkıştırması gerekirse sistem yapar; bu plan bunun yerine kalıcı teknik referans sağlar.

Gerçek e-posta, domain/TLS, object storage, MFA şifreleme anahtarı ve dış Robomotion bağlantısı için ortam değerleri gerekir. Geliştirme karşılıkları ayrı profilde sağlanacak. Planın uygulanması, gerçek hesaplara ileti gönderme veya production yayınına kendiliğinden onay anlamına gelmez.
