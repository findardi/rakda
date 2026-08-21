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

Wadi adalah Virtual Data Room: ruang aman untuk menyimpan, membagikan, dan
mengaudit dokumen rahasia selama proses deal. Inti nilainya bukan fitur, tapi
kepercayaan — para pihak harus yakin datanya aman dan akurat, lalu bisa
menemukan serta membagikan dokumen dengan cepat. Sukses = pengguna mempercayakan
dokumen paling sensitif mereka ke Wadi dan tidak pernah ragu siapa mengakses apa.

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
