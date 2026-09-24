# Sigap — Fase 1: Audit Repositori & Arsitektur UX

> Status: DRAFT untuk review. Belum ada layar Hi-Fi, belum ada perubahan kode produksi.
> Sumber kebenaran: `apps/web` (SvelteKit), `apps/api` (Go), `apps/queue-engine` (Rust),
> `packages/db/migrations`, `packages/db/seed/rbac.sql`. Jika dokumen di `docs/` bertentangan
> dengan kode, kode adalah acuannya (terdapat drift — lihat L.12).
> Tanggal: 2026-09-15
>
> **Revisi r.2 (2026-09-15):** Fase 1 disetujui dengan koreksi (owner decisions D1–D14).
 > Terapkan di seluruh dokumen: preservasi route produksi, limit notifikasi terverifikasi,
 > 500 duplikat booking tetap generik, menu akun netral (tanpa introspeksi peran),
 > indikator progress kunjungan (bukan riwayat), wallet dikecualikan dari Fase 2,
 > polling antrean 30 dtk dengan syarat, fasilitas = penyaring klien (bukan context switch),
 > home bebas dashboard demo, gitignore `design/` selektif. Lencana di tiap bagian: **[D#]**
 > merujuk keputusan owner.
 >
 > **Revisi r.3 (2026-09-15):** dua koreksi faktual akhir sebelum Hi-Fi:
 > **[R3-F1] Notifikasi TIDAK ber-paginasi server** — hanya filter + `limit` (default 100,
 > maks. 500); tanpa offset/page/cursor/next-token. UI = slice dataset klien; paginasi
 > server sejati = kemampuan backend masa depan.
 > **[R3-F2] Check-In janji temu vs walk-in queue adalah dua jalur berbeda**
 > (`/appointments/check-in` = `appointment_id`+`checkin_code`; `POST /queues/generate`
 > = nama+HP). Label: "Check-In Janji Temu" dan "Ambil Antrean Tanpa Janji"; kontrak
 > backend tidak boleh dilebur. Lencana **[R3-F1]** / **[R3-F2]** menandai bagian terkait.
 >
 > **Revisi r.4 (2026-09-20): CITIZEN UI — FROZEN.** Alur warga Fase 2B
 > (Buat Janji Temu 3 langkah, Check-In Janji Temu, Ambil Antrean Tanpa Janji,
 > Status Kunjungan, Login, Register + seluruh state) disetujui dan dibekukan
 > dalam proyek desain `design/generated/sigap-warga-mobile` dan
 > `design/generated/sigap-warga-desktop` (37 halaman per viewport, lolos
 > `validate-design-workspace` dan `validate-finish-readiness`). Contoh data
 > sintetis netral: "Budi Santoso". Fase 2C (Admin) BELUM dimulai.

---

## A. Inventaris Route yang Ada

### A.1 Route Wajah (SvelteKit, `apps/web/src/routes`)

| Route | Audiens | Auth saat ini | Sumber data | Aksi/formulir | State yang ditangani |
|---|---|---|---|---|---|
| `/` (Beranda) | Publik (anonim & login, termasuk admin — tidak ada pembagian peran) | Tidak ada | 6 fasilitas **hardcoded demo** (f1–f6) + SSE `GET /api/v1/events/beds` + `POST /api/v1/queues/generate` | Pencarian/filter/sort di klien; form antrean (Nama + No. HP); tombol chaos/geolokasi/modal peta rujukan | Tidak ada skeleton; data langsung karena statis; error antrean hanya via pesan 429 dari backend |
| `/appointments/new` (Buat Janji Temu) | Publik, tanpa auth | Tidak ada | `GET /api/v1/public/facilities` + `GET /api/v1/public/service-units` untuk `<select>`; degradasi ke input teks UUID manual bila gagal; `POST /api/v1/appointments` | Formulir: fasilitas*, unit layanan*, praktisi (opsional), tanggal+waktu* (dirakit jadi `${date}T${time}:00Z`), HP*, nama*, catatan | Error inline; sukses menampilkan `checkin_code` + panel "Referensi demo" (ID + status mentah) |
| `/appointments/check-in` (Check-In) | Publik | Tidak ada | `POST /api/v1/appointments/{id}/check-in` (proxy yang benar-benar interpolasi `event.params.id`) | Input `appointment_id` (UUID) + `checkin_code`; deep-link dari query `appointment_id`/`checkin_code` | Error generik; sukses: nomor antrean `formatted_number` + `estimated_wait_minutes` (hardcoded 25 di engine) |
| `/patient/status` (Status Kunjungan) | Publik | Tidak ada | `GET /api/v1/patient/status?code=` (kode check-in ATAU nomor antrean, pencarian dua tahap; 30 req/menit/IP) | Satu input kode | Kosong (form), error "Kode tidak ditemukan", hasil dengan badge status |
| `/auth/login` | Publik | Aja (login ke Supabase) | Form action server: `signInWithPassword` | Email + kata sandi | 503 unconfigured, 400 validasi, 401 kredensial salah, banner sukses `?registered` |
| `/auth/register` | Publik | Aja | Form action server: `signUp` | Email, password (min 8), konfirmasi | Validasi inline; sukses redirect ke `/auth/login?registered=1` |
| `/auth/logout` | Publik | Aja | Action server `signOut` | Interstitial "Sedang keluar..." | — |
| `/wallet` (Dompet Jejak Medis) | Publik | Tidak ada | **Jalan data rusak**: memanggil `GET /api/v1/medical-records?phone=` — proxy SvelteKit-nya TIDAK ADA → selalu error; HP demo `081234567890` prefilled | Input HP + "Muat Dompet" | Selalu error/empty. Halaman stub "Apple Wallet style" |
| `/admin/appointments` | Admin/staf | Hanya ditegakkan di Go API (Bearer / dev-header); halaman disajikan ke semua orang | `GET /api/v1/admin/appointments` (semua baris, tanpa filter server); `PATCH .../status` | Filter status di klien (bukan request); aksi baris sesuai state machine: scheduled → Selesai/Batal/Tidak Hadir; checked_in/queued → Selesai/Batal | Loading, error banner (401/403 → "Akses ditolak. Pastikan Anda memiliki izin yang sesuai."), kosong, manual reload; panel "lastUpdate" monospace mentah |
| `/admin/queues` | Admin/staf | Sama | `GET /api/v1/admin/queues` (filter `?facility_id=` TIDAK dihormati backend); `PATCH /admin/queues/{id}/status` | Filter berupa **input UUID mentah** "Filter ID Fasilitas"; aksi transisi: waiting→called/cancelled; called→in_service/cancelled/skipped; in_service→completed | Loading, error, kosong + saran "Masukkan ID fasilitas" (saran yang menyesatkan, karena filter tidak didukung server) |
| `/admin/facilities` | Admin/staf | Sama | `GET/POST /api/v1/admin/facilities`, `PATCH .../{id}`, `PATCH .../{id}/deactivate` | Tabel + modal create/edit (`confirm()` native untuk nonaktifkan); modal dengan `svelte-ignore a11y_*` | Loading, error, kosong; modal tanpa focus trap/ESC |
| `/admin/schedules` | Admin/staf | Sama | `GET/POST /api/v1/admin/schedules`, `PATCH .../{id}`; proxy `admin/service-units` ada tapi TIDAK dipakai halaman | Tabel + modal; input **ID UUID mentah** untuk fasilitas/layanan/praktisi | Loading, error, kosong. Di bawah identitas dev, create/edit → **403** (dev permission set tidak memuat `schedule.manage`) — tidak ditangani khusus |
| `/admin/notifications` | Admin/staf | Sama | `GET /api/v1/admin/notifications` (filter status/channel/template/dari/sampai di server; frontend saat ini mengirim `limit=200`; **terverifikasi di Go handler**: default 100, nilai hanya dihormati jika `0 < limit ≤ 500` [D3]), `GET .../summary`, `POST .../{id}/retry` & `cancel` | Filter bar URL-synced (shareable), summary cards per status, skeleton, aksi retry/cancel | Halaman terbaik di app: loading (skeleton), error list & summary terpisah, kosong + filter, aksi per status |

### A.2 Proxy API SvelteKit (`src/routes/api/v1/...`)

| Proxy | Endpoint Go yang dipanggil |
|---|---|
| `public/facilities`, `public/service-units` | `GET /api/v1/public/...` (catalog minim: id, nama, short_code, is_active — tanpa alamat/kasur) |
| `appointments` (POST), `appointments/[id]/check-in` | booking publik + check-in (proxy check-in interpolasi ID benar; proxy admin tidak) |
| `queues/generate` | antrean walk-in (Rust engine) |
| `patient/status` | pencarian status publik per kode |
| `events/beds` | SSE — praktis hanya membawa event `queue_created`, bukan data kasur |
| `admin/*` (facilities, service-units, schedules, appointments, queues, notifications, notifications/summary) | admin endpoints, Bearer dari sesi Supabase atau `X-Sigap-Dev-User-ID` bila `SIGAP_DEV_IDENTITY=true` |

Tidak ada proxy untuk `medical-records` (menghancurkan /wallet) maupun `facilities/nearby` (modal peta rujukan fallback ke data klien).

### A.3 Navigasi global saat ini

- Header: wordmark "Sigap" + lencana **MVP** + nav desktop `hidden md:flex` (Beranda, Buat Janji Temu, Check-In, Status Kunjungan, Admin→/admin/queues). **Tidak ada menu mobile sama sekali** — di layar <md tujuan utama tak terjangkau.
- 4 dari 5 halaman admin (appointments, facilities, schedules, notifications) dan `/wallet` **orphan** — hanya bisa dijangkau lewat URL ketik manual.
- Footer: "Sigap — Open Source Civic-Tech • Data contoh untuk scaffolding".
- Tidak ada guard route di SvelteKit; penguatan terjadi di Go API + throw klien pada 401/403.

---

## B. Peta Audiens / Peran

| Audiens | Identitas | Realitas saat ini |
|---|---|---|
| Warga / pasien anonim | Tidak ada identitas di sisi pasien; booking/check-in/status berbasis **kode + nomor HP**, bukan akun | Seluruh flow pasien publik dan tanpa auth |
| Warga berakun | User Supabase (login/register) | Login ada, tapi **tidak ada "akun warga"** di app: tidak ada data "janji temu saya", riwayat, atau inbox. Satu-satunya manfaat login hari ini: sesi Bearer untuk endpoint admin |
| `super_admin` | Peran sistem; scope global | Role global (`facility_id = NULL`) justru menghasilkan **himpunan fasilitas kosong** → list kosong di semua endpoint ber-scope (kecuali actor dev). Jebakan nyata |
| `facility_admin` | Peran per-fasilitas | CRUD fasilitas + jadwal + antrean + notifikasi + janji temu dalam scope |
| `operator` | Peran per-fasilitas, terbatas | `appointment.read/manage`, `queue.*`, `schedule.read` (TANPA `schedule.manage`), `notification.read` → tombol retry/cancel notifikasi harus disembunyikan |
| `viewer` | Read-only | Semua `.read` |
| Aktor dev (`admin-ui`) | `X-Sigap-Dev-User-ID` saat `SIGAP_DEV_IDENTITY=true` | 11 izin hardcoded, **tidak termasuk `schedule.manage`** → form jadwal create/edit pasti 403 pada demo lokal |

Fakta penting: tidak ada API/UI untuk membuat user, memberi peran, atau mengelola izin (hanya bootstrap CLI + SQL manual). Penetapan peran = pekerjaan operasional di luar aplikasi.

---

## C. Audit UX Saat Ini (kritik agresif)

**Struktur & navigasi**
1. Satu shell untuk semua peran. Admin dan warga melihat header yang sama; "Admin" adalah satu link yang ternyata ke `/admin/queues` saja. 4 halaman admin lain tak terjangkau dari UI mana pun.
2. Mobile: nav utama `hidden md:flex` → aplikasi yang klaimnya mobile-first warga justru **buta mobile**. Tidak ada bottom nav, tidak ada hamburger.
3. Tidak ada guard: warga anonim melihat semua tombol admin; kegagalan muncul sebagai banner merah di tengah halaman, bukan penghalang yang jelas.
4. Home bukan tujuan, melainkan "panggung demo": dashboard kasur dari 6 baris hardcoded + form antrean + peta rujukan, tanpa satu pun penjelasan tugas.

**Bahasa & copy**
5. Lencana "MVP", footer "Data contoh untuk scaffolding", panel "Referensi demo" (ID mentah, status mentah), tombol "Chaos Mode (Load Test 50x)", "Test Modal Peta Rujukan", "Log Radar Anti-Calo (Gamifikasi)" — scaffolding & demo langsung di produk, semua memakai emoji sebagai icon UI.
6. Campuran Inggris di UI admin: header tabel "Created/Channel/Template/Attempts", tombol "Edit/Retry/Cancel" — hanya copy warga yang terjemah.
7. Pesan error menyamaratakan kasus: "Gagal menghubungi layanan. Silakan coba lagi." dipakai untuk 503, jaringan, dan 400 validasi.

**Keputusan desain**
8. Aksen emerald Tailwind `#059669` + palet slate, bukan sistem warna kano (`#0F766E`, canvas `#F7F6F3`). Varian `dark:` via media OS tanpa toggle — setengah matang.
9. "Apple Wallet style" cards + placeholder QR + tipografi monospace di mana-mana: estetika generik, bukan layanan publik.
10. Striper merah diagonal + log berjalan merah di home = visual alarm permanen yang menguras perhatian.
11. Filter antrean = input UUID mentah; form jadwal = 3 input UUID mentah. UI memantulkan struktur database, bukan tujuan pengguna.
12. `confirm()` native untuk menonaktifkan fasilitas; modal kustom tanpa focus trap / ESC / dialog semantics yang utuh (`svelte-ignore a11y_*`).
13. `relativeTime()` pada notifikasi salah urut: detik ditampilkan sebagai "m lalu", jam sebagai "h lalu" — data operasional bisa menyesatkan.
14. Tidak ada auto-refresh di mana pun untuk antrean hidup; operator harus memencet "Muat" berulang.
15. Baris aksi admin menumpuk tombol teks 10px satu di atas yang lain — target ketik terlalu kecil untuk meja operasional.
16. Home mengklaim ketersediaan kasur hidup (SSE), padahal feed SSE hanya membawa event antrean; angka kasurnya demo statis → **mengklaim data yang tidak ada** (fake metrics).

**Aksesibilitas**
17. Status dikomunikasikan dengan warna chip 10px (kontras di bawah AA untuk teks 10px), emoji sebagai aksen, `svelte-ignore a11y_*` di modal, tidak ada `aria-live` untuk pembaruan status antrean, heading hanya H1 per halaman.

---

## D. Arsitektur Informasi yang diusulkan

Dua shell, satu merek, dua densitas.

**[D2] Preservasi route produksi.** Nama Indonesia pada bagian ini adalah **label UI saja**,
bukan rename URL. Semua route yang sudah ada dipertahankan apa adanya; route baru hanya
ditawarkan untuk kemampuan yang memang belum punya layar (penemuan fasilitas & ringkasan admin).

```
/  → SHELL WARGA (mobile-first, tujuan, kanvas #F7F6F3, kontrol 44px)
│  ├── /                     Beranda  [D12: bebas dashboard demo; hanya data nyata]
│  ├── /faskes               Cari Faskes — ROUTE BARU (penemuan fasilitas; tak ada layarnya sekarang)
│  ├── /appointments/new     label "Janji Temu" (route lama dipertahankan)
│  ├── /appointments/check-in  label "Check-In"
│  ├── /patient/status       label "Status Kunjungan"
│  └── /auth/login, /auth/register, /auth/logout — label Masuk/Daftar/Keluar + menu akun netral [D5]
│  └── /wallet               DIKELUARKAN dari navigasi & DIKECUALIKAN dari Hi-Fi Fase 2 [D7];
│                           (kode tidak dihapus pada fase desain)
│
/admin → SHELL ADMIN (desktop-first, sidebar, kanvas hangat, kontrol 36–40px)
   ├── /admin                Ringkasan hari ini — ROUTE BARU (hanya nilai turunan nyata) [D13]
   ├── /admin/queues         label "Antrean"
   ├── /admin/appointments   label "Janji Temu"
   ├── /admin/schedules      label "Jadwal"
   ├── /admin/facilities     label "Fasilitas"
   └── /admin/notifications  label "Notifikasi"
```

Prinsip:
- **Warga tidak melihat console, operator tidak melihat panggung demo.** Sepenuhnya terpisah, berbagi token (warna/tipe/ikon/semantik).
- **Route baru hanya 2** dan hanya bila fungsinya memang belum punya layar: `GET /faskes` (penemuan fasilitas dari catalog publik) dan `GET /admin` (ringkasan dari endpoint yang ada). Semua yang lain: reka-rupa layar pada route yang sama. [D2]
- `/wallet` dihapus dari navigasi; tidak masuk Hi-Fi Fase 2; kode halaman dibiarkan utuh selama fase desain. [D7]
- Kapan pun data tidak ada, UI menyatakan ketiadaan itu secara jujur (state kosong benar), tidak mengarang. [D12]

---

## E. Model Navigasi Warga

- **Mobile (390px)**: header ramping (logo Sigap + menu akun) + **bottom tab 4 tujuan** [D11]:
  1. Beranda — konteks, aksi cepat
  2. Faskes — penemuan fasilitas
  3. Check-In — tugas paling time-critical; layarnya menyajikan **dua jalur eksplisit dan terpisah** [R3-F2]: **"Check-In Janji Temu"** (ID janji + kode) dan **"Ambil Antrean Tanpa Janji"** (nama + HP)
  4. Status — posisi antrean saya
  - "Janji Temu Baru" = CTA menonjol dari Beranda **dan** dari halaman Faskes, bukan tab permanen [D11].
- **Tablet/Desktop (1440px)**: bottom nav hilang; tujuan jadi nav horizontal di header + CTA "Janji Temu" tetap terlihat. Form booking jadi 2 kolom (form kiri, ringkasan kanan).
- **Menu akun netral** [D5]: frontend tidak memiliki sumber peran/capability yang di-derive dari server
  (layout loader hanya exposing `userEmail`; introspeksi peran via endpoint admin tidak ada dan tidak
  boleh dipakai sebagai pemicu UI). Maka menu akun hanya: saat anonim → "Masuk" / "Daftar";
  saat login → email + "Keluar". **Tidak** ada tautan kondisional "Lihat sisi operator" —
  endpoint admin protektif tetap menjadi satu-satunya sumber otorisasi untuk sisi admin.
- **Beranda** hanya menampilkan data yang benar-benar didukung kontrak backend produksi [D12]:
  CTA tugas, ringkasan catalog (jika ada), dan cara kerja Sigap. Tidak ada dashboard bed,
  metrik "live" palsu, chaos mode, gamifikasi, kontrol test, atau peta demo.
- Setiap tugas warga = satu halaman satu tugas, dengan langkah berikutnya selalu terlihat sebagai aksi
  utama (booking sukses → kartu kode + tombol Check-In; check-in sukses → kartu antrean +
  tombol "Pantau status").

## F. Model Navigasi Admin

- **Sidebar kiri tetap** (sticky, putih, border-kanan, lebar 240px @1440):
  - Konteks: nama + lencana peran actor aktif.
  - **Penyaring fasilitas klien-side** [D9]: daftar fasilitas hasil `GET /admin/facilities`
    disajikan sebagai filter operasional ("Fasilitas: Semua / [nama]") yang menyaring
    dataset yang sudah dimuat di klien. **Bukan "context switch" dan tidak menyiratkan
    penyaringan server-side** — backend memang tidak mendukung query `?facility_id`
    di daftar antrean/janji temu/jadwal (hanya notifikasi). UI harus jujur tentang ini:
    label "Menampilkan: X dari Y", bukan klaim filter terpusat.
  - Nav [D10]: **Ringkasan** (`/admin`), **Antrean** (`/admin/queues`), **Janji Temu**
    (`/admin/appointments`), **Jadwal** (`/admin/schedules`), **Fasilitas**
    (`/admin/facilities`), **Notifikasi** (`/admin/notifications`).
  - Bawah: menu akun (email, "Keluar", tautan netral "Lihat Beranda Warga" → `/` —
    tautan statis, bukan deteksi peran [D5]).
- **Header halaman**: judul tugas + subjudul satu baris + aksi utama halaman di kanan. Tidak ada breadcrumb — satu level, tak perlu.
- **Ringkasan hari ini** [D13]: route `/admin` baru; hanya nilai turunan yang benar-benar
  dihitung dari daftar yang sudah ada (antrean menunggu, janji temu hari ini per status,
  outbox gagal/pending). **Tidak ada KPI fiktif, tidak ada chart fiktif.** Setiap angka
  diberi label sumber + waktu perbarui ("perbarui manual • pukul 09:41").
- **Refresh** [D8]: tombol "Muat ulang" manual tersedia di semua halaman. Di **Antrean**
  (pandangan operasional) disetujui **polling 30 detik**, dengan syarat:
  1. refresh manual tetap tersedia di samping polling,
  2. polling **dijeda/dikurangi saat tab tersembunyi** (`visibilitychange`),
  3. **tidak boleh disebut "realtime"** — ini polling periodik,
  4. selalu menampilkan **waktu perbarui terakhir** ("Diperbarui pukul 09:41"),
  5. tidak ada perilaku SSE palsu.

---

## G. Inventaris Layar Lengkap + State

Legenda: N=normal, L=loading/skeleton, E=kosong, NR=tanpa hasil, S=sukses, V=error validasi, SR=error server, U=tanpa auth, F=forbidden.

### Warga

| Layar | State wajib |
|---|---|
| Beranda | N (aksi cepat + panduan 3 langkah; tanpa konten demo [D12]), E (catalog tanpa faskes aktif → pesan jujur), SR (catalog gagal + ulangi). Publik, tak butuh U. |
| Cari Faskes (`/faskes`, route baru) | N (daftar + filter tipe), L (skeleton 4 baris), E ("Belum ada data faskes" — masalah ops, jangan telan), NR (filter tak cocok + reset), SR |
| Janji Temu Baru | N (stepper 3 langkah), V (HP 10–15 digit, waktu harus masa depan), S (kartu Kode Check-In + langkah berikutnya), SR (429 "2/jenis per hari", 409 slot penuh bila pilih jadwal, 500 generik → UX error server generik + coba lagi; **copy spesifik "booking duplikat" TIDAK ditampilkan** [D4]). |
| Check-In (jalur **Check-In Janji Temu**; `/appointments/check-in`, butuh `appointment_id` + `checkin_code` — R3-F2) | N (form kode + ID prefilled dari link booking), V, S (nomor antrean besar + "Perkiraan menunggu: ±25 menit"), SR (401 kode salah, 404 tak ketemu, 409 state salah, 429 5×/5menit) |
| Walk-In (jalur **Ambil Antrean Tanpa Janji**; `POST /api/v1/queues/generate`, butuh nama + HP — R3-F2; jalur berbeda, jangan digabung dengan check-in janji temu) | N (form nama + HP → tiket antrean + perkiraan), V (HP 10–15 digit), S (nomor antrean `formatted_number` + "Perkiraan menunggu: ±25 menit" + CTA pantau status), SR (429 "2 antrean/hari" per HP+faskes, mesin antrean penuh 300 tiket/hari → pesan apa adanya), E (faskes penuh → ajukan pindah faskes lain, bukan antrean diam-diam) |
| Cek Status | N (kartu posisi + **indikator "Progress kunjungan"** berupa penanda state sekarang Check-In → Antre → Dilayani → Selesai, **bukan riwayat** dan tanpa timestamp yang diarangkan [D6]), NR ("Kode tidak ditemukan. Periksa kembali kode Anda." + panduan), L, SR (429 30/menit) |
| Masuk / Daftar | N, V (per-field), SR (503 "Autentikasi belum dikonfigurasi"), S (redirect + banner) |
| Menu akun | U (anonim → CTA masuk), N (login → email + Keluar; netral, tanpa tautan peran [D5]) |

### Admin

| Layar | State wajib |
|---|---|
| /admin Ringkasan | N, L, E (scope kosong: "Anda belum memiliki fasilitas dalam cakupan" — state kelas-1, lihat M.9), U (anonim → panel "Masuk untuk melanjutkan"), F (403 → panel izin yang kurang) |
| Antrean | N (board: Sedang dipanggil / Menunggu / Selesai hari ini; polling 30 dtk + refresh manual + label "Diperbarui pukul …" [D8]), L, E (hari ini sepi), NR (filter faskes kosong), S (update row inline), SR (401/403 panel, 409 transisi tidak valid), F (viewer: aksi di-drop, bukan disabled) |
| Janji Temu | N, L, E, NR (filter status+tanggal), S, SR, F (viewer hanya lihat) |
| Jadwal | N, L, E, S, V (slot 5–180 mnt & membagi rentang, kapasitas 1–100, end>start), SR, F (dev/operator tanpa `schedule.manage`: form disembunyikan + panel izin) |
| Fasilitas | N, L, E, S, V (field wajib, telepon blacklist karakter), SR, F, dialog konfirmasi nonaktifkan (bukan `confirm()`) |
| Notifikasi | N, L (skeleton — pola terbaik yang ada, pertahankan), E, NR, S (retry/cancel), SR (list & summary terpisah), F (retry/cancel hilang untuk role tanpa `notification.manage`). **Limit server terverifikasi [D3]**: default 100, nilai `limit` hanya sah jika `0 < n ≤ 500` (di luar itu mundur ke default); maksimum 500 baris per permintaan. **TIDAK ada paginasi server** (tanpa offset/page/cursor/next-token) — UI Fase 2 = filter + jumlah hasil + manajemen baris termuat (slice di klien dari ≤500 baris); kontrol Next/Previous yang menyiratkan halaman server **tidak** boleh ada. Paginasi server sejati dicatat sebagai kemampuan backend masa depan [M.2]. |

State global admin: banner "Identitas dev aktif" bila `SIGAP_DEV_IDENTITY=true`; skeleton per tabel; panel U/F seragam ("Masuk sebagai staf faskes" + tombol Masuk).

---

## H. Alur Pengguna Kritis

1. **Warga → booking**: Beranda/CTA → Faskes (pilih) → Janji Temu Baru (faskes terisi otomatis) → langkah [Fasilitas → Layanan → Waktu + Data Diri] → Ringkasan → Kirim → **S: kartu Kode Check-In** (salin / simpan lokal opsional) → CTA "Siap? Check-In" → **S: Nomor Antrean + perkiraan menunggu** → CTA "Pantau Status".
   Titik gagal nyata: catalog down (state jelas, bukan error merah), 429 harian, 409 slot penuh. Duplikat faskes+HP+hari saat ini memunculkan **500 generik** — tampil sebagai error server generik + "coba lagi"; copy spesifik duplikat **tidak** ditampilkan sampai backend memberi kontrak error stabil [D4, M.13].
2. **Warga walk-in → antrean (jalur terpisah [R3-F2], BUKAN check-in janji temu)**: di layar Check-In, pilih jalur **"Ambil Antrean Tanpa Janji"** → form nama + HP → `POST /api/v1/queues/generate` → tiket antrean (`formatted_number` + perkiraan menunggu) → Status (nomor antrean). Jalur ini sama sekali tidak menyentuh `appointment_id`/`checkin_code`; kontrak backend-nya berbeda dan tidak boleh digabungkan dengan alur Check-In Janji Temu (#1).
3. **Operator pagi → kondisi hari ini**: /admin Ringkasan (antrean menunggu & janji hari ini) → board Antrean (polling 30 dtk + label perbarui terakhir, bukan realtime [D8]) → Dipanggil → Dilayani → Selesai (transisi persis state machine, 1 klik).
4. **Operator memproses janji temu**: Janji Temu (filter hari ini + status) → scheduled→checked_in (pasien tiba dengan kode) → … → completed / cancelled / no_show. Konfirmasi hanya untuk `cancelled`.
5. **Operator memelihara jadwal**: Jadwal → buat/edit (faskes & layanan dari list data, tanggal/waktu/slot/kapasitas) → validasi server → daftar update. 403 → panel izin.
6. **Admin memelihara faskes**: Fasilitas → tambah/edit/nonaktifkan (dialog) → efek ke catalog publik dinyatakan di dialog ("faskes ini tak lagi muncul untuk warga").
7. **Operator menangani outbox**: Notifikasi → summary chips (gagal >0 yang menarik) → filter `failed` → retry / cancel (hanya role `notification.manage`).

---

## I. Inventaris Komponen

### Warga (densitas 44px)
- `CitizenHeader` (logo + menu akun), `CitizenBottomNav` (4 tab), `QuickActions` (Beranda — tanpa konten demo [D12])
- `FacilitySearch` + `FacilityResultRow` (nama, tipe, short code, layanan — TANPA klaim ketersediaan, data tak ada)
- `BookingStepper` (3 langkah), `BookingSummaryCard`, `CheckinForm` (jalur **Check-In Janji Temu** — `appointment_id` + `checkin_code`), `WalkInForm` (jalur **Ambil Antrean Tanpa Janji** — nama + HP → `queues/generate`; dua jalur terpisah secara eksplisit, kontrak backend tidak dilebur [R3-F2]), `QueueTicket` (nomor besar + perkiraan + langkah berikutnya), `VisitStatusCard` + `VisitProgress` — **indikator state sekarang** "Progress kunjungan" (Check-In → Antre → Dilayani → Selesai), bukan riwayat, tanpa timestamp/kejadian yang difabrikasi [D6]
- `AuthForms` (login/daftar), `AccountMenu` (netral: Masuk/Daftar bila anonim; email + Keluar bila login — tanpa tautan peran [D5])

### Admin (densitas 36–40px)
- `AdminSidebar` (nav + penyaring fasilitas klien-side + menu akun), `AdminPageHeader`, `FacilityFilter` (penyaring operasional klien-side atas dataset termuat; **bukan context switch**, tidak menyiratkan filter server [D9]), `FilterBar`, `DataTable` (header 11px uppercase, baris 36–40px), `QueueBoardRow` + `QueueBoard` (Sedang dipanggil / Menunggu / Selesai hari ini; polling 30 dtk + "Diperbarui pukul …" [D8]), `AppointmentRow`, `ScheduleEditor` (dialog), `FacilityEditor` (dialog), `NotificationRow` (recipient masked, attempts, last error) + `OutboxSummaryChips`, `ConfirmDialog` (gantikan `confirm()`)

### Berbagi (token kano)
- Button (primary `#0F766E` / hover `#0B6B63` / active `#084F49`; secondary; danger; text; radius 6px; tinggi 44px warga / 36–40px admin)
- Input / Select / Date / Time (label di atas, helper, error field-level)
- `StatusBadge` (semantik kano: info `#1D6BB5`, warning `#B45309`, success `#2F7D32`, danger `#C4322A`, netral; teks ≥11px, kontras AA; warna + label, bukan warna saja)
- `Alert` (error server, izin), `Toast` (sukses aksi kecil), `Dialog` (radius maks 12px, focus trap, ESC, aria-modal)
- `ResultPager` (hanya Notifikasi — **bukan paginasi server** [R3-F1]: membatasi tampilan dataset yang sudah termuat (maks. 500 baris, default 100); kontrol "Tampilkan N baris berikutnya" + jumlah hasil di klien — **tanpa** offset/page/cursor/next-token di backend), `Skeleton`, `EmptyState` (judul + penyebab + CTA), `ErrorState`, `ForbiddenPanel`, `UnauthPanel`, `CodeDisplay` (untuk kode check-in — satu-satunya tempat monospace layak)
- **Dua jalur antrean warga yang tetap terpisah secara eksplisit [R3-F2]**:
  - `CheckinForm` = **Check-In Janji Temu** — butuh `appointment_id` + `checkin_code`, memanggil `POST /api/v1/appointments/{id}/check-in`
  - `WalkInForm` = **Ambil Antrean Tanpa Janji** — butuh nama warga + nomor HP, memanggil `POST /api/v1/queues/generate`
  - Label, form, dan kontrak backend **tidak boleh dilebur** satu sama lain; secara visual boleh tetangga, secara kontrak tetap dua jalur berbeda.

Komponen **tidak** masuk inventaris (tak ada justifikasi produk): kalender mingguan (schedule single-date), timeline riwayat pasien (medical_records tanpa read API), chart tren (tidak ada endpoint agregat selain summary notifikasi), **"Dompet"/wallet** (jalan data mati; dikecualikan dari Hi-Fi Fase 2 [D7]), peta rujukan (nearby hardcoded — jangan dipertahankan [D12]).

---

## J. Strategi Responsif

**Warga** — 390px (primer): header 56px + bottom nav 4 tab; form satu kolom; tiket antrean full-width. 768px (tablet): bottom nav → nav header horizontal; booking 2 kolom. 1440px: konten terpusat maks ~1024px, layout tidak berubah lagi. Target ketik 44px di semua breakpoint.

**Admin** — 1440px optimal: sidebar 240px + konten fluid. 1280px: sidebar 208px. <1024px (tablet): sidebar → drawer hamburger. **Tabel padat**: (a) kolom opsional disembunyikan di bawah 1280 (Diperbarui, Attempts) dengan akses via drawer baris; (b) horizontal scroll + sticky kolom pertama di bawah 900; (c) aksi >2 → menu "Lainnya". QueueBoard: 3 kolom → 1 kolom ber-urutan di tablet.

---

## K. Matriks State Global

| State | Pemicu nyata | Pola UI |
|---|---|---|
| Loading / skeleton | fetch list apa pun | Skeleton bentuk tabel/kartu (pola Notifikasi = rujukan); <500ms boleh spinner |
| Kosong | list kosong scope | Judul + penyebab (data ops vs benar-benar kosong) + CTA sesuai peran |
| Tanpa hasil | filter tak cocok | "Tidak ada yang cocok untuk [filter]" + Reset |
| Sukses aksi | PATCH/POST | Update inline + toast ~4 dtk; panel "ID mentah" dihapus; ID jadi expand "Detail" |
| Validasi | 400 | Field-level bila terstruktur; selain itu pesan aksi + contoh (HP 10–15 digit) |
| Error server | 5xx / jaringan | Panel + "Coba lagi" (idempotent); pesan spesifik per kasus, bukan satu frasa untuk semua. **500 tetap diperlakukan generik** — jangan menebak penyebab (mis. duplikat booking) [D4] |
| 429 | rate limit (2/hari booking & antrean, 5/5mnt check-in, 30/mnt status) | Pesan backend apa adanya (sudah bahasa Indonesia) + saran tindakan |
| Tanpa auth | anonim membuka /admin/* | `UnauthPanel`: "Masuk sebagai staf faskes" + Masuk/Daftar |
| Forbidden | 403 (role, dev tanpa schedule.manage, scope kosong) | `ForbiddenPanel`: izin yang kurang (dari pesan backend "Akses ditolak: <permission>") + tahu ke siapa |
| 409 | transisi state tidak valid | Tampilkan pasangan transisi persis ("Menunggu → Selesai tidak diizinkan; harus Dipanggil dulu") |

---

## L. Keputusan UX Kunci + Alasan

**Ditetapkan oleh owner (r.2)**

 1. **[D1] Spesifikasi desain masuk version control; artefak generated/tmp diabaikan secara selektif.** `design/*.md` (spesifikasi) trackable; hanya `design/generated/` dan `design/tmp/` yang masuk `.gitignore`.
 2. **[D2] Preservasi route produksi.** Nama Indonesia = label UI saja; URL lama (`/appointments/new`, `/admin/queues`, dst.) tidak diubah. Route baru hanya 2: `GET /faskes` & `GET /admin` (fungsi tanpa layar).
 3. **[D3] Limit notifikasi terverifikasi: default 100, maksimum 500.** Paginasi desain berdasar angka ini saja.
 4. **[D4] 500 duplikat booking tetap generik.** Copy spesifik duplikat tidak ditampilkan; rekomendasi backend terpisah: konflik deterministik → HTTP 409 + kode error machine-readable (lihat M.13).
 5. **[D5] Menu akun netral.** Tidak ada UI kondisional berbasis peran karena frontend tidak punya sumber peran dari server; otorisasi admin = endpoint protektif saja.
 6. **[D6] "Progress kunjungan" = indikator state sekarang, bukan riwayat.** Tanpa timestamp/kejadian yang difabrikasi.
 7. **[D7] Wallet dikecualikan** dari navigasi & Hi-Fi Fase 2; kode tidak dihapus.
 8. **[D8] Polling antrean 30 dtk disetujui** + refresh manual, jeda saat tab hidden, label waktu perbarui, bukan "realtime", tanpa SSE palsu.
 9. **[D9] Fasilitas = penyaring klien-side**, bukan context switch, tanpa klaim filter server.
 10. **[D10] Label nav admin disetujui:** Ringkasan, Antrean, Janji Temu, Jadwal, Fasilitas, Notifikasi.
 11. **[D11] Nav warga mobile disetujui:** Beranda, Faskes, Check-In, Status; "Janji Temu Baru" = CTA dari Beranda & Faskes.
 12. **[D12] Home bebas konten demo** (dashboard bed, chaos, gamifikasi, kontrol test, peta palsu dihapus dari desain produksi).
 13. **[D13] Ringkasan admin = nilai turunan nyata** dari endpoint yang ada; tanpa KPI/chart fiktif.
 14. **[D14] Design system kanonik dipertahankan:** primary `#0F766E` (hover `#0B6B63`, active `#084F49`), canvas `#F7F6F3`, surface `#FFFFFF`, foreground `#1C1B1A`, muted `#57534E`, border `#E0DDD8`; Inter; spacing 8px; radius terkendali (kontrol 6px, panel ±8px, dialog maks ±12px); border & spacing didahulukan atas bayangan; kontrol warga ±44px, admin ±36–40px.

 **Koreksi faktual akhir r.3**

 15. **[R3-F1] Notifikasi bukan paginasi server.** Backend hanya menyediakan filter server + `limit` (default 100, maks. 500); tidak ada offset/page/cursor/next-token. UI Fase 2 memakai filter + jumlah hasil + manajemen baris termuat; kontrol paging bila ada hanya memotong dataset klien. Paginasi server sejati dicatat sebagai kemampuan backend masa depan (M.2).
 16. **[R3-F2] Check-In janji temu dan walk-in queue adalah dua jalur berbeda.** "Check-In Janji Temu" (`/appointments/check-in`: `appointment_id` + `checkin_code`) ≠ "Ambil Antrean Tanpa Janji" (`POST /api/v1/queues/generate`: nama + HP). Keduanya boleh bersebelahan secara visual di layar Check-In, tetapi label, form, dan kontrak backend dipertahankan terpisah — tidak ada penggabungan kontrak.

**Keputusan desain (diwarisi dari r.1, masih berlaku)**

1. **Dua shell penuh, bukan satu shell bercabang.** Densitas berlawanan (44px vs 36–40px) dan tujuan audiens berlawanan; satu shell hybrid menghasilkan "card soup".
2. **Kode check-in = identitas warga.** Tak ada "akun warga"; UX dibangun sekitar kepemilikan kode (kartu sukses + salin + langkah berikutnya). Penyimpanan lokal kode (localStorage) = kenyamanan murni klien, tanpa klaim backend.
3. **Perkiraan menunggu ditampilkan apa adanya** ("Perkiraan menunggu: ±25 menit") karena engine hardcode 25 — jujur > dramatis.
4. **Aksi di-drop bila role tak berwenang, bukan di-disable diam.** Lebih jelas daripada tombol abu-abu tanpa penjelasan; info sekali di halaman.
5. **ForbiddenPanel memetakan pesan 403 ke izin yang kurang.** Pesan generik lama hanya dipakai saat pesan backend tak terbaca.
6. **State "scope kosong" kelas-1** di Ringkasan & semua list admin, karena role global NULL memang menghasilkan scope kosong di backend.
7. **Satu aksen `#0F766E`, canvas `#F7F6F3`, border > bayangan** [D14]. Sejalan token kano; membuang emerald-600/slate/striper-merah/emoji. Merah hanya untuk truly negative.
8. **Copy 100% Indonesia di semua lapisan admin** (tabel, tombol, filter).
9. **Drift docs vs kode tidak ditiru ke spesifikasi.** Contoh: `FACILITY_ADMIN_REPORT` mengklaim peran `patient` + endpoint medical-records (tidak ada); `APPOINTMENTS_REPORT` klaim filter status appointment (tidak ada di router). Desain dibangun di atas endpoint nyata saja.

---

## M. Batas Backend yang Membatasi Desain (Fase 2 tidak boleh melampaui ini tanpa keputusan produk)

1. **Tidak ada identitas warga**: tak ada "janji temu saya", riwayat, cancel/reschedule oleh pasien. Reschedule tak mungkin di UI; cancel hanya oleh admin.
2. **Tidak ada search/pagination server** di daftar appointments, queues, schedules, service-units, facilities. Filter = klien saja. Notifikasi satu-satunya ber-filter server [D3]: **limit terverifikasi** — default 100, nilai hanya dihormati bila `0 < n ≤ 500` (maks. 500 baris/permintaan); nilai di luar itu diam-diam mundur ke default 100. Frontend saat ini mengirim `limit=200`. **Tidak ada kontrak paginasi server apa pun** (tanpa offset, page, cursor, next-token) [R3-F1] — UI Fase 2 hanya boleh memotong/membatasi dataset yang sudah termuat di klien (filter + jumlah hasil + manajemen baris termuat). **Paginasi server sejati = kemampuan backend masa depan, bukan fitur produk saat ini** — tidak boleh dirancang seolah sudah ada.
3. **Catalog publik minim** (nama, tipe, short code, unit layanan — tanpa alamat/telepon/kasur/jarak): "Cari Faskes" hanya bisa menampilkan itu. Bed-availability & peta = demo, jangan masuk produk.
4. **`facilities/nearby` hardcoded (f1–f6, bukan UUID DB)**; `events/beds` SSE hanya membawa event `queue_created`.
5. **`estimated_wait_minutes` konstan 25** (engine), bukan dihitung.
6. **Schedules single-date, tanpa rekurensi, tanpa deteksi bentrok antar-praktisi, tanpa aturan jam operasi** (waktu masa depan apa pun diterima tanpa schedule).
7. **Praktisi: tanpa API CRUD** — `practitioner_id` di form jadwal hanya input bebas; tak ada list praktisi untuk dipajang.
8. **Notifikasi: 2 template (booking & check-in), delivery disimulasikan (DevProvider ~75%), tanpa read-state citizen, tanpa reminder / gerak-antrean.**
9. **Scope NULL = scope kosong** untuk role global non-dev — seed demo wajib punya baris `user_roles` ber-fasilitas agar admin nyata melihat data.
10. **Dev identity 403 pada `schedule.manage` & tulis `service-units`** — form terkait menampilkan ForbiddenPanel, bukan error merah.
11. **Rate limits persis**: 2/hari (booking & antrean per HP), 5/5menit check-in, 30/menit status — pesan 429 backend dipakai apa adanya.
12. **Audit log & medical-records ada di DB tapi tanpa endpoint read** — jangan bangun UI "Riwayat"/"Log Audit" sekarang.
13. **Duplikat booking (faskes+HP+hari) memblok via partial-unique index → 500 generik** [D4]. Tidak boleh diasumsikan bahwa semua 500 = duplikat; sampai backend memberi kode error stabil, UI memperlakukannya sebagai **error server generik** (panel + "coba lagi", tanpa copy spesifik). **Rekomendasi perbaikan backend (diluar fase desain, jangan diimplementasikan sekarang):** kembalikan konflik deterministik — idealnya **HTTP 409 + kode error machine-readable** (mis. `DUPLICATE_APPOINTMENT`) dari `booking.go` saat `ERR_DUPLICATE_KEY` / violation 23505 terdeteksi, dengan pesan error yang tetap berbahasa Indonesia.
14. **Keamanan yang WAJIB dipertahankan redesign**: cookie Supabase SSR, Bearer forwarding, `SIGAP_DEV_IDENTITY` + dev header, CSRF (action SvelteKit + fetch proxy sendiri), RBAC DB (peran diselesaikan server; klaim token tak dipercaya), header keamanan di hooks (CSP, X-Frame-Options), masking recipient notifikasi (UI TIDAK PERNAH menampilkan kontak asli/hash).

---

## Status Keputusan (r.2 + r.3)

Semua item checklist r.1 telah dijawab oleh owner; keputusan D1–D14 diterapkan di seluruh dokumen ini. Dua koreksi faktual akhir r.3 (R3-F1, R3-F2) juga sudah diterapkan.

| Item | Keputusan |
|---|---|
| Dua shell + bottom-nav 4 tab | **Disetujui** [D11] |
| `/wallet` | **Dikecualikan** dari navigasi & Hi-Fi Fase 2; kode tetap [D7] |
| Polling 30 dtk Antrean | **Disetujui** + 4 syarat [D8] |
| Penyaring fasilitas | **Disetujui** sebagai filter klien [D9] |
| Istilah nav admin | **Disetujui** [D10] |
| 500 duplikat booking | **Tetap generik di UI**; rekomendasi backend 409 + kode stabil terpisah [D4] |
| Paginasi notifikasi | **Bukan paginasi server**; slice klien + filter + jumlah hasil; paginasi server = kemampuan backend masa depan [R3-F1] |
| Check-in vs walk-in | **Dua jalur terpisah eksplisit**: "Check-In Janji Temu" ≠ "Ambil Antrean Tanpa Janji"; kontrak tidak dilebur [R3-F2] |

**Tidak ada pertanyaan terbuka yang tersisa.** Fase 2 (layar Hi-Fi) menunggu persetujuan eksplisit owner; belum ada kode produksi yang disentuh.
