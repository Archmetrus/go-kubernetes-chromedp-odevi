# Web kaydı ve ilk yönetici

Bu sürümde kullanıcı e-posta/parolasını web arayüzünde girer. Terminal yalnızca ilk ana yöneticinin hangi kayıtlı hesap olacağını belirler. Sonraki hesapları ana yönetici web arayüzündeki **Hesap yönetimi** üzerinden onaylar veya askıya alır.

Bu, ekip planının kimlik aşamasıdır. E-posta kod doğrulama akışı geliştirme posta kutusuyla; MFA/TOTP ve kurtarma kodları gerçek ikinci faktör olarak uygulanmıştır. Gerçek SMTP teslimatı, parola sıfırlama, grup liderliği ve API token'ları henüz tamamlanmadığından **yeni hesap onaylı mod staging/production ortamında başlatılamaz**. Yerel posta kutusundan kod okumak gerçek bir e-posta hesabına sahip olunduğunu kanıtlamaz; geliştirme simülasyonudur. Hesap onayı yalnızca kişisel görev alanını açar; ana yöneticinin bütün görevleri görmesi ve görev ataması sonraki yetki aşamasındadır.

## Terminal menüsünden

```fish
cd (git rev-parse --show-toplevel)/gorev-atolyesi
and bash scripts/atolye.sh control tui
```

1. `local` profilinde sunucu açıksa **Durdur** seç.
2. **Hesaplı kaydı etkinleştir** seç; açıklamayı okuyup Enter ile onayla.
3. **Başlat**, ardından **Siteyi tarayıcıda aç** seç.
4. Sitede e-posta ve en az 12 karakterlik parolanla **Hesap oluştur** seç. **Giriş yap** ile e-posta doğrulama ekranını aç.
5. Terminal menüsünde **Doğrulama iletilerini göster** seç. Kendi e-postanın altındaki 32 karakterlik kodu kopyalayıp sitedeki **E-postamı doğrula** alanında kullan. Kod gerekirse **Yeni kod iste** ile yenilenebilir.
6. Web'de **İki adımlı doğrulamayı kur / yönet** ile MFA kur ve kurtarma kodlarını sakla. Sonra terminal menüsünde **İlk yöneticiyi seç** seç. Yalnızca e-postası doğrulanmış ve MFA kurulmuş adaylar listelenir. Kendi hesabını oklarla seç ve onayla. Terminalde parola yazman gerekmez. [MFA kurulumu](MFA-VE-KURTARMA.md).
7. Sitede yeniden giriş yap. Sol menüde **Hesap yönetimi** açılır.
8. Diğer kişiler aynı kayıt ve doğrulama ekranından geçer. Hesap yönetiminde ilgili kişiyi onaylarken mevcut yönetici parolanı gir. E-postası doğrulanmayan bir hesap onaylanamaz.

Bootstrap tek seferliktir. İki terminal aynı anda denerse yalnızca biri başarılı olur. Terminaldeki aday listesi ilk 100 hesabı gösterir; daha büyük kurulumda aşağıdaki e-posta komutunu kullan. Ana yönetici kendini askıya alamaz; bu sürümde ikinci yönetici atama/rol değiştirme endpoint'i yoktur.

## Menüsüz Fish komutları

Kayıt moduna geç ve başlat:

```fish
cd (git rev-parse --show-toplevel)/gorev-atolyesi
and bash scripts/atolye.sh system down --profile local
and bash scripts/atolye.sh system account-mode --profile local
and bash scripts/atolye.sh system up --profile local
```

**Web üzerinden kayıt olduktan sonra** geliştirme iletisini oku:

```fish
bash scripts/atolye.sh admin mailbox
```

Kodu sitede doğrulayıp MFA kurulumunu tamamladıktan sonra adresi kendi kayıtlı e-postanla değiştir:

```fish
bash scripts/atolye.sh admin list --json
and bash scripts/atolye.sh admin bootstrap --email 'senin-adresin@example.com'
```

Liste 100 kayıtla sınırlıdır. Son kullanıcının `id` değeriyle `admin list --after 'id' --json` sonraki sayfayı getirir. Bu yerel komutlar yalnızca OS operatörü içindir; normal kullanıcıların veya Robomotion'un DB erişimi için değildir. Robot API yetkilendirmesi ayrıca geliştirilecektir.

Güncelle ve durumu gör:

```fish
bash scripts/atolye.sh system restart --profile local
and bash scripts/atolye.sh system status --profile local --json
```

## Veri ve yetki sınırları

- Migration 3–4 mevcut kullanıcıları, kullanıcı kimliklerini ve görevleri korur. Önceden var olan hesaplar aktif üye kalır; yeni hesaplar onay bekler. Hesap onaylı modda eski hesaplar ve mevcut ana yönetici de e-posta kodunu bir kez doğrular; görevleri silinmez. İlk iletisi olmayan eski hesap ekrandan **Yeni kod iste** kullanır. Hesapsız ve önceki onaysız kişisel modda bu yeni doğrulama şartı uygulanmaz.
- Sahibi `local` olan eski kişisel görevler DB'de kalır ve bir kullanıcıya kendiliğinden verilmez. Açık sahiplik aktarımı için önceki `claim-local` iş akışı korunur; kayıt olmak eski görevleri sahiplenmez.
- Hesap zorunluluğu DB'de saklanır. Ortam değişkenini kaldırmak veya `.control/accounts-enabled` dosyasını silmek DB'yi tekrar hesapsız erişime açmaz. Dosya yalnızca TUI/CLI'nin başlatma ortamını hatırlar.
- Aynı `DB_PATH`/`DATABASE_URL` ortamını kontrol aracı ve sunucu için kullan. Varsayılan SQLite dosyası `data/gorevler.db`. `.env` otomatik okunmaz.
- `ACCOUNT_APPROVAL=true`, `AUTH_ENABLED=true` ve `ALLOW_SIGNUP=true` manuel Go başlatma karşılığıdır. TUI bunları hesaplı mod için kendisi ayarlar.
- Aktif olmayan hesapların listeleme, dışa aktarma, görev yazma ve yönetim API'lerine erişimi sunucuda engellenir. Bekleyen kullanıcı yalnızca giriş/çıkış ve hesap durumunu kullanabilir.
- Kayıt isteğindeki `role`, `status` gibi bilinmeyen alanlar reddedilir. Onaylı modda tekrar kayıt ve ilk kayıt aynı cevabı verir.
- Yönetim işlemlerinde rol/durum tekrar DB'den okunur, yönetici parolası tekrar doğrulanır ve kullanıcı başına dakikada beş deneme sınırı uygulanır. Sürüm kontrolü eski ekrandan yapılan değişikliği reddeder.
- Onaylama ve askıya alma, oturum iptali ve denetim kaydıyla aynı transaction içinde tamamlanır. Bootstrap da mevcut oturumları kapatır. Parola ve oturum sırları denetim kaydına yazılmaz.

## Mevcut HTTP sözleşmesi

| Uç nokta | Yetki ve davranış |
|---|---|
| `POST /api/register` | Kayıt açıksa `email`, `password`; onaylı modda genel 202 cevabı |
| `POST /api/login` | Geçerli parola; askıdaki hesap için genel 401; bekleyen hesap durum ekranına giriş yapabilir |
| `GET /api/session` | `approval_required` ve kullanıcı `role`, `status`, `version` alanları |
| `POST /api/email-verification/request` | Oturum açmış, askıda olmayan hesap; `{}`; dakikada 1 yeni kod |
| `POST /api/email-verification/confirm` | Aynı hesabın oturumu ve `{ "code": "..." }`; dakikada 10 deneme; onay durumunu değiştirmez |
| `GET /api/admin/users?after=...` | Aktif ana yönetici; 100 kayıt, `users` ve `next` |
| `POST /api/admin/users/{id}/status` | Aktif ana yönetici; `status`, `version`, mevcut `password`; başarı 204 |

Hesap durum değişikliği sadece `active` veya `suspended` kabul eder; rol değiştirme parametresi kabul etmez. JSON gövdesi, mevcut Origin kontrolü, SameSite/HttpOnly oturum cookie'si ve DB'deki hız sınırları kullanılır. Bu endpoint'ler Bearer otomasyon API'si değildir.

## Doğrulama

```fish
source scripts/env.fish
and go test -race ./...
and go vet ./...
and go run ./cmd/identity-e2e
```

Yeni testler: rol enjeksiyonu, tekrar kayıtta hesap ifşası, bekleyen üye erişimi, yönetici parolası, eski sürüm, oturum iptali, askıdaki giriş, eski görevlerin korunması, eşzamanlı bootstrap ve yayın sınırı. Tarayıcı testleri geçici DB/hesap kullanır; mobil/koyu tema ekranları `outputs/identity-*.png` altında üretilir. PostgreSQL kimlik akışı bu teslimde gerçek sunucuda ayrıca test edilmedi.

Güvenlik kararları için başvurular: [OWASP Authorization](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html), [OWASP Authentication](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html).

## Doğrulama kodlarının yaşam döngüsü

Kayıt, kod hash'i ve geliştirme iletisi aynı transaction içinde yazılır. Kod 128 bit rastgelelik taşır, 24 saat geçerlidir ve hesaba bağlıdır. Kullanım, tekrar gönderim ve süre aşımı önceki kodu geçersiz kılar. GET isteği kod tüketmez; doğrulama yalnızca oturumlu POST ile yapılır. E-posta doğrulaması yönetici onayını veya hesap rolünü kendiliğinden değiştirmez.

`email_verifications` tablosu kodun SHA-256 hash'ini tutar. Yalnızca geliştirme için kullanılan `dev_mailbox` teslimat simülatörü okunabilir kodu geçici tutar; bu tablo bir üretim posta sistemi değildir. İleti tüketilince veya hesap askıya alınınca silinir; süresi dolan kayıtlar periyodik temizlenir ve posta kutusunda gösterilmez. Kullanıcı başına yalnızca son ileti tutulur. `admin mailbox --json` yerel operatör içindir; hiçbir web API'si posta kutusu kodlarını döndürmez.

Gelecekte SMTP teslimatı eklenirken geliştirme doğrulamaları gerçek e-posta sahipliği kanıtı olarak taşınmamalıdır. Şimdilik staging/production engeli korunur. [Tek kullanımlık kodlar için OWASP ilkeleri](https://cheatsheetseries.owasp.org/cheatsheets/Forgot_Password_Cheat_Sheet.html).

Migration 5 sonrası mevcut yönetici de MFA kurmadan çalışma alanına erişemez. MFA açık hesapta parola girişinden sonra ikinci faktör gerekir. Yönetim için son 5 dakikada yapılmış doğrulama aranır; hesap durum değişikliği ayrıca mevcut parola ister. `.control/mfa.key` dosyasını korumalı biçimde yedekleyin. [Anahtar ve kurtarma ayrıntıları](MFA-VE-KURTARMA.md).
