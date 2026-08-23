## <!-- SEED: re-run /impeccable document once there's code to capture the actual tokens and components. -->

name: Rakda
description: Virtual Data Room — secure, document-first, controlled-access workspace for deals.

---

# Design System: Rakda

## 1. Overview

**Creative North Star: "The Clean Room"**

Rakda terasa seperti ruang bersih berakses terkontrol: dingin, presisi, dan
tenang. Permukaan kosong dan netral; warna hanya muncul saat punya arti —
sebuah aksi, sebuah status, sebuah hak akses. Ketenangan visual ini bukan gaya,
tapi pesan: data di sini aman dan akurat. Sistem ini mengambil ketenangan
document-first ala Notion/Dropbox lalu menambah keseriusan sebuah VDR enterprise.

Density-nya sedang-ke-padat saat dibutuhkan (daftar dokumen, jejak audit,
tabel permission), tetapi tidak pernah berantakan. Chrome mundur; konten dan
status memimpin. Setiap state — siapa mengakses apa, kapan, dengan izin apa —
terbaca sekilas.

Sistem ini **menolak** look generic-SaaS (gradient ungu, background cream/sand,
blok hero-metric, grid kartu identik) dan juga menolak kekacauan legacy-VDR yang
berat dan kuno. Kredibilitas incumbent ditiru; clutter-nya tidak.

**Key Characteristics:**

- Cool, restrained, document-first
- Warna = makna (aksi/status/akses), bukan dekorasi
- Flat by default; kedalaman muncul sebagai respons state
- Fakta mesin (ID, hash, timestamp, audit) selalu mono

## 2. Colors

Palet cool dan restrained: permukaan near-white netral, satu suara brand
slate-teal, neutral di-tint tipis ke arah teal. Hex/ramp final ditetapkan saat
implementasi (`/impeccable document` ulang setelah ada kode).

### Primary

- **Slate Teal** (anchor `oklch(0.50 0.09 200)` — _[ramp to be resolved during
  implementation]_): aksi utama, seleksi aktif, indikator status aktif. Cool dan
  presisi tanpa jadi biru fintech klise.

### Neutral

- **Ink** (`[to be resolved]`, cool near-black): teks utama. Wajib ≥7:1 vs bg.
- **Muted** (`[to be resolved]`): teks sekunder, ≥4.5:1 vs bg.
- **Surface** (`[to be resolved]`, near-white cool): kanvas konten.
- **Panel** (`[to be resolved]`): lapis neutral kedua untuk sidebar/toolbar —
  sedikit lebih cool/gelap dari surface konten.
- **Border/Divider** (`[to be resolved]`): garis tipis, low-contrast.

### Accent / Semantic (to be resolved)

Vocabulary status — `success` / `warning` / `error` / `info` — ditetapkan saat
implementasi sebagai hue yang jelas berbeda dari Primary dalam hue _dan_
lightness, dan harus mampu menahan teks pada fill (white text di atas fill
saturated).

### Named Rules

**The One Voice Rule.** Primary dipakai ≤10% dari layar mana pun — hanya untuk
aksi utama, seleksi sekarang, dan indikator state aktif. Kelangkaannya adalah
intinya.

**The Quiet Surface Rule.** Background tetap near-white/cool-neutral. Warna
masuk hanya saat berarti sesuatu (aksi, status, akses). Permukaan tidak pernah
"dihias".

## 3. Typography

**Body/UI Font:** Single sans, tajam & netral _(family dipilih saat
implementasi — arah: technical sans seperti Inter/Geist)_
**Data/Mono Font:** Mono _(family to be chosen at implementation)_

**Character:** Satu sans menopang heading, label, dan body; mono mengambil semua
"fakta mesin". Pasangan dipilih pada sumbu kontras (sans vs mono), bukan dua sans
yang mirip.

### Hierarchy

Skala rem tetap (bukan fluid), rasio rapat ~1.125–1.2 — khas product UI.

- **Headline** (semibold): judul halaman/section. _[size to be resolved]_
- **Title** (medium): header panel/card, nama dokumen. _[size to be resolved]_
- **Body** (regular): prosa & isi, maks 65–75ch. _[size to be resolved]_
- **Label** (medium, sentence-case): label form, tab, nav. _[size to be resolved]_
- **Mono** (regular): ID dokumen, hash, ukuran file, timestamp, baris audit.

### Named Rules

**The Mono-for-Facts Rule.** Apa pun yang machine-true — ID dokumen, hash,
ukuran, timestamp, entri audit — diset mono. Prosa dan label UI tetap sans.
Mono adalah sinyal "ini fakta yang terverifikasi".

## 4. Elevation

Flat by default. Kedalaman disampaikan lewat tonal layering (surface vs panel vs
border), bukan bayangan dekoratif. Shadow hanya muncul sebagai respons state —
hover yang bisa diklik, overlay/menu/dialog yang mengambang di atas konten.

### Named Rules

**The Flat-By-Default Rule.** Permukaan datar saat diam. Elevation hanya hadir
sebagai respons terhadap state (hover, active, focus) atau untuk lapisan
mengambang (dropdown, modal, toast). Tidak ada kartu melayang tanpa alasan.

## 6. Do's and Don'ts

### Do:

- **Do** jaga Primary ≤10% layar — aksi utama, seleksi, status aktif saja.
- **Do** beri setiap komponen interaktif state lengkap: default, hover, focus,
  active, disabled, loading, error.
- **Do** pakai skeleton untuk loading konten, bukan spinner di tengah.
- **Do** tulis empty state yang mengajari antarmuka, bukan "nothing here".
- **Do** set semua fakta mesin (ID, hash, timestamp, audit) dengan mono.
- **Do** penuhi kontras WCAG AA: body ≥4.5:1, teks besar ≥3:1; dukung
  `prefers-reduced-motion`; navigasi keyboard-first.

### Don't:

- **Don't** pakai look generic-SaaS: gradient ungu, background cream/sand, blok
  hero-metric, atau grid kartu identik. (Anti-reference PRODUCT.md.)
- **Don't** tiru kekacauan legacy-VDR yang berat & kuno (gaya lama
  Intralinks/Datasite). (Anti-reference PRODUCT.md.)
- **Don't** pakai border-left/right >1px sebagai stripe warna pada card/list/alert.
- **Don't** pakai gradient text (`background-clip: text`).
- **Don't** pakai glassmorphism dekoratif sebagai default. **Satu pengecualian
  yang disahkan (Fase 10):** tirai "Mode privasi" di viewer
  (`backdrop-filter: blur(16px)` + tint `base-200` 60%, bentuk lewat
  `mask-image`) — di sana blur adalah *mekanismenya*, bukan hiasannya. Ia wajib
  menyertakan fallback opaque untuk `@supports not (backdrop-filter)` dan
  `prefers-reduced-transparency: reduce`. Jangan menurunkan pengecualian ini ke
  permukaan lain.
- **Don't** pakai display font pada label, tombol, atau data.
- **Don't** pakai motion dekoratif — gerak hanya menyampaikan state (150–250ms).
- **Don't** jadikan modal pilihan pertama — habiskan alternatif inline/progressive dulu.
