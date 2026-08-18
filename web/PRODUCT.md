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

## Keamanan unduhan (batas yang diketahui)

Unduhan selalu dilayani sebagai PDF — ber-watermark untuk `can_download`, bersih
untuk `can_download_original` — dan byte native tidak pernah keluar. Tanda air di
**PDF unduhan** bersifat deterensi, bukan proteksi: ia objek teks vektor di atas
konten, sehingga bisa dihapus dengan alat PDF gratis (mis. `pdfcpu watermark
remove`, Acrobat) dalam hitungan detik sampai menit. Tanda air di **viewer**
(dibakar ke piksel PNG) tidak bisa dihapus bersih, jadi bocor lewat foto/crop
layar tetap teratribusi. Pemegang `can_download_original` menerima PDF tanpa
tanda — keputusan produk, bukan celah teknis. Ditetapkan 2026-08-18: keterbatasan
ini diterima untuk tier saat ini; bila atribusi kebocoran menjadi kebutuhan nyata,
solusinya adalah PDF terenkripsi per-unduhan (pola iDeals), bukan menebalkan
tanda air.
