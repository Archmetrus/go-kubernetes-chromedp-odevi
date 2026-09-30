document.addEventListener('DOMContentLoaded', () => {

    // ═══════════ TOAST NOTIFICATION SYSTEM ═══════════
    const toastContainer = document.getElementById('toastContainer');
    function showToast(message, type = 'info') {
        const icons = { success: '✓', error: '✕', info: 'ℹ' };
        const toast = document.createElement('div');
        toast.className = `toast toast-${type}`;
        toast.innerHTML = `<span class="toast-icon">${icons[type] || icons.info}</span><span>${message}</span>`;
        toastContainer.appendChild(toast);
        setTimeout(() => {
            toast.classList.add('toast-out');
            setTimeout(() => toast.remove(), 200);
        }, 4000);
    }

    // ═══════════ LIGHTBOX ═══════════
    const lightboxModal = document.getElementById('lightboxModal');
    const lightboxImage = document.getElementById('lightboxImage');
    const lightboxClose = lightboxModal?.querySelector('.lightbox-close');
    const lightboxBackdrop = lightboxModal?.querySelector('.lightbox-backdrop');

    function openLightbox(src) {
        if (!lightboxModal || !lightboxImage) return;
        lightboxImage.src = src;
        lightboxModal.classList.add('open');
    }

    function closeLightbox() {
        if (lightboxModal) lightboxModal.classList.remove('open');
    }

    if (lightboxClose) lightboxClose.addEventListener('click', closeLightbox);
    if (lightboxBackdrop) lightboxBackdrop.addEventListener('click', closeLightbox);
    document.addEventListener('keydown', (e) => {
        if (e.key === 'Escape') closeLightbox();
    });

    // ═══════════ NAVIGATION TABS ═══════════
    const navItems = document.querySelectorAll('.nav-item');
    const tabContents = document.querySelectorAll('.tab-content');
    const pageTitle = document.getElementById('pageTitle');

    const titles = {
        'dashboard': 'Web Scraping Dashboard',
        'visual-diff': 'Görsel Regresyon & Diff Slider'
    };

    navItems.forEach(item => {
        item.addEventListener('click', (e) => {
            e.preventDefault();
            const targetTab = item.getAttribute('data-tab');
            navItems.forEach(n => n.classList.remove('active'));
            tabContents.forEach(t => t.classList.remove('active'));
            item.classList.add('active');
            const targetEl = document.getElementById(`tab-${targetTab}`);
            if (targetEl) targetEl.classList.add('active');
            if (titles[targetTab] && pageTitle) pageTitle.innerText = titles[targetTab];
        });
    });

    // ═══════════ WEBSOCKET ═══════════
    const wsStatus = document.getElementById('wsStatus');
    const liveLogs = document.getElementById('liveLogs');
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/ws/live`;

    let socket;
    function connectWebSocket() {
        socket = new WebSocket(wsUrl);

        socket.onopen = () => {
            if (wsStatus) {
                wsStatus.innerText = 'Bağlandı';
                if (wsStatus.previousElementSibling) {
                    wsStatus.previousElementSibling.style.backgroundColor = '#22c55e';
                }
            }
            appendLog('[SİSTEM] WebSocket bağlandı.');
        };

        socket.onmessage = (event) => {
            try {
                const data = JSON.parse(event.data);
                appendLog(`[AKIŞ] ${data.type}: ${JSON.stringify(data)}`);
            } catch (err) {
                appendLog(`[MESAJ] ${event.data}`);
            }
        };

        socket.onclose = () => {
            if (wsStatus) {
                wsStatus.innerText = 'Kesildi';
                if (wsStatus.previousElementSibling) {
                    wsStatus.previousElementSibling.style.backgroundColor = '#ef4444';
                }
            }
            setTimeout(connectWebSocket, 3000);
        };
    }
    connectWebSocket();

    function appendLog(text) {
        if (!liveLogs) return;
        const line = document.createElement('div');
        line.className = 'log-line';
        line.innerText = `[${new Date().toLocaleTimeString()}] ${text}`;
        liveLogs.appendChild(line);
        liveLogs.scrollTop = liveLogs.scrollHeight;
    }

    // ═══════════ LOADING STATE ═══════════
    const loadingOverlay = document.getElementById('loadingOverlay');
    const loadingTimer = document.getElementById('loadingTimer');
    let timerInterval = null;
    let scrapeStartTime = 0;

    function showLoading() {
        if (loadingOverlay) loadingOverlay.classList.remove('hidden');
        scrapeStartTime = performance.now();
        timerInterval = setInterval(() => {
            const elapsed = ((performance.now() - scrapeStartTime) / 1000).toFixed(1);
            if (loadingTimer) loadingTimer.textContent = `${elapsed}s`;
        }, 100);
    }

    function hideLoading() {
        if (loadingOverlay) loadingOverlay.classList.add('hidden');
        if (timerInterval) {
            clearInterval(timerInterval);
            timerInterval = null;
        }
    }

    function getElapsedTime() {
        return ((performance.now() - scrapeStartTime) / 1000).toFixed(1);
    }

    function formatBytes(bytes) {
        if (bytes < 1024) return bytes + ' B';
        if (bytes < 1048576) return (bytes / 1024).toFixed(1) + ' KB';
        return (bytes / 1048576).toFixed(1) + ' MB';
    }

    // ═══════════ SCRAPE METRICS ═══════════
    const metricDuration = document.getElementById('metricDuration');
    const metricSize = document.getElementById('metricSize');
    const metricMode = document.getElementById('metricMode');
    const metricStatus = document.getElementById('metricStatus');

    const modeLabels = {
        'chromedp': 'Chromedp',
        'fast_curl': 'cURL',
        'scrape_do': 'Scrape.do'
    };

    function updateMetrics(duration, htmlContent, mode) {
        if (metricDuration) metricDuration.textContent = duration + 's';
        if (metricSize) metricSize.textContent = htmlContent ? formatBytes(new Blob([htmlContent]).size) : '—';
        if (metricMode) metricMode.textContent = modeLabels[mode] || mode;
        if (metricStatus) metricStatus.textContent = 'Tamamlandı';
    }

    // ═══════════ IFRAME PREVIEW & SOURCE TOGGLE ═══════════
    const btnTogglePreview = document.getElementById('btnTogglePreview');
    const btnToggleSource = document.getElementById('btnToggleSource');
    const iframePreviewContainer = document.getElementById('iframePreviewContainer');
    const htmlPreviewFrame = document.getElementById('htmlPreviewFrame');
    const sourceCodeContainer = document.getElementById('sourceCodeContainer');

    let lastHtmlContent = '';

    if (btnTogglePreview) {
        btnTogglePreview.addEventListener('click', () => {
            const isVisible = !iframePreviewContainer.classList.contains('hidden');
            if (isVisible) {
                iframePreviewContainer.classList.add('hidden');
                btnTogglePreview.textContent = '🌐 HTML Önizle';
            } else {
                iframePreviewContainer.classList.remove('hidden');
                if (sourceCodeContainer) sourceCodeContainer.classList.add('hidden');
                if (btnToggleSource) btnToggleSource.textContent = '{ } Kaynak Kodu';
                if (htmlPreviewFrame && lastHtmlContent) {
                    htmlPreviewFrame.srcdoc = lastHtmlContent;
                }
                btnTogglePreview.textContent = '🌐 Önizlemeyi Kapat';
            }
        });
    }

    if (btnToggleSource) {
        btnToggleSource.addEventListener('click', () => {
            const isVisible = !sourceCodeContainer.classList.contains('hidden');
            if (isVisible) {
                sourceCodeContainer.classList.add('hidden');
                btnToggleSource.textContent = '{ } Kaynak Kodu';
            } else {
                sourceCodeContainer.classList.remove('hidden');
                if (iframePreviewContainer) iframePreviewContainer.classList.add('hidden');
                if (btnTogglePreview) btnTogglePreview.textContent = '🌐 HTML Önizle';
                btnToggleSource.textContent = '{ } Kaynağı Kapat';
            }
        });
    }

    // ═══════════ COMPARE IN VISUAL DIFF ═══════════
    let lastScreenshotUrl = '';
    let previousScreenshotUrl = '';

    const btnCompareInDiff = document.getElementById('btnCompareInDiff');
    if (btnCompareInDiff) {
        btnCompareInDiff.addEventListener('click', () => {
            if (!lastScreenshotUrl) {
                showToast('Karşılaştırma için henüz ekran görüntüsü yok.', 'info');
                return;
            }

            // Set images in visual diff
            const currentImg = document.getElementById('currentImg');
            if (currentImg) currentImg.src = lastScreenshotUrl;

            if (previousScreenshotUrl) {
                const baselineImg = document.getElementById('baselineImg');
                if (baselineImg) baselineImg.src = previousScreenshotUrl;
            }

            // Switch to visual diff tab
            navItems.forEach(n => n.classList.remove('active'));
            tabContents.forEach(t => t.classList.remove('active'));
            const diffTab = document.querySelector('[data-tab="visual-diff"]');
            const diffSection = document.getElementById('tab-visual-diff');
            if (diffTab) diffTab.classList.add('active');
            if (diffSection) diffSection.classList.add('active');
            if (pageTitle) pageTitle.innerText = titles['visual-diff'];

            showToast('Ekran görüntüleri karşılaştırma alanına yüklendi.', 'success');
        });
    }

    // ═══════════ INSTANT CAPTURE ═══════════
    const btnInstantCapture = document.getElementById('btnInstantCapture');
    const instantUrlInput = document.getElementById('instantUrlInput');
    const previewImage = document.getElementById('previewImage');
    const screenshotContainer = document.getElementById('screenshotContainer');

    if (btnInstantCapture && instantUrlInput) {
        btnInstantCapture.addEventListener('click', async () => {
            const url = instantUrlInput.value.trim();
            if (!url) return showToast('Lütfen geçerli bir URL giriniz.', 'error');

            btnInstantCapture.disabled = true;
            btnInstantCapture.innerText = 'Taranıyor...';
            appendLog(`[İSTEK] Ekran görüntüsü: ${url}`);

            try {
                const resp = await fetch('/api/v1/capture', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                        'X-API-Key': 'dev-secret-key'
                    },
                    body: JSON.stringify({ url: url })
                });

                const data = await resp.json();
                if (data.screenshotUrl && previewImage) {
                    previewImage.src = data.screenshotUrl;
                    previewImage.classList.remove('hidden');
                    if (screenshotContainer) {
                        screenshotContainer.querySelector('.placeholder-text')?.classList.add('hidden');
                    }
                    appendLog(`[OK] Ekran görüntüsü alındı: ${data.screenshotUrl}`);
                    showToast('Ekran görüntüsü alındı.', 'success');

                    previousScreenshotUrl = lastScreenshotUrl;
                    lastScreenshotUrl = data.screenshotUrl;

                    const currentImg = document.getElementById('currentImg');
                    if (currentImg) currentImg.src = data.screenshotUrl;
                } else if (data.error) {
                    appendLog(`[HATA] ${data.error}`);
                    showToast(data.error, 'error');
                }
            } catch (err) {
                appendLog(`[HATA] ${err.message}`);
                showToast('Bağlantı hatası: ' + err.message, 'error');
            } finally {
                btnInstantCapture.disabled = false;
                btnInstantCapture.innerText = 'SS Al';
            }
        });
    }

    // ═══════════ SCREENSHOT LIGHTBOX ═══════════
    if (previewImage) {
        previewImage.addEventListener('click', () => {
            if (previewImage.src && !previewImage.classList.contains('hidden')) {
                openLightbox(previewImage.src);
            }
        });
    }

    // ═══════════ SCRAPE.DO TOKEN ═══════════
    const selectScrapeMode = document.getElementById('selectScrapeMode');
    const containerScrapeDoToken = document.getElementById('containerScrapeDoToken');
    const inputScrapeDoToken = document.getElementById('inputScrapeDoToken');

    if (inputScrapeDoToken) {
        inputScrapeDoToken.value = localStorage.getItem('scrape_do_token') || '';
        inputScrapeDoToken.addEventListener('input', (e) => {
            localStorage.setItem('scrape_do_token', e.target.value.trim());
        });
    }

    function ensureScrapeDoToken() {
        let token = inputScrapeDoToken ? inputScrapeDoToken.value.trim() : '';
        if (!token) {
            token = prompt('Scrape.do API Token\'ınızı giriniz:');
            if (token) {
                token = token.trim();
                if (inputScrapeDoToken) inputScrapeDoToken.value = token;
                localStorage.setItem('scrape_do_token', token);
            }
        }
        return token;
    }

    if (selectScrapeMode && containerScrapeDoToken) {
        selectScrapeMode.addEventListener('change', () => {
            if (selectScrapeMode.value === 'scrape_do') {
                containerScrapeDoToken.classList.remove('hidden');
                ensureScrapeDoToken();
            } else {
                containerScrapeDoToken.classList.add('hidden');
            }
        });
    }

    // ═══════════ FULL SCRAPE ═══════════
    const btnFullScrape = document.getElementById('btnFullScrape');
    const scrapeDownloadBar = document.getElementById('scrapeDownloadBar');
    const linkDownloadHtml = document.getElementById('linkDownloadHtml');
    const linkDownloadPdf = document.getElementById('linkDownloadPdf');
    const linkDownloadPng = document.getElementById('linkDownloadPng');
    const scrapedHtmlContent = document.getElementById('scrapedHtmlContent');

    if (btnFullScrape && instantUrlInput) {
        btnFullScrape.addEventListener('click', async () => {
            const url = instantUrlInput.value.trim();
            if (!url) return showToast('Lütfen geçerli bir URL giriniz.', 'error');

            const mode = selectScrapeMode?.value || 'chromedp';
            let scrapeDoToken = '';

            if (mode === 'scrape_do') {
                scrapeDoToken = ensureScrapeDoToken();
                if (!scrapeDoToken) {
                    return showToast('Scrape.do API Token girilmediği için iptal edildi.', 'error');
                }
            }

            const blockImages = (document.getElementById('selectImageMode')?.value === 'block_images');
            const waitUntil = document.getElementById('selectWaitStrategy')?.value || 'domReady';
            const scrollToBottom = document.getElementById('checkScrollBottom')?.checked || false;
            const includePdf = document.getElementById('checkIncludePdf')?.checked || false;

            btnFullScrape.disabled = true;
            btnInstantCapture.disabled = true;
            showLoading();
            appendLog(`[İSTEK] Site kazıma (${modeLabels[mode]}, Resimsiz: ${blockImages}): ${url}`);

            try {
                const resp = await fetch('/api/v1/scrape', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                        'X-API-Key': 'dev-secret-key'
                    },
                    body: JSON.stringify({
                        url: url,
                        mode: mode,
                        scrapeDoToken: scrapeDoToken,
                        blockImages: blockImages,
                        waitUntil: waitUntil,
                        scrollToBottom: scrollToBottom,
                        includeScreenshot: !blockImages || mode === 'chromedp',
                        includePDF: includePdf,
                        includeHTML: true
                    })
                });

                const elapsed = getElapsedTime();
                const data = await resp.json();

                if (data.status === 'COMPLETED') {
                    // Update metrics
                    updateMetrics(elapsed, data.htmlContent, mode);

                    // Screenshot
                    if (data.screenshotUrl && previewImage) {
                        previewImage.src = data.screenshotUrl;
                        previewImage.classList.remove('hidden');
                        if (screenshotContainer) {
                            screenshotContainer.querySelector('.placeholder-text')?.classList.add('hidden');
                        }
                        if (linkDownloadPng) {
                            linkDownloadPng.href = data.screenshotUrl;
                            linkDownloadPng.style.display = 'inline-flex';
                        }
                        previousScreenshotUrl = lastScreenshotUrl;
                        lastScreenshotUrl = data.screenshotUrl;

                        const currentImg = document.getElementById('currentImg');
                        if (currentImg) currentImg.src = data.screenshotUrl;
                    } else if (blockImages) {
                        if (previewImage) previewImage.classList.add('hidden');
                        if (linkDownloadPng) linkDownloadPng.style.display = 'none';
                        if (screenshotContainer) {
                            const pText = screenshotContainer.querySelector('.placeholder-text');
                            if (pText) {
                                pText.innerText = '🚫 Resimsiz mod — görsel engellendi.';
                                pText.classList.remove('hidden');
                            }
                        }
                    }

                    // Download links
                    if (data.htmlUrl && linkDownloadHtml) linkDownloadHtml.href = data.htmlUrl;
                    if (data.pdfUrl && linkDownloadPdf) linkDownloadPdf.href = data.pdfUrl;

                    // Store HTML for preview
                    if (data.htmlContent) {
                        lastHtmlContent = data.htmlContent;
                        if (scrapedHtmlContent) scrapedHtmlContent.innerText = data.htmlContent;
                    }

                    // Show result bar
                    if (scrapeDownloadBar) scrapeDownloadBar.classList.remove('hidden');

                    // Reset toggles
                    if (iframePreviewContainer) iframePreviewContainer.classList.add('hidden');
                    if (sourceCodeContainer) sourceCodeContainer.classList.add('hidden');
                    if (btnTogglePreview) btnTogglePreview.textContent = '🌐 HTML Önizle';
                    if (btnToggleSource) btnToggleSource.textContent = '{ } Kaynak Kodu';

                    appendLog(`[OK] Site indirildi — ${elapsed}s`);
                    showToast(`Tamamlandı — ${elapsed}s`, 'success');
                } else if (data.error) {
                    appendLog(`[HATA] ${data.error}`);
                    showToast(data.error, 'error');
                }
            } catch (err) {
                appendLog(`[HATA] ${err.message}`);
                showToast('Bağlantı hatası: ' + err.message, 'error');
            } finally {
                hideLoading();
                btnFullScrape.disabled = false;
                btnFullScrape.innerText = '📥 Siteyi İndir & SS Al';
                btnInstantCapture.disabled = false;
            }
        });
    }

    // ═══════════ VISUAL DIFF SLIDER ═══════════
    const sliderHandle = document.getElementById('sliderHandle');
    const comparisonSlider = document.getElementById('comparisonSlider');

    if (sliderHandle && comparisonSlider) {
        const updateSlider = (val) => {
            comparisonSlider.style.setProperty('--clip-pos', `${val}%`);
        };
        sliderHandle.addEventListener('input', (e) => updateSlider(e.target.value));

        let isDragging = false;
        comparisonSlider.addEventListener('pointerdown', (e) => {
            isDragging = true;
            comparisonSlider.setPointerCapture(e.pointerId);
            const rect = comparisonSlider.getBoundingClientRect();
            const val = Math.max(0, Math.min(100, ((e.clientX - rect.left) / rect.width) * 100));
            sliderHandle.value = val;
            updateSlider(val);
        });
        comparisonSlider.addEventListener('pointermove', (e) => {
            if (!isDragging) return;
            const rect = comparisonSlider.getBoundingClientRect();
            const val = Math.max(0, Math.min(100, ((e.clientX - rect.left) / rect.width) * 100));
            sliderHandle.value = val;
            updateSlider(val);
        });
        comparisonSlider.addEventListener('pointerup', () => { isDragging = false; });
        comparisonSlider.addEventListener('pointercancel', () => { isDragging = false; });
    }

    // ═══════════ DIFF IMAGE UPDATE ═══════════
    const btnUpdateDiffImages = document.getElementById('btnUpdateDiffImages');
    const inputBaselineUrl = document.getElementById('inputBaselineUrl');
    const inputCurrentUrl = document.getElementById('inputCurrentUrl');

    if (btnUpdateDiffImages) {
        btnUpdateDiffImages.addEventListener('click', () => {
            const bUrl = inputBaselineUrl ? inputBaselineUrl.value.trim() : '';
            const cUrl = inputCurrentUrl ? inputCurrentUrl.value.trim() : '';

            if (bUrl) {
                const bImg = document.getElementById('baselineImg');
                if (bImg) bImg.src = bUrl;
            }
            if (cUrl) {
                const cImg = document.getElementById('currentImg');
                if (cImg) cImg.src = cUrl;
            }
            if (!bUrl && !cUrl) {
                showToast('En az bir görsel URL giriniz.', 'info');
            } else {
                showToast('Görseller güncellendi.', 'success');
            }
        });
    }
});
