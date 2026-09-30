# Ekip ağacı, görev atama ve demo hesaplar

Sonraki teslim: [JSON/XLSX toplu atama ve otomasyon anahtarları](TOPLU-ATAMA-VE-OTOMASYON.md) artık kullanılabilir. Aşağıdaki başlangıç hiyerarşisi ve demo komutları korunur.

11 Eylül 2026. Kullanıcının öncelik değişikliğiyle SMTP/parola sıfırlamadan önce ekip hiyerarşisi uygulandı. Ana yönetici, grup lideri ve üyeler gerçek DB kayıtlarıdır; ekran, görevi ve yetkiyi API'den alır.

## Açma ve kapatma — Fish

Demo ilk kez oluşturuldu ve DB üzerinden doğrulandı. Aşağıdaki `seed` mevcut demo varsa verileri değiştirmeden çıkar:

```fish
cd ~/PROJE/go-kubernetes-chromedp-odevi/gorev-atolyesi
and bash scripts/ekip-demo.sh seed
and bash scripts/ekip-demo.sh up
and bash scripts/ekip-demo.sh accounts
```

Tarayıcıda **http://127.0.0.1:8092** adresini aç. Terminalde gösterilen hesap/parolayla giriş yap, sol menüde **Ekip ağacı ve görevler** seç. Ana yönetici hesabı `yonetici@atolye.example.test`.

Lider ve yönetici için ikinci faktör gerekir. `accounts` komutu bu test hesaplarının geçerli TOTP kodunu ve kalan saniyesini gösterir. Kod kullanıldıysa yeni zaman aralığını bekleyip komutu tekrar çalıştır. İstersen aynı komuttaki TOTP anahtarını kendi doğrulama uygulamana elle ekle. Bu kolaylık yalnızca yerel demo dosyasını okuyabilen OS operatöründedir; web'de giriş atlama veya kod öğrenme endpoint'i yoktur.

Oklar ve Enter ile kontrol:

```fish
bash scripts/ekip-demo.sh tui
```

Kodu güncelleme / yeniden derleme:

```fish
bash scripts/ekip-demo.sh restart
```

Kapatma ve durum kontrolü:

```fish
bash scripts/ekip-demo.sh down
and bash scripts/ekip-demo.sh status
```

Demo TUI ve CLI mevcut işletim katmanını kullanır; ayrı port, süreç kaydı ve anahtarı vardır. Ana kişisel sunucuyu kapatmaz. TUI'de `q` yalnızca paneli kapatır. `kind` profili bu demo için kullanılmaz; yerel profilde kal.

## Ağaç ve örnek kayıtlar

```text
yonetici — ana yönetici
├── Go Ekibi
│   ├── go-lider — lider
│   ├── go-deniz — atama yetkili üye
│   ├── go-ece — üye
│   └── yonetici — görev alabilen grup üyesi
├── Platform Ekibi
│   ├── platform-lider — lider
│   ├── platform-ali — atama yetkili üye
│   └── platform-selin — üye
└── Kalite Ekibi
    ├── kalite-lider — lider
    ├── kalite-can — atama yetkili üye
    └── kalite-derya — üye
```

Bütün adreslerin sonu `@atolye.example.test`. Her hesaba farklı rastgele parola üretilir. Başlangıçta 10 hesap, 3 grup, 12 görev ve 12 görev geçmişi kaydı vardır. Liderlerin ve ana yöneticinin de atanmış görevleri bulunur.

## Kullanılabilen işlemler

| Yetki | Görünür görevler | Yapabilecekleri |
|---|---|---|
| Ana yönetici | Bütün ekip görevleri | Grup oluşturma, lider değişimi, üye/yetki yönetimi, gruptaki kişilere atama, düzenleme, durum ve geçmiş |
| Grup lideri | Kendi grubunun bütün görevleri | Üye ekleme/çıkarma, atama yetkisi verme, MFA kurulmuş üyeye liderlik devretme, atama ve takip |
| Atama yetkili üye | Kendisine atanan ve mevcut grubunda kendisinin oluşturduğu görevler | Grup üyelerine atama, kendi oluşturduklarını düzenleme, kendi görevlerinin durumunu değiştirme |
| Üye | Kendisine atanmış ekip görevleri | Durum değiştirme ve görev geçmişini görme |

Bir kişi birden fazla gruba katılabilir; liderlik ve atama izni grup bazındadır. Bir işin tek sorumlusu vardır. Lider veya yönetici de görev alacaksa ilgili grubun üyesi olmalıdır.

Ağaçta grup veya kişi seçimi görevlere filtre uygular. Arama, durum filtresi, tamamlanma göstergesi, 5 saniyelik yenileme, görev geçmişi ve JSON indirme bulunur. JSON indirme, ekran filtrelerinden bağımsız olarak **oturumun görmeye yetkili olduğu bütün ekip görevlerini** içerir.

**Üyeler ve yetkiler** seçeneği bir grup seçildiğinde görünür. Üye eklemek için aktif/doğrulanmış hesabın tam e-postası kullanılır. Lider seçilecek hesabın MFA'sı kurulmuş olmalıdır. Üye çıkarma öncesinde açık görev sayısı gösterilir; bu işler lidere aktarılır. Tamamlanmış görevlerin eski sorumlusu korunur. Lider çıkarılamaz veya hesabı askıya alınamaz; önce liderlik devredilir.

## Veriler nerede?

- Demo DB: `data/demo/ekip.db`.
- Demo giriş bilgileri: `.control/demo/accounts.json`, `0600` izinli; HTTP üzerinden sunulmaz.
- Demo MFA anahtarı: `.control/demo/mfa.key`.
- Demo süreç kaydı/log/binary: `.control/demo/`.
- Asıl kişisel DB: `data/gorevler.db`; demo oluşturulurken bu dosyaya dokunulmadı.

Yeni `teams`, `team_members`, `team_tasks`, `team_events` tabloları migration 6 ile eklenir. Eski kişisel işler `tasks` tablosunda ve **Kişisel tezgâh** görünümünde kalır. Ana yönetici için bütün **ekip** işlerinin görünürlüğü sağlandı; kişisel işleri sessizce ekiplere aktarma veya diğer kişilerin kişisel alanını açma yapılmaz.

DBeaver'da demo için doğru dosyayı aç: `data/demo/ekip.db`. SQL değişikliğini commit ettikten sonra web'de Yenile kullan; açık ekip ekranı 5 saniyede bir de yenilenir. Elle SQL düzenlemek uygulamanın yetki, görev devri ve denetim kayıtlarını çalıştırmaz; görev/üyelik değişikliklerinde API veya ekranı kullan.

Demo doğrulaması operatör tarafından DB'ye işlenir ve `identity_audit` içinde `demo.account.verified` olarak kaydedilir. Gerçek e-posta gönderildiği veya sahiplik doğrulandığı iddia edilmez. Demo seed yalnızca boş yerel geliştirme DB'sini kabul eder; mevcut hesap/görevleri silmez veya yeniden parolalamaz.

## API ve güvenlik

Cookie oturumu ve mevcut Origin/CSRF, hesap durumu, e-posta ve MFA denetimleri korunur. Lider/yöneticinin yazma işlemlerinde son 5 dakika içinde MFA gerekir. Yetki ve DB sürümü aynı transaction içinde kontrol edilir. Ekip yazmaları hesap askıya alma ile aynı kilidi paylaşır; atama/yetki değişimi ve geçmiş kaydı atomiktir.

| Uç nokta | İşlem |
|---|---|
| `GET /api/teams` | Yetkili ağaç, görünür görevler ve işlem izinleri |
| `POST /api/teams` | Grup oluştur; `name`, `email` (lider) |
| `POST /api/teams/{id}/members` | `action`: `member`, `leader`, `remove`; `version`, `user_id` veya `email`, `can_assign` |
| `POST /api/team-tasks` | Gruba görev ata; `team_id`, `assignee_id`, görev alanları |
| `PUT /api/team-tasks/{id}` | Yetkili düzenleme; mevcut `version` gerekli |
| `POST /api/team-tasks/{id}/status` | `status`, mevcut `version` |
| `GET /api/team-tasks/{id}/history` | Yetkili görev geçmişi; en son 100 kayıt |
| `GET /api/teams/export` | Yetkili görevlerin JSON çıktısı |

Rol, oluşturucu ve görünürlük tarayıcıdan gelen yetki alanlarıyla belirlenmez. Grup üyeliği kaldırılınca geçmiş ve export erişimi de kalkar. Atama izni kaldırılan üye, kendisine ait olmayan eski atamalarını göremez. Normal üye başlığı/sorumluyu değiştiremez; yalnızca durumunu değiştirir. Sürüm çakışması `409`, yetki reddi `403` döner.

## Test ve kalan kapsam

DB fixture doğrulaması; yetki matrisi, gruplar arası erişim, sorumlu üyeliği, lider MFA, lider devri, görev aktarımı, izin iptali, eşzamanlı sürüm çakışması, geçmiş/export kapsamı, askıdaki hesap, eski MFA ve gerçek tarayıcıda yönetici/lider/üye senaryoları bulunur.

```fish
source scripts/env.fish
and go test -race ./internal/tasks -run TestTeam -count=1
and go run ./cmd/teams-e2e
```

Bu öncelikli demo için SQLite desteği korundu; migration SQL'i PostgreSQL için de yazıldı ancak bu ekip akışı gerçek PostgreSQL sunucusunda henüz doğrulanmadı. Canlı ekip dağıtımı için PostgreSQL planı ve production engeli korunur. SMTP/parola sıfırlama, otomasyon token'ları, Excel toplu atama, grup mesajları/dosyaları ve kişisel görevleri ekibe taşıma sonraki işlerdir.
