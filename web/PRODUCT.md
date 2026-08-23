# Product

## Register

product

## Users

Deal teams, founder/operator startup, serta profesional legal & finance yang
menjalankan due diligence, fundraising, atau M&A. Konteks pemakaian: pekerjaan
rahasia, bertaruh tinggi, dan terikat waktu — mereka perlu membagikan dokumen
sensitif ke pihak luar dengan akses terkontrol dan teraudit. Target awal SaaS
(tim deal kecil–menengah), berkembang ke enterprise.

## Product Purpose

Rakda adalah Virtual Data Room: ruang aman untuk menyimpan, membagikan, dan
mengaudit dokumen rahasia selama proses deal. Inti nilainya bukan fitur, tapi
kepercayaan — para pihak harus yakin datanya aman dan akurat, lalu bisa
menemukan serta membagikan dokumen dengan cepat. Sukses = pengguna mempercayakan
dokumen paling sensitif mereka ke Rakda dan tidak pernah ragu siapa mengakses apa.

## Brand Personality

Modern, sharp, efficient. Kepercayaan dibangun lewat presisi dan restraint, bukan
seremoni. Nadanya tenang dan faktual — terdengar seperti tooling profesional yang
dipakai orang setiap hari, bukan brosur penjualan.

## Anti-references

- Generic-SaaS template: gradient ungu, background cream/sand, blok hero-metric,
  grid kartu identik. Ini "tell" AI dan harus dihindari.
- Legacy-VDR yang berat dan penuh sesak (gaya lama Intralinks/Datasite):
  kredibilitasnya boleh ditiru, tapi clutter dan kesan kunonya tidak.

## Design Principles

1. **Trust is shown, not claimed.** State akurat, status akses jelas, tanpa
   ornamen dekoratif. Ketenangan visual = sinyal keamanan.
2. **Document-first.** Konten (file, folder, jejak audit, permission) memimpin;
   chrome mundur ke belakang — setenang Notion/Dropbox.
3. **Sharp over soft.** Tipografi tajam dan spacing rapat yang disengaja
   menyiratkan efisiensi; hindari kelembutan ala marketing.
4. **Enterprise credibility, tanpa beratnya.** Serius dan dapat diandalkan
   seperti incumbent, tetapi bersih dan cepat.
5. **Every state legible.** Permission, akses, dan status terbaca sekilas;
   kejelasan mengalahkan kepadatan.

## Accessibility & Inclusion

WCAG AA sebagai baseline: kontras cukup untuk membaca dokumen/data panjang,
dukungan `prefers-reduced-motion`, dan navigasi keyboard-first untuk power user.

## Keamanan unduhan

Unduhan selalu dilayani sebagai PDF — ber-watermark untuk `can_download`, bersih
untuk `can_download_original` — dan byte native tidak pernah keluar. Sejak
2026-08-21 (step 9-g) **varian ber-watermark dirakit ulang sebagai raster
ter-flatten**: tiap halaman dirender lalu tanda air dibakar ke piksel memakai
mekanisme yang sama persis dengan viewer, dan dirakit kembali jadi PDF. Tandanya
tidak bisa dihapus dengan alat PDF apa pun (stamp vektor pdfcpu yang bisa
dicabut satu perintah — `pdfcpu watermark remove` — telah dihapus). Harga yang
dibayar terang-terangan: berkas unduhan ber-watermark **kehilangan lapisan
teks** (tak bisa diseleksi, disalin, di-Ctrl+F, atau dibaca pembaca layar),
ukurannya membengkak, dan CPU dibayar tiap unduhan karena tandanya unik per
permintaan. Pemegang `can_download_original` menerima PDF tanpa tanda dan tanpa
perubahan — keputusan produk, bukan celah teknis.

**Unduhan ber-watermark dibatasi 150 halaman.** Di atas itu tombol unduhnya mati
dan menjelaskan alasannya dengan dua angka — batasnya, dan jumlah halaman
dokumen itu — lalu mengarahkan ke viewer. Batas ini ongkos, bukan izin:
merakit PDF raster menahan sekitar 10 MB piksel per halaman, dan proxy web
memutus permintaan di 300 detik. Pemegang `can_download_original` tidak kena
batas ini sama sekali — berkasnya cuma disalin, tidak dirakit ulang.

Angka 150 dikirim server ke klien, tidak ditulis di kode web. Kalau batasnya
berubah, UI ikut tanpa perlu disentuh.

## Perlindungan layar

Viewer meraster halaman dan membakar watermark per permintaan. Di atas itu,
Fase 10 menambah lapisan yang melawan jalur kebocoran yang **tidak** ditutup
watermark: mata orang lain, berbagi layar tak sengaja, dan kamera ponsel.

**Yang selalu aktif, untuk semua peran, tanpa sakelar:**

- Klik kanan di halaman dokumen tidak menawarkan "Simpan gambar sebagai…".
- Ctrl+P tidak mencetak isi. Yang keluar satu halaman pemberitahuan, dan ia
  menyebut tombol Unduh **hanya** bila pembaca benar-benar punya izinnya.
- Isi tertutup saat jendela kehilangan fokus lebih dari setengah detik, dan
  terbuka sendiri saat pembaca kembali. Selama tertutup, waktu baca tidak
  dihitung.

**Yang dinyalakan pembaca sendiri — "Mode privasi":** pita yang mengikuti
kursor; sisanya dikaburkan. Diingat lintas dokumen. Bukan izin: owner tidak
bisa memaksakannya dan tidak bisa melihat siapa yang memakainya.

**Yang TIDAK dijanjikan — dan tidak boleh dijanjikan di salinan UI mana pun:**

- Kami **tidak** memblokir tangkapan layar. Tidak ada API peramban yang bisa.
  Win+Shift+S membekukan isi layar sebelum kami sempat menutupinya.
- Terhadap kamera ponsel, Mode privasi hanya **mengurangi** hasilnya. Pemotret
  yang sabar tetap bisa mengambil banyak jepretan.
- Blur menghapus teks isi, bukan struktur. Letak paragraf dan tabel tetap
  terlihat, dan **judul besar masih bisa tertebak bentuk katanya**.
- Gambar halaman utuh tetap sampai ke peramban dan bisa diambil lewat DevTools.

Frasa **"proteksi screenshot" dilarang**. Yang melindungi sungguhan tetap
tiga hal: viewer raster tanpa lapisan teks, watermark yang dibakar dan tidak
bisa dilucuti, dan jejak audit yang menamai pembacanya.

