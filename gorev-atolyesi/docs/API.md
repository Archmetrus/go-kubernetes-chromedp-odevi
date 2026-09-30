# Görev API sözleşmesi

Taban adres: `http://127.0.0.1:8090`. Veri yazan istekler `Content-Type: application/json` kullanır. Görev listesi bu kişisel uygulamada bütünüyle döner; büyük veri kümeleri için sayfalama sonraki adımdır.

## Hesaplı sürüm ekleri

`AUTH_ENABLED=true` modunda görev uç noktaları oturum cookie'si gerektirir; yalnızca o kullanıcıya ait veriler döner. Oturumsuz istek 401, hız sınırı 429, çapraz kaynak yazma 403 döner. Yerel rehberin adresi 8091, kind yönlendirmesi 8090'dır; port veritabanı türünü belirlemez.

| Metot | Yol | Davranış |
|---|---|---|
| GET | `/api/session` | Giriş gerekliliği, kayıt açıklığı, kullanıcı ve sürüm |
| POST | `/api/register` | `{email,password}`; yalnızca kayıt açıksa, 201 |
| POST | `/api/login` | `{email,password}`; 200 ve HttpOnly oturum cookie'si |
| POST | `/api/logout` | Oturumu tüm replikalar için iptal eder, 204 |
| GET | `/api/tasks?scope=active\|archive\|trash` | Seçili alandaki kullanıcı görevleri; varsayılan active |
| POST | `/api/tasks/{id}/restore` | `{version}` ile çöp kutusundan geri alır, 200 |
| POST | `/api/import/tasks?preview=true` | Export dosyasını doğrular; kayıt yapmadan count/tasks döner |
| POST | `/api/import/tasks` | Aynı dosyayı transaction içinde ekler; 201 `{imported:N}` |
| GET | `/metrics` | Bearer METRICS_TOKEN ile Prometheus sayaçları |

Göreve `archived` (boolean) ve `deleted_at` (sunucu yönetir) eklendi. PUT `archived=true` ile arşivler. DELETE artık fiziksel silmez; `deleted_at` ve `updated_at` yazar, version artırır. Geri alma version'ı tekrar artırır. Başka kullanıcıya ait görev için 404 döner.

Export v1 zarfı korunur, görevlerde ek alanlar bulunur; aktif ve arşiv kayıtları dahil, çöp kutusu hariçtir. Import en fazla 4 MiB/1000 görev kabul eder, count ile dizi uzunluğu eşleşmelidir; yeni kimlik/sürüm ve zaman değerleri oluşturur. Tekrar import kopya oluşturur. Kullanıcı kimliği istek gövdesinden alınmaz.

| Metot | Yol | Başarılı cevap |
|---|---|---|
| GET | `/api/export/tasks` | 200, indirilebilir JSON dosyası |
| GET | `/api/tasks` | 200, görev dizisi (boşsa `[]`) |
| POST | `/api/tasks` | 201, oluşturulan görev |
| PUT | `/api/tasks/{id}` | 200, güncellenmiş görev |
| DELETE | `/api/tasks/{id}?version=1` | 204, boş gövde |
| GET | `/healthz` | 200, süreç canlı |
| GET | `/readyz` | 200, veritabanı erişilebilir; aksi halde 503 |

Oluşturma gövdesi:

```json
{
  "title": "Go testlerini tamamla",
  "description": "Hata senaryolarını da kapsa.",
  "category": "Go",
  "priority": "high",
  "status": "todo",
  "due_date": ""
}
```

Başlık 3–120, açıklama en fazla 2000 karakterdir. Kategori `Go`, `Kubernetes`, `chromedp`, `Genel`; öncelik `low`, `medium`, `high`; durum `todo`, `doing`, `done`. Tarih boş veya `YYYY-MM-DD` olmalıdır.

Sunucu `id`, `version`, `created_at`, `updated_at` ekler. PUT tüm kullanıcı alanlarını ve son okunan `version` değerini ister. Başarılı güncelleme sürümü bir artırır. DELETE de son sürümü ister.

Hatalar JSON `{ "error": "açıklama" }`: 400 geçersiz veri, 404 olmayan görev, 409 eski sürüm, 415 içerik türü, 500 depolama hatası. Standart HTTP katmanının 403 çapraz kaynak, 404 statik dosya ve 405 metot cevapları düz metin olabilir.

Fish ile örnek:

```fish
curl -i http://127.0.0.1:8090/api/tasks \
  -H 'Content-Type: application/json' \
  -d '{"title":"İlk gerçek görev","description":"","category":"Go","priority":"medium","status":"todo","due_date":""}'
curl http://127.0.0.1:8090/api/tasks
```

Sürüm çakışması: aynı görevi iki sekmede açın, birinde kaydedin, diğerinde eski veriyi kaydedin. İkinci istek 409 alır. Panoyu yenileyip yeniden düzenleyin.

## Tek tıkla JSON indirme

`GET /api/export/tasks` veritabanından güncel tam listeyi okur. `Content-Disposition: attachment` ve tarih-saat içeren `.json` dosya adı gönderir. Tarayıcı Blob oluşturmadan yerel indirme mekanizmasını kullanır. Dosyada `format`, `exported_at` (UTC), `count` ve `tasks` bulunur. Boş liste `tasks: []`, `count: 0` üretir. Hata durumunda dosya başlıkları yerine normal hata cevabı döner. Yanıt önbelleğe alınmaz.
