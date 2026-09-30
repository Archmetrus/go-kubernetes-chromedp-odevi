# Görev Atölyesi tasarım kararı

## Konu ve kullanım

Go, Kubernetes ve chromedp öğrenirken gerçek işleri takip eden kişisel çalışma alanı. Ana iş: bir görevi bulmak, düzenlemek ve tamamlamak. Tasarımın konusu bir tanıtım sitesi değil, kişinin çalışma tezgâhı.

## Kısa tasarım sistemi

- Kâğıt mavisi: `#E8EEF3`, çalışma zemini.
- Mürekkep: `#16374A`, metin ve marka.
- Porselen: `#FAFCFD`, görev yüzeyi.
- Petrol: `#19746B`, birincil eylem ve tamamlanma.
- Kehribar: `#DEB665`, devam eden işlerin işareti.
- Çelik: `#647D8D`, yardımcı metin ve sınırlar.
- Bricolage Grotesque: başlık ve marka; Manrope: görevler, form ve gezinme. Yazı tipleri yerel dosyalardan sunulur.
- Boyutlar: 12 / 14 / 16 / 20 / 28 / 40. Başlıklar ve metin sola hizalı; sayılar sekmeli rakamlarla.

## İki yerleşim fikri

```
A: Genel gösterge paneli
menü | büyük başlık + dört kart
     | takvim
     | görevler

B: Çalışma tezgâhı (seçilen)
menü | başlık + yeni görev / JSON indir
     | küçük ilerleme satırı + yedi gün
     | görünüm ve filtreler
     | bekleyen tepsisi | çalışma tepsisi | tamamlanan tepsisi
     | başlangıç şablonları
```

## Plan eleştirisi ve karar

A, mevcut genel amaçlı gösterge panelini tekrar ediyor ve görevleri aşağıya itiyor. B seçildi: işin aşamalarını gösteren sekmeli tepsi başlıkları bu ürünün ayırt edici öğesi. Diğer alanlar sakin kalacak. İlerleme sayıları tanıtım öğesi değil, gerçek görev durumunu özetleyen küçük bir satır olacak.

Rastgele sıra numaraları, büyük harf etiketleri, dekoratif yıldızlar ve eğimli renkli başlık parçaları kullanılmayacak. Yapısal kesikler yalnızca görev tepsilerinde; her bileşene aynı kart görünümü uygulanmayacak. Koyu görünüm aynı hiyerarşiyi koruyacak.

## Doğrulama

Görev işlemleri, görünüm/tema tercihi, filtreler ve sunucudan tek tıkla JSON indirme korunacak. Masaüstü, mobil, koyu görünüm ve form ekranları gerçek Chrome ile incelenecek. Mevcut kayıtlar tasarım için değiştirilmez; görsel kontroller geçici veriler kullanır.
