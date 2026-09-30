# Terminal kontrol paneli

Toplu atama eklendi: **Dosyadan görev / önizle**, **Son önizlemeyi onayla / ata**, **Toplu atama sonuçları**. Panel `data/imports/` içindeki JSON/XLSX dosyalarını API ile işler; doğrudan DB'ye yazmaz. Önce web'de grup kapsamlı anahtar oluşturup `bash scripts/ekip-demo.sh assign setup` (demo) veya `bash scripts/atolye.sh assign setup` ile korumalı dosyaya kaydet. [Ayrıntılı akış](TOPLU-ATAMA-VE-OTOMASYON.md).

Ekip demosu için `bash scripts/ekip-demo.sh tui` kullanılır. Aynı kontrol katmanı `.control/demo` süreç kaydını ve 8092 portunu kullanır; demo panelinde Kubernetes profiline geçiş kapalıdır. Asıl sunucu bu profilden durdurulmaz. [Demo hesapları ve komutlar](EKIP-AGACI-DEMO.md).

Fish veya Bash içinde proje klasöründen:

```fish
cd (git rev-parse --show-toplevel)/gorev-atolyesi
and bash scripts/atolye.sh control tui
```

Ok tuşları seçenek değiştirir, Enter çalıştırır. Durdur, yeniden başlat, hesaplı mod ve ilk yönetici seçimi için ikinci Enter onay verir; Esc vazgeçer. Tab yerel (`local`) ve Kubernetes (`kind`) profilleri arasında geçer. PgUp/PgDn sonuçları kaydırır; `q` veya Ctrl+C paneli kapatır. İşlem sürerken önce tamamlanması beklenir. Paneli kapatmak sunucuyu durdurmaz. Terminal en az 40 sütun ve 19 satır olmalıdır. `NO_COLOR=1` renksiz görünüm sağlar.

Menüdeki **Hesaplı kaydı etkinleştir**, **Doğrulama iletilerini göster** ve **İlk yöneticiyi seç**, web kayıt sürecini aynı kontrol katmanına bağlar. Doğrulama iletileri gerçek e-posta gönderimi içermeyen geliştirme kutusudur. Menüsüz karşılığı `bash scripts/atolye.sh admin mailbox`; JSON için `--json` eklenir. [Kayıt, yönetici ve hesap onayı komutları](HESAP-ONAYI.md).

Panel Python 3'ün curses modülünü kullanır; Go sunucusu mevcut Go araçlarıyla derlenir. CachyOS/Linux hedeflenmiştir. Ek pip paketi veya kontrol HTTP sunucusu yoktur. Mevcut işletim sistemi kullanıcısının yetkileriyle çalışır, sudo çağırmaz.

**Kayıp MFA / operatör kurtarması** menüsü MFA açık bir hesabı seçip açık onayla sıfırlar. Tüm oturumlar ve eski kurtarma kodları iptal edilir. CLI karşılığı `bash scripts/atolye.sh admin mfa-reset --email 'adres@example.com'`. İlk yönetici listesi artık e-posta doğrulaması ve MFA kurulumu tamamlanmış adayları gösterir. `.control/mfa.key` geçici dosya değildir; silmeyin ve ayrı korumalı yedekleyin. [MFA ve kurtarma rehberi](MFA-VE-KURTARMA.md).

## Menüsüz kullanım

Yerel sunucuyu derleyip arka planda başlat, ardından durumunu göster:

```fish
bash scripts/atolye.sh system up --profile local
and bash scripts/atolye.sh system status --profile local --json
```

Güncellenmiş kodu derleyerek yeniden başlat:

```fish
bash scripts/atolye.sh system restart --profile local
and bash scripts/atolye.sh system logs --profile local
```

Durdur:

```fish
bash scripts/atolye.sh system down --profile local
```

Diğer işlemler: `system open`, `system logs`, `system backups`. `--json` otomasyon için sonucu `ok` ve `result`/`error` alanlarıyla döndürür. Başarı kodu 0, işlem hatası 1, hatalı komut kullanımı 2'dir. Loglar son bölümün anlık görüntüsüdür; yenilemek için yeniden çalıştırılır.

Yerel varsayılan adres `http://127.0.0.1:8090`, SQLite dosyası `data/gorevler.db` olur. Ortamdaki `ADDR`, `DB_PATH`, `DATABASE_URL` ve uygulamanın diğer ortam değişkenleri korunur; `.env` kendiliğinden yüklenmez. Adresi değiştirmek için Fish'te `set -gx ADDR 127.0.0.1:8091` kullanın. TUI üzerinden yerel başlatma yalnızca `127.0.0.1` adresine izin verir.

Süreç kimliği, derlenmiş sunucu ve yerel loglar `.control/` altında özel izinlerle tutulur. Port başka uygulamadaysa başlatma hata verir; başka süreç öldürülmez. Önceden `proje-baslat.sh` veya başka araçla açılmış sunucu bu panelin yönetimine otomatik alınmaz. “Yönetilen süreç yok” makinedeki bütün sunucuların kapalı olduğu anlamına gelmez. Çalışan sunucu varken `.control/` klasörünü silmeyin.

## Kubernetes

```fish
bash scripts/atolye.sh control tui --profile kind
# Veya:
bash scripts/atolye.sh system up --profile kind
and bash scripts/atolye.sh system status --profile kind --json
```

Bu profil mevcut `go-odev` kümesindeki `gorev-atolyesi` deployment'ını yönetir. İlk kurulum/imaj güncellemesi mevcut `bash scripts/proje-k8s.sh` ile yapılır. Menüdeki kind başlat/yeniden başlat işlemi yeni imaj derlemez; mevcut deployment'ı bir replikaya çıkarır. Durdurma mevcut güvenli `proje-kapat.sh` üzerinden yapılır; küme, volume ve veri silinmez. API erişilemiyorsa hata gösterilir, başarılı kapatma varsayılmaz.

Kind için port yönlendirmesini ayrı terminalde `bash scripts/proje-ac.sh` ile açın. TUI bu önceden açılmış terminalin port yönlendirmesini yönetmez. “Siteyi tarayıcıda aç” şu an yönetilen yerel sunucuya yöneliktir.

Yedek menüsü proje içindeki `backups/` dosyalarını listeler; uzak yedekleri ve geri yüklenebilirliği doğrulamaz. Migration çalıştırma, yedek geri yükleme ve production dağıtımı bu menüde sunulmaz. Web kullanıcı rolleri bu yerel işletim aracının yetkilerini belirlemez.

## Doğrulama

```fish
python3 -m unittest discover -s scripts -p 'test_atolye_control.py' -v
```

Süreç kimliği uyuşmazlığında sinyal göndermeme, doğru süreci kapatma, eşzamanlı işlem kilidi ve log temizleme testleri bulunur. Geliştirme sırasında ayrıca geçici DB ile gerçek Go sunucusunun HTTP 200 / kapatma / veri koruma akışı ve PTY üzerinden klavye, onay iptali, yeniden boyutlandırma ve terminal ayarlarının geri yüklenmesi doğrulandı. Gerçek Kubernetes üzerinde değiştirme testi yapılmadı.

Arayüzün terminal yönetimi için başvuru: [Python curses belgeleri](https://docs.python.org/3/library/curses.html).

### Kapatıp açarken port kontrolü

Port ön kontrolü `SO_REUSEADDR` kullanır; kapanmış bağlantıların `TIME_WAIT` kayıtları yeniden başlatmayı engellemez. Aktif bir dinleyici varsa işlem açıklayıcı hatayla durur; süreç öldürülmez ve port paylaşılmaz. Aynı portta üç ardışık gerçek Go başlat/HTTP/kapat döngüsü ve iki socket regresyon testiyle doğrulandı. Eski TUI oturumu açıksa `q` ile çıkıp paneli yeniden açarak düzeltmeyi yükleyin. [Python socket açıklaması](https://docs.python.org/3/library/socket.html#example).
