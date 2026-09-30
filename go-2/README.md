# 🚀 Sentetik Web İzleme, Kazıma & Kubernetes Operator Platformu

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.23-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go Version">
  <img src="https://img.shields.io/badge/Chromedp-Headless%20Chrome-4285F4?style=for-the-badge&logo=googlechrome&logoColor=white" alt="Chromedp">
  <img src="https://img.shields.io/badge/Scrape.do-Anti--Bot%20Proxy-F59E0B?style=for-the-badge" alt="Scrape.do">
  <img src="https://img.shields.io/badge/Kubernetes-Native%20Operator-326CE5?style=for-the-badge&logo=kubernetes&logoColor=white" alt="Kubernetes">
  <img src="https://img.shields.io/badge/Security-SSRF%20Guard-10B981?style=for-the-badge" alt="SSRF Guard">
</p>

---

## 📌 Proje Hakkında

**`k8s-chromedp-synthetics`**, modern web sitelerini kazımak (web scraping), dinamik JavaScript içeriklerini render etmek, görsel değişimleri (Visual Regression/Drift) tespit etmek ve Kubernetes kümesi üzerinde bildirimsel (declarative) sentetik web testleri koşturmak için geliştirilmiş **yüksek performanslı bir mikroservis ve Kubernetes Operator platformudur**.

Bu proje, hem **görsel bir Web Dashboard** üzerinden hem **REST API** endpoint'leri aracılığıyla hem de **Kubernetes Custom Resource (CRD)** nesneleri ile yönetilebilir.

---

## 🛠️ Kullanılan Teknolojiler ve Detaylı Mimarisi

Bu projede kullanılan temel teknolojiler, mimarideki rolleri ve sundukları avantajlar aşağıda detaylandırılmıştır:

### 1. 🦫 Go (Golang 1.23) — *Çekirdek Motor & Performans*
Projenin backend çekirdeği **Go** ile yazılmıştır. Go'nun sunduğu yüksek eşzamanlılık (concurrency - Goroutine ve Channels), düşük bellek footprint'i ve hızlı derleme süresi platformun ana omurgasını oluşturur.
- **HTTP REST Engine & Static Server:** Harici bir framework bağımlılığı olmadan, Go'nun `net/http` paketi ile optimize edilmiş API router ve statik UI sunucusu.
- **SSRF Protection Guard (`pkg/security`):** Dışarıdan gelen web kazıma isteklerinin iç ağdaki özel IP adreslerine (`127.0.0.1`, `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`) ve Cloud Metadata servislerine (`169.254.169.254`) erişmesini engelleyen güvenlik duvarı.
- **Visual Diff & Image Delta Engine (`pkg/diff`):** Saf Go `image` paketi kullanılarak iki ekran görüntüsü arasındaki piksel bazlı renk farkını (RGB Delta) hesaplayan ve görsel sapmayı (Visual Drift %) bulan grafik motoru.
- **Prometheus Metrik Exporter (`pkg/metrics`):** Test süreleri, başarı/başarısızlık oranları ve görsel kayma yüzdelerini Prometheus uyumlu `/metrics` endpoint'inde sunar.

---

### 2. 🌐 Chromedp — *Headless Browser Engine (Chrome DevTools Protocol)*
[Chromedp](https://github.com/chromedp/chromedp), Go dili için geliştirilmiş en popüler Chrome DevTools Protocol (CDP) kütüphanesidir. Selenium veya Puppeteer gibi ağır Node.js/Java sürücülerine ihtiyaç duymadan doğrudan Chromium ile haberleşir.
- **Tam JS Rendering:** React, Vue, Angular ve Next.js gibi Single Page Application (SPA) mimarisindeki sitelerin JavaScript kodlarını çalıştırıp DOM'un son halini elde eder.
- **Ekran Görüntüsü & PDF:** Tam sayfa yüksek çözünürlüklü ekran görüntüsü (`FullScreenshot`) ve CSS uyumlu PDF rapor çıktısı (`PrintToPDF`) üretir.
- **Resim & Kaynak Engelleme (`blockImages`):** `cdproto/network` paketi seviyesinde resim, font ve stil isteklerini engelleyerek bant genişliğini korur ve tarama hızını 3x katına çıkarır.
- **Çok Adımlı Senaryo Çalıştırıcı:** `Navigate`, `Click`, `Type`, `WaitVisible`, `ScrapeText`, `ExtractTable` ve `EvaluateJS` gibi adımları sırayla yürüterek form doldurma ve uçtan uca kullanıcı simülasyonu yapar.

---

### 3. 🛡️ Scrape.do — *Proxy API & Anti-Bot Bypass Integration*
[Scrape.do](https://scrape.do/), Cloudflare, Akamai, DataDome gibi gelişmiş bot koruma sistemlerini ve CAPTCHA engelleyicilerini aşmak için kullanılan proxy ağ servisidir.
- **Dönen Proxy Altyapısı (Rotating Residential Proxies):** Hedef sitelerin IP engelini aşarak kesintisiz kazıma sağlar.
- **Esnek Parametre Haritalaması:**
  - `render=true`: JS ağırlıklı siteleri Scrape.do bulutundaki Chrome ile çalıştırır.
  - `blockResources=true`: Görselleri engelleyerek proxy maliyetini ve yanıt süresini düşürür.
  - `fullScreenShot=true&returnJSON=true`: Görsel ile birlikte HTML çıktısını tek yanıt paketinde Base64 kodlamasıyla alır.
- **Otomatik Token Yönetimi:** Kullanıcı arayüzünde LocalStorage'a kaydolur ve istek gövdesinde veya çevre değişkeninde (`SCRAPE_DO_TOKEN`) otomatik olarak taşınır.

---

### 4. ☸️ Kubernetes (K8s) Operator & Custom Resource Definitions (CRD)
Platform, Kubernetes kümesinde **Native Operator** olarak çalışacak şekilde tasarlanmıştır.
- **Custom Resource Definition (`SyntheticTest`):** `monitoring.k8s.io/v1alpha1` API grubu altında tanımlanan deklaratif test dosyalarıdır.
- **Dynamic Informer & Reconciler Loop (`k8s.io/client-go`):** Kubernetes API server'ı sürekli dinler. Yeni bir `SyntheticTest` manifesti uygulandığında (`kubectl apply`), otomatik olarak test senaryosunu arka planda başlatır.
- **Status Reconciliation:** Test sonuçlarını, geçen süreyi ve görsel kayma (`visualDrift`) yüzdesini doğrudan Kubernetes nesnesinin `status` alanına yazar (`kubectl get synthetictest`).
- **Esnek Çalışma Modları (In-Cluster / Standalone):** Küme içerisinde çalışırken ServiceAccount RBAC haklarını kullanır; küme dışında veya lokal geliştirmede ise otomatik olarak Standalone moda geçerek çökmeden çalışmaya devam eder.

---

### 5. 🎨 Modern Web UI — *Glassmorphism Dashboard*
- **Vanilla HTML/CSS/JavaScript:** Ağır frontend framework bağımlılıkları olmadan ultra hızlı yüklenen modern koyu tema (Dark Mode) arayüz.
- **3 Farklı Kazıma Modu:** `Chromedp (Full JS)`, `Fast cURL (Statik HTML)` ve `Scrape.do (Proxy API)` modları arasında tek tıkla geçiş.
- **Visual Diff Slider:** Referans (baseline) ve yeni alınan ekran görüntüsünü çubuk ile canlı olarak karşılaştıran arayüz.
- **HTML Önizleme & Kaynak Kodu:** Kazınan sayfayı güvenli bir `iframe` içerisinde canlı önizleme veya saf HTML kaynak kodunu görüntüleme seçeneği.
- **Görsel Lightbox & Toast Sistem:** Bildirim ve görseller için modern kullanıcı deneyimi.

---

## 🏗️ Mimari Şema

```
                                  +-----------------------+
                                  |   Kullanıcı / İstemci |
                                  +-----------+-----------+
                                              |
                   +--------------------------+--------------------------+
                   |                          |                          |
                   v                          v                          v
        +--------------------+      +-------------------+      +--------------------+
        | Web Dashboard UI   |      | REST API (cURL)   |      | kubectl apply -f   |
        | (Glassmorphism JS) |      | /api/v1/scrape    |      | SyntheticTest CRD  |
        +----------+---------+      +---------+---------+      +----------+---------+
                   |                          |                          |
                   +--------------------------+--------------------------+
                                              |
                                              v
                              +-------------------------------+
                              |   Go HTTP Server & Security   |
                              |   (SSRF Guard, Auth, Limiter) |
                              +---------------+---------------+
                                              |
             +--------------------------------+--------------------------------+
             |                                |                                |
             v                                v                                v
  +----------------------+        +----------------------+        +----------------------+
  | Local Chromedp Engine|        | Scrape.do Proxy API  |        | Fast cURL Scraper    |
  | (Headless Chrome JS) |        | (Anti-Bot Bypass)    |        | (Static HTML - ms)   |
  +----------+-----------+        +----------+-----------+        +----------+-----------+
             |                               |                               |
             +-------------------------------+-------------------------------+
                                              |
                                              v
                              +-------------------------------+
                              | Storage & Visual Diff Engine  |
                              | (Screenshots, HTML, Metrics)  |
                              +-------------------------------+
```

---

## ⚡ 3 Farklı Kazıma Modu Karşılaştırması

| Yöntem (`mode`) | Çalışma Prensibi | Yanıt Hızı | JS Render Desteği | Kullanım Amacı |
| :--- | :--- | :---: | :---: | :--- |
| **`chromedp`** | Yerel Headless Chrome çalıştırır | Normal (1-3s) | ✅ Var | Dinamik SPA siteleri, ekran görüntüsü ve PDF alma |
| **`fast_curl`** | Doğrudan HTTP GET isteği atar | Ultra Hızlı (ms) | ❌ Yok | Statik HTML çekme, SEO analizi ve hızlı içerik kontrolü |
| **`scrape_do`** | Scrape.do bulut proxy ağı üzerinden geçer | Yüksek (2-5s) | ✅ Var | Cloudflare / CAPTCHA korumalı zorlu siteleri aşma |

---

## 🚀 Hızlı Başlangıç

### Gereksinimler
- **Go** 1.23 veya üzeri
- **Google Chrome** / **Chromium** (Chromedp modunu lokalde çalıştırmak için)
- *(Opsiyonel)* **Kubernetes Kümesi** & `kubectl` (CRD Operator modunda çalıştırmak için)

---

### 1. Uygulamayı Derleme ve Çalıştırma

```bash
# Bağımlılıkları indirin ve projeyi derleyin
make build

# Web sunucusunu başlatın
make run-server
```

Sunucu çalıştıktan sonra tarayıcınızdan aşağıdaki adreslere erişebilirsiniz:
- 📊 **Web Dashboard:** [http://localhost:8080](http://localhost:8080)
- 🌐 **Kazıma API Endpoint:** `POST http://localhost:8080/api/v1/scrape`
- 📈 **Prometheus Metrikleri:** [http://localhost:8080/metrics](http://localhost:8080/metrics)
- 🩺 **Sağlık Durumu:** [http://localhost:8080/api/v1/healthz](http://localhost:8080/api/v1/healthz)

---

### 2. Örnek API İstekleri (cURL)

#### A. Chromedp ile Resimsiz Kazıma:
```bash
curl -X POST http://localhost:8080/api/v1/scrape \
  -H "Content-Type: application/json" \
  -H "X-API-Key: dev-secret-key" \
  -d '{
    "url": "https://example.com",
    "mode": "chromedp",
    "blockImages": true,
    "scrollToBottom": true,
    "includeScreenshot": true
  }'
```

#### B. Scrape.do API ile Bot Korumasını Aşma:
```bash
curl -X POST http://localhost:8080/api/v1/scrape \
  -H "Content-Type: application/json" \
  -H "X-API-Key: dev-secret-key" \
  -d '{
    "url": "https://target-website.com",
    "mode": "scrape_do",
    "scrapeDoToken": "YOUR_SCRAPE_DO_TOKEN",
    "blockImages": true
  }'
```

---

### 3. Kubernetes Operator Dağıtımı

Kubernetes kümenizde deklaratif test tanımlamak için manifestleri uygulayabilirsiniz:

```bash
# CRD, RBAC ve Deployment manifestlerini yükleyin
make k8s-apply

# Sentetik testlerin durumunu kontrol edin
kubectl get synthetictests
```

#### Örnek `SyntheticTest` Custom Resource (`sample-test.yaml`):

```yaml
apiVersion: monitoring.k8s.io/v1alpha1
kind: SyntheticTest
metadata:
  name: ecommerce-checkout-monitor
  namespace: default
spec:
  url: "https://example.com"
  schedule: "*/5 * * * *"
  visualRegression: true
  driftThreshold: 2.5
  viewport:
    width: 1920
    height: 1080
  steps:
    - name: "Ana Sayfaya Git"
      action: "navigate"
      target: "https://example.com"
    - name: "Arama Çubuğuna Yaz"
      action: "type"
      target: "input[name='q']"
      value: "laptop"
    - name: "Başlığı Oku"
      action: "scrape_text"
      target: "h1"
```

---

## 📁 Proje Dizin Yapısı

```
.
├── cmd/
│   ├── operator/main.go         # Kubernetes Operator (CRD Controller) giriş noktası
│   └── server/main.go           # REST API & Web Dashboard sunucusu
├── pkg/
│   ├── api/api.go               # HTTP API route ve handler'ları
│   ├── controller/controller.go # Kubernetes client-go Informer & Reconciler döngüsü
│   ├── diff/diff.go             # Go native görsel karşılaştırma & piksel delta motoru
│   ├── metrics/metrics.go       # Prometheus metrik kayıtları
│   ├── runner/runner.go         # Chromedp, Fast cURL ve Scrape.do kazıma motoru
│   ├── security/security.go     # SSRF Guard, Rate Limiting & Auth katmanı
│   ├── store/store.go           # Dosya ve görsel depolama yöneticisi
│   └── types/types.go           # Veri tipleri ve CRD şeması
├── static/                      # Web Dashboard (SPA)
│   ├── index.html               # UI HTML yapısı
│   ├── css/style.css            # Custom Vanilla CSS & Glassmorphism teması
│   └── js/app.js                # Dinamik UI, iframe önizleme & visual diff JS kodları
├── deployments/                 # Kubernetes Manifestleri
│   ├── crds/synthetictest.yaml  # SyntheticTest CustomResourceDefinition
│   ├── samples/                 # Örnek CRD test manifestleri
│   ├── deployment.yaml          # K8s Deployment & Service
│   └── rbac.yaml                # ServiceAccount & ClusterRole tanımları
├── Dockerfile                   # Chromium + Go multi-stage build Dockerfile
├── Makefile                     # Derleme ve test otomasyonu
└── README.md                    # Detaylı İnsan Okunabilir Dokümantasyon
```

---

## 🔒 Güvenlik Kontrolleri

1. **SSRF Koruması:** Özel ve yerel IP aralıklarına (`127.0.0.1`, `10.0.0.0/8`, `169.254.169.254` vb.) yönelen istekler tespit edilerek anında engellenir.
2. **API Key Kimlik Doğrulaması:** Üretim ortamında `X-API-Key` başlığı kontrol edilir.
3. **Hardened Chromium Sandbox:** Chrome işlemleri kısıtlı sistem izinleriyle başlatılır.
