# İki adımlı doğrulama ve kurtarma

11 Eylül 2026 — TOTP, kurtarma kodları ve yerel operatör sıfırlaması uygulandı. Mevcut hesap onaylı geliştirme modunda çalışır. Gerçek posta teslimatı/parola sıfırlama tamamlanmadığından staging/production engeli korunur.

## Güncelleme

Açık TUI'den `q` ile çıkıp Fish terminalinde:

```fish
cd (git rev-parse --show-toplevel)/gorev-atolyesi
and bash scripts/atolye.sh system restart --profile local
and bash scripts/atolye.sh control tui
```

SQLite migration 5 otomatik uygulanır; görevler ve hesaplar korunur. PostgreSQL için mevcut migrate dağıtım adımı gerekir. MFA anahtarı ilk hesap onaylı sunucu açılışında oluşturulur.

## İlk kurulum

1. Web'de kayıt ol, giriş yap ve e-posta doğrulamasını tamamla.
2. Onay bekleme ekranındaki **İki adımlı doğrulamayı kur / yönet** düğmesini kullan. Aktif hesapta aynı işlem sol menüdedir. Mevcut ana yöneticilerde kurulum ekranı zorunlu açılır.
3. Mevcut parolanı girip **Kurulumu başlat** seç.
4. Kimlik doğrulama uygulamana hesabı elle ekle: ekrandaki anahtar, zaman tabanlı TOTP, 6 hane, 30 saniye. Bu sürüm QR yerine kopyalanabilir kurulum anahtarı sunar.
5. Uygulamadaki kodu girerek etkinleştir. Sırf anahtarın gösterilmesi MFA'yı etkinleştirmez; kurulum aynı tarayıcı oturumunda 10 dakika içinde doğrulanmalıdır.
6. Gösterilen **10 kurtarma kodunu** güvenli bir yere kaydet. İstersen **Kodları indir** ile metin dosyası al. Kodlar daha sonra tekrar gösterilmez.
7. İlk ana yönetici henüz seçilmediyse TUI'deki **İlk yöneticiyi seç** ile hesabını seç. Yalnızca e-postası doğrulanmış ve MFA kurulmuş adaylar görünür. Bootstrap sonrası yeniden giriş yap.

Üyeler MFA'yı isteğe bağlı açar. Ana yöneticide zorunludur. Grup liderleri henüz uygulamada bulunmuyor; lider aşamasına aynı zorunluluk bağlanacaktır.

## Sonraki girişler ve yönetim

Önce e-posta/parola, sonra yeni TOTP veya kullanılmamış bir kurtarma kodu girilir. Parola sonrası kısmi oturum en fazla 5 dakika geçerlidir ve görev erişimi sağlamaz. Başarılı MFA oturum kimliğini değiştirir ve 12 saatlik tam oturum açar.

Hesap yönetimi için son 5 dakika içinde yapılmış MFA doğrulaması gerekir. Süre dolduğunda yeniden kod istenir; ardından yönetim ekranını açıp işlemi tekrar seç. Hesap onaylama/askıya alma ayrıca mevcut yönetici parolanı ister. Aynı 30 saniyelik TOTP tekrar kullanılamaz; uygulamadaki yeni kodu bekle. Bilgisayar ve telefon saati doğru olmalıdır; ±1 zaman aralığı toleransı vardır.

## Telefon veya kurtarma kodları kaybolursa

Telefon yoksa parola sonrası ekranda bir kurtarma kodu kullan. Her kod yalnızca bir kez çalışır; hesaba bağlıdır ve parola yerine geçmez.

Kalan kodları yenilemek için **İki adımlı doğrulama** düğmesine basıp parolanı ve kullanılmamış TOTP/kurtarma kodunu gir. Yeni liste bir kez gösterilir; eski kurtarma kodlarının tamamı ve diğer oturumlar iptal edilir. Kurulum/yenileme cevabı bağlantı nedeniyle kaybolursa anahtar ve eski kodlar geri gösterilmez; yeni TOTP ile listeyi tekrar yenileyebilirsin.

Telefon ve bütün kodlar kayıpsa, makinenin yetkili operatörü terminal menüsündeki **Kayıp MFA / operatör kurtarması** bölümünden hesabı seçip onaylar. Menüsüz karşılık:

```fish
bash scripts/atolye.sh admin mfa-reset --email 'kayitli-adresin@example.com'
```

Bu işlem kullanıcı rolünü veya parolasını değiştirmez. MFA sırrını, kurtarma kodlarını ve tüm oturumları siler; denetim kaydı oluşturur. Ana yönetici yeniden MFA kurmadan çalışma alanına erişemez. Web'den MFA kapatma/sıfırlama endpoint'i yoktur. Yerel OS/DB erişimi, web yöneticiliğinden ayrı bir işletim yetkisidir.

## Şifreleme anahtarı ve yedek

Varsayılan dosya `.control/mfa.key`; farklı konum için `MFA_KEY_FILE` kullanılır. TUI/CLI aynı dosyayı sunucuya geçirir. Dosya 32 baytlık hex anahtar içerir, izinleri `0600` olmalıdır ve symlink kabul edilmez. Anahtar log veya API cevabına yazılmaz.

**Veritabanı yedeğiyle birlikte bu dosyayı da ayrı ve korumalı biçimde yedekle.** DB'de yalnızca anahtarın fingerprint'i tutulur. Dosya kaybolursa veya farklı dosya verilirse sunucu yeni anahtar üretip devam etmez; asıl anahtar gerekir. Telefon kurtarma kodları kaybolmuş şifreleme anahtarının yerine geçmez. `.control/` klasörünü temizlemek bu anahtarı da siler; klasörü geçici build dizini gibi değerlendirme. Mevcut DB yedek scriptleri bu dosyayı otomatik olarak yedeklemez.

## Güvenlik sınırları ve API

- TOTP sırrı kullanıcıya ve kullanım türüne bağlı AAD ile AES-256-GCM şifrelenir. Bekleyen kurulum ayrı AAD taşır; farklı hesabın şifreli kaydı kopyalanarak kullanılamaz.
- TOTP: HMAC-SHA1, 30 saniye, 6 hane. Son tüketilen zaman adımı DB'dedir; eşzamanlı tekrar kullanımı önlenir.
- Kurtarma kodu başına 128 bit rastgelelik, DB'de SHA-256 hash, tek kullanımlık tüketim. Aktif sır ve kurtarma kodları hiçbir listeleme API'sinde dönmez.
- `POST /api/mfa/setup`: parola, aynı oturuma bağlı bekleyen anahtar.
- `POST /api/mfa/confirm`: ilk TOTP; etkinleştirme, oturum değişimi ve 10 kurtarma kodu.
- `POST /api/mfa/verify`: TOTP/kurtarma koduyla kısmi oturumu tamamlama veya güncel MFA doğrulaması.
- `POST /api/mfa/recovery`: parola + kullanılmamış ikinci faktörle yeni kurtarma listesi.
- Her hesap için bütün MFA işlemlerinde toplam 5 dakikada 10 deneme sınırı vardır. E-posta doğrulanmış olmalı ve hesap askıda olmamalıdır. Kod ve parola gövdeleri loglanmaz.
- Kurulum ve kod yenileme diğer oturumları kapatır; doğrulama sadece kullanılan oturumu değiştirir. İşlemler, kod tüketimi ve audit aynı transaction'dadır.

## Doğrulama

RFC test vektörleri, şifreleme/AAD/anahtar uyuşmazlığı, anahtar kaybı, oturuma bağlı ve süreli kurulum, parola ile tek başına erişememe, tüketilen TOTP/kurtarma kodu, eşzamanlı kurtarma, yeni kod setinde eski kodların iptali, MFA reset sonrası zorunlu kurulum, hız sınırı ve eski MFA ile yönetim engeli test edilir. Chrome testi gerçek TOTP kurulumunu ve kurtarma koduyla girişi geçici DB'de çalıştırır. `.sh` acil sıfırlama akışı da izole yerel sunucuda doğrulanmıştır. Gerçek PostgreSQL üzerinde bu MFA akışı ayrıca test edilmedi.

```fish
source scripts/env.fish
and go test -race ./...
and go vet ./...
and go run ./cmd/identity-e2e
```

Başvurular: [RFC 6238](https://www.rfc-editor.org/rfc/rfc6238), [OWASP MFA](https://cheatsheetseries.owasp.org/cheatsheets/Multifactor_Authentication_Cheat_Sheet.html).
