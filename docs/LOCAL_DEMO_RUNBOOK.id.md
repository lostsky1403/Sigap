# Runbook Presentasi Langsung Sigap

> **English**: [LOCAL_DEMO_RUNBOOK.md](./LOCAL_DEMO_RUNBOOK.md) (versi acuan/authoritative).
> **Aturan sinkronisasi**: versi Inggris adalah sumber kebenaran; kedua berkas WAJIB diperbarui di PR yang sama setiap kali alur demo berubah. Jangan membuat panduan demo ketiga.

| | |
|---|---|
| **Audiens** | Presenter demo langsung 5–10 menit, dan operator yang menjaga stack tetap sehat |
| **Tugas** | Menyiapkan lingkungan, membawakan alur 7 layar standar, pulih dari gangguan umum, dan menjalankan checklist akhir sebelum presentasi |
| **Jenis** | How-to operasional (runbook) |
| **Sumber kebenaran** | Berkas ini bersama versi Inggris adalah satu-satunya panduan demo kanonik. `DEMO_FLOW.md` adalah stub pensiun; `DEMO_HANDOFF.md` adalah catatan checkpoint titik-waktu, bukan panduan. |
| **Verifikasi terakhir** | 2026-08-23 @ main `c755042`, bootstrap bare-metal Windows |
| **Pemicu pembaruan** | Jika port, `Start-LocalDev.ps1`, salah satu layar demo, perilaku seed, atau jumlah langkah smoke berubah — perbarui dokumen ini dan kembaran Inggrisnya **di PR yang sama** |

---

## Apa yang ditunjukkan demo ini

Sigap adalah platform informasi kesehatan dan antrean tingkat daerah: pasien
membuat janji temu dan check-in dari ponsel, petugas fasilitas menjalankan
konsol antrean, dan administrator melihat status fasilitas/bed seluruh wilayah
secara langsung — tanpa kerumitan teknis yang terlihat oleh siapa pun.

**Pengguna:** pasien (janji temu, check-in, cek status), petugas fasilitas
(konsol antrean, kelola janji temu), administrator daerah (fasilitas, bed,
notifikasi).

**Alur inti:** buat janji temu → terima kode check-in → check-in → dapat nomor
antrean beserta estimasi tunggu → petugas melayani kunjungan → status dapat
diverifikasi secara publik tanpa membuka data pribadi.

**Demo-only vs production-ready:**

| Kemampuan | Status saat demo |
|---|---|
| API booking / check-in / antrean, validasi, rate limit | Desain setara produksi |
| RBAC + audit event pada rute admin | Desain setara produksi |
| Penyamaran PII & lookup status publik yang aman privasi | Desain setara produksi (hanya data sintetis) |
| Queue engine (Rust) dengan tanda tangan traceability | Layanan nyata secara lokal; mode fallback hanya untuk demo |
| Header dev identity (`X-Sigap-Dev-User-ID`) | **Khusus demo** — jangan pernah di luar pengembangan lokal |
| Provider pengiriman notifikasi | **Khusus demo** — provider offline deterministik, tidak ada yang benar-benar terkirim |

Semua data demo bersifat sintetis: nama seperti `Pasien Demo*` dan nomor telepon
pada rentang fiksi ITU-T `+62-555-01xx`. Jangan pernah menggantinya dengan data
pasien sungguhan.

---

## Persiapan pra-demo

### 1. Bootstrap (satu perintah)

Membutuhkan PowerShell 7+, Go, Rust+`cargo` (+`protoc`), Node 20+/pnpm,
PostgreSQL, dan `psql` di PATH.

```powershell
# DATABASE_URL harus disetel di shell pemanggil (jangan pernah di-commit).
$env:DATABASE_URL = "postgresql://<user>:<password>@localhost:5434/sigap"
pwsh -NoProfile -File scripts/dev/Start-LocalDev.ps1
```

Skrip membuka tiga jendela:

| Jendela | Layanan | Alamat |
|---|---|---|
| 1 | Queue engine Rust (gRPC) | `127.0.0.1:50051` |
| 2 | Go API | `http://127.0.0.1:18080` |
| 3 | Web SvelteKit | `http://localhost:5173` |

Skrip juga menyetel `SIGAP_API_BASE=http://127.0.0.1:18080` di shell miliknya —
jika Anda menjalankan skrip smoke dari terminal *berbeda*, setel juga di sana:

```powershell
$env:SIGAP_API_BASE = "http://127.0.0.1:18080"
```

Tunggu ±5–10 detik setelah peluncuran sebelum memeriksa health.

### 2. Pastikan semuanya hidup

```powershell
Invoke-RestMethod http://127.0.0.1:18080/health    # -> ok / json status
Invoke-RestMethod http://127.0.0.1:18080/readyz    # -> ready (DB terjangkau)
Invoke-WebRequest -UseBasicParsing http://localhost:5173   # -> 200 OK
```

Engine: jendela queue-engine seharusnya menampilkan server gRPC listening di
`:50051`. (Jika engine tidak bisa jalan, restart API dengan
`SIGAP_ENGINE_FALLBACK=dev` — lihat Pemulihan.)

Port yang harus bebas: **5434** (PostgreSQL), **18080** (API), **50051**
(engine), **5173** (web). Cek:
`netstat -ano | findstr ":18080 :50051 :5173"`.

### 3. Reset / seed (aman, idempoten)

Seed bersifat aditif dan idempoten — dijalankan berulang tidak pernah
menduplikasi baris:

```powershell
psql $env:DATABASE_URL -f packages/db/seed/rbac.sql
psql $env:DATABASE_URL -f packages/db/seed/dev.sql     # fasilitas
psql $env:DATABASE_URL -f packages/db/seed/demo.sql    # jadwal, SMOKE01, outbox
```

- Semua data seed sintetis (`+62-555-01xx`, `Pasien Demo*`). Tidak ada yang
  dikirim ke penerima sungguhan.
- **Pulihkan baseline SMOKE01**: menjalankan ulang `demo.sql` mengembalikan
  janji temu seed dengan kode check-in `SMOKE01` (status `scheduled`) dan
  mereset baris outbox deterministik menjadi `pending`.
- Alternatif reset lengkap: `pwsh -NoProfile -File scripts/smoke/sigap-full-local-demo.ps1`
  melakukan seed dan menjalankan ketiga suite smoke dalam satu perintah
  (perlu `DATABASE_URL`, `SIGAP_DATABASE_URL`, dan `SIGAP_API_BASE` diekspor
  di shell tersebut).

---

## Naskah demo

Identitas presenter sintetis: nama **Demo Rehearsal Patient**, telepon
**08555001999** (rentang tes tercadang). Salin ID ke catatan seiring jalan —
layar 4–6 memakainya kembali.

### Layar 1 — Beranda (tinjauan langsung)

- **URL:** <http://localhost:5173>
- **Lakukan:** tunjuk papan ketersediaan bed; buka DevTools → tab Network dan
  sorot baris event stream `/api/v1/events/beds` (pembaruan langsung tanpa refresh).
- **Ucapkan:** "Status bed setiap fasilitas mengalir ke dasbor ini begitu
  berubah — inilah tampilan langsung yang dipakai administrator seluruh daerah."
- **Hasil diharapkan:** dasbor tampil dengan jumlah bed; koneksi SSE tetap
  terbuka; tidak ada error konsol.
- **Salin:** tidak ada.
- **Jangan tunjukkan:** detail payload network, jendela terminal.

### Layar 2 — Pembuatan janji temu

- **URL:** <http://localhost:5173/appointments/new>
- **Lakukan:** pilih **Sigap Demo Facility** → **Poli Umum Demo** dari dropdown;
  tanggal = besok, waktu = 09:30; nama `Demo Rehearsal Patient`; telepon
  `08555001999`; submit.
- **Ucapkan:** "Pasien memilih fasilitas dan poli semudah memilih produk —
  tanpa kode, tanpa formulir kertas. Sistem memvalidasi kapasitas dan
  mengembalikan kode check-in seketika."
- **Hasil diharapkan:** kartu hijau berisi kode check-in 6 karakter plus
  referensi janji temu.
- **Salin:** **kode check-in** dan **appointment_id**.
- **Jangan tunjukkan:** respons JSON mentah, header dev, UUID yang diketik manual.

### Layar 3 — Check-in pasien

- **URL:** gunakan tautan "→ Check-In sekarang" pada kartu sukses (atau
  <http://localhost:5173/appointments/check-in> lalu tempel kodenya).
- **Lakukan:** pastikan kedua kolom terisi otomatis (ID janji temu + kode);
  klik **Check-In**.
- **Ucapkan:** "Di pintu masuk, pasien cukup satu kode untuk check-in dan
  mendapat nomor antrean yang adil dengan estimasi tunggu yang jujur."
- **Hasil diharapkan:** panel sukses: nomor antrean `RSK-000x`, estimasi tunggu
  (~25 menit), status `queued`.
- **Salin:** **nomor antrean** (mis. `RSK-0002`).
- **Jangan tunjukkan:** internal engine/gRPC di balik pembuatan tiket.

### Layar 4 — Status pasien (lookup publik aman privasi)

- **URL:** <http://localhost:5173/patient/status>
- **Lakukan:** cek kode baru Anda dulu (tampil nomor antrean + status
  Menunggu); lalu klik "Cek kode lain" dan masukkan **SMOKE01** (menampilkan
  janji temu seed yang masih *Belum check-in*).
- **Ucapkan:** "Siapa pun yang memegang kode dapat memverifikasi status
  kunjungannya — dan hanya itu. Tanpa nama, tanpa nomor telepon, tanpa rekam
  medis yang bocor keluar."
- **Hasil diharapkan:** kartu status bersih untuk kedua kode; nol data pribadi
  yang ditampilkan.
- **Salin:** tidak ada.
- **Jangan tunjukkan:** isi respons API (jaga klaim privasi tetap visual).

### Layar 5 — Konsol antrean admin

- **URL:** <http://localhost:5173/admin/queues>
- **Lakukan:** temukan tiket Anda (`RSK-000x`, badge **Menunggu**). Klik
  **→ Dipanggil**, lalu **→ Dilayani**. Tekan **Muat** atau reload halaman dan
  tunjukkan state tersimpan.
- **Ucapkan:** "Petugas memindahkan pasien melalui kunjungan dengan satu klik.
  Setiap perubahan divalidasi terhadap transisi yang diizinkan dan tercatat di
  audit trail."
- **Hasil diharapkan:** badge berubah Menunggu → Dipanggil → Dilayani; state
  bertahan setelah reload.
- **Salin:** tidak ada.
- **Jangan tunjukkan:** jalur batal/lewati, tooling database.

### Layar 6 — Janji temu admin

- **URL:** <http://localhost:5173/admin/appointments>
- **Lakukan:** temukan booking hari ini ("Demo Rehearsal Patient", badge
  **Antre**); klik **Selesai**; tekan **Muat ulang** untuk membuktikan
  persistensi. Opsional: gunakan filter status.
- **Ucapkan:** "Siklus janji temu mencerminkan kenyataan dari awal sampai
  akhir — terjadwal, check-in, antre, selesai."
- **Hasil diharapkan:** badge berubah menjadi **Selesai** dan tetap setelah reload.
- **Salin:** tidak ada.
- **Jangan tunjukkan:** alur Batal/Tidak Hadir (hindari kesan menyalahkan pasien).

### Layar 7 — Fasilitas admin (tinjauan saja)

- **URL:** <http://localhost:5173/admin/facilities>
- **Lakukan:** scroll singkat; sebutkan bahwa pengeditan kapasitas tersedia.
- **Ucapkan:** "Registri fasilitas menjadi fondasi semua hal di hulu — bed di
  dasbor, dropdown saat booking, antrean per lokasi."
- **Hasil diharapkan:** daftar fasilitas tampil.
- **Salin:** tidak ada. **Jangan lakukan operasi CRUD secara langsung.**

---

## Langkah pemulihan

| Gangguan | Tindakan |
|---|---|
| **API tidak tersedia** (health gagal) | Periksa jendela API apakah crash. Restart stack: tutup jendela hasil spawn, jalankan ulang `scripts/dev/Start-LocalDev.ps1`, tunggu ±10 detik, cek ulang `/health` dan `/readyz`. |
| **Engine tidak terjangkau** (check-in gagal dengan error antrean) | Restart jendela engine (`cd apps/queue-engine; cargo run`). Jika tidak bisa jalan, matikan jendela API, set `$env:SIGAP_ENGINE_FALLBACK = "dev"`, luncurkan ulang via `Start-LocalDev.ps1` — check-in tetap bekerja lewat fallback berbasis database. |
| **Konflik port** (jendela langsung tertutup) | `netstat -ano | findstr ":<port>"` untuk 5434/18080/50051/5173; hentikan proses yang bentrok atau bebaskan portnya, lalu luncurkan ulang layanan tersebut. |
| **State browser basi** (UI lama setelah perubahan kode) | Hard reload: Ctrl+F5 / reload dari DevTools yang menghapus cache; buka kembali `http://localhost:5173/appointments/new`. |
| **Antrean sudah ditransisi** (tidak ada tombol Menunggu) | Pilih tiket lain yang masih **Menunggu**, atau buat baru: booking lagi (Layar 2) lalu check-in (Layar 3). Transisi bersifat satu arah by design. |
| **Tidak ada tiket menunggu** | Sama seperti di atas — book + check-in janji temu sintetis baru; kurang dari satu menit. |
| **Seed drift** (SMOKE01 tidak dikenal, daftar fasilitas/unit aneh) | Jalankan ulang tiga perintah seed dari [Reset / seed](#3-reset--seed-aman-idempoten); `demo.sql` memperbaiki linkage service unit demo sendiri dan mengembalikan SMOKE01. Lalu hard reload browser. |
| **Halaman admin 401/403 / dev identity hilang** | Rute admin butuh dev identity aktif saat API start: pastikan jendela API diluncurkan oleh `Start-LocalDev.ps1` (menyetel `SIGAP_AUTH_MODE=dev` + `SIGAP_DEV_IDENTITY=true`). Restart stack jika API dijalankan manual tanpa itu. |
| **Booking bilang waktu harus di masa depan** | Gunakan tanggal besok; pastikan jam shell/API/PostgreSQL tersinkronisasi NTP. |
| **Skrip smoke mengarah ke port 8080** | Berikan `-ApiBase http://127.0.0.1:18080` (atau ekspor `SIGAP_API_BASE`) — runtime bare-metal memakai 18080, bukan default skrip. |

---

## Checklist validasi final (tepat sebelum presentasi)

Jalankan dari atas ke bawah; berhenti dan perbaiki jika ada yang gagal:

```powershell
Invoke-RestMethod http://127.0.0.1:18080/health          # 200
Invoke-RestMethod http://127.0.0.1:18080/readyz           # 200
$env:SIGAP_API_BASE = "http://127.0.0.1:18080"
pwsh -NoProfile -File scripts/smoke/sigap-demo-smoke.ps1  # 8/8 PASS
```

- [ ] `/health` mengembalikan 200
- [ ] `/readyz` mengembalikan 200
- [ ] Beranda termuat (<http://localhost:5173>) dengan dasbor bed
- [ ] Booking bekerja (dropdown terisi; kode baru dikembalikan)
- [ ] Check-in bekerja (nomor antrean diterbitkan)
- [ ] Status pasien bekerja (kode baru + `SMOKE01`)
- [ ] Admin antrean bekerja (daftar + transisi)
- [ ] Admin janji temu bekerja (daftar + satu transisi)
- [ ] Suite smoke: 8/8 PASS
- [ ] Konsol browser: tanpa error (favicon 404 yang jinak boleh muncul)

Gerbang lebih dalam (opsional): `sigap-full-local-demo.ps1` → harapkan
**FULL LOCAL DEMO: PASS**.

---

## Bacaan lanjutan

- [`LOCAL_DEMO_RUNBOOK.md`](./LOCAL_DEMO_RUNBOOK.md) — versi Inggris (acuan)
- [`../scripts/smoke/README.md`](../scripts/smoke/README.md) — parameter smoke, exit code
- [`DEV_SETUP.md`](./DEV_SETUP.md) — setup developer lengkap dan mode auth
- [`DEMO_HANDOFF.md`](./DEMO_HANDOFF.md) — catatan checkpoint green terakhir
