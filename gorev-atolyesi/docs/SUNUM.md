# 5–7 dakikalık proje sunumu

1. **Problem:** Ders ve proje çalışmalarını tek bir panoda takip etmek.
2. **Canlı kullanım:** Yeni görev ekleyin, kategori ve öncelik seçin. Durumu değiştirin. Arama ve filtreleri deneyin.
3. **Go:** HTTP yönlendirme, struct, JSON, hata yönetimi ve context kavramlarını `internal/tasks` üzerinden anlatın.
4. **Veritabanı:** Uygulamayı durdurup açın; görevlerin kaldığını gösterin. Parametreli SQL ve `version` alanını açıklayın.
5. **chromedp:** `bash scripts/proje-kontrol.sh` ile tarayıcı kabul testini çalıştırın; ekran görüntülerini açın.
6. **Kubernetes:** Pod, Service, ConfigMap ve PVC'yi gösterin. Readiness/liveness farkını açıklayın. Tek replika seçiminin SQLite ile ilişkisini anlatın.
7. **Sınırlar:** Bu sürüm kişisel kullanım içindir. Sonraki geliştirmeler kullanıcı hesabı, PostgreSQL, sayfalama, erişim kontrolü ve dış ortam için HTTPS olabilir.

Sorulabilecek sorular:

- Neden Go? API, dosyaların gömülmesi ve testler tek dil/derleme altında.
- Neden SQLite? Kişisel uygulama için ek sunucu gerektirmeyen kalıcı veri.
- Neden tek Pod? Yerel veritabanı dosyasının bir uygulama örneği tarafından yönetilmesi.
- Neden `version`? Eşzamanlı düzenlemelerde veri kaybını önlemek.
- Unit/integration/E2E farkı? Bu projede gerçek geçici DB kullanan entegrasyon testleri ve tarayıcıdan tam akışı sınayan E2E testi var.
