# Antigravity Next Steps After Frontend MVP

Tujuan dokumen ini adalah menjadi arahan kerja lanjutan untuk Antigravity setelah `Frontend MVP` mendapat status **APPROVED FOR MVP DEMO** dari Codex.

Besok, review sebaiknya dibagi menjadi dua bagian:

1. **Review Step Sekarang:** validasi live demo QA dari frontend MVP yang sudah dibuat.
2. **Step Besok:** lanjut ke hardening, backend contract enrichment, owner management nyata, dan demo polish.

---

## A. Step Sekarang - Live Demo QA Frontend MVP

Status Codex saat ini: **APPROVED FOR MVP DEMO**, bukan final production-ready.

Antigravity perlu melakukan live QA dengan backend asli dan demo seed besar.

### A1. Environment Wajib

Backend:

```bash
cd apps/api
go run ./cmd/demo-seed
go run ./cmd/api
```

Frontend:

```env
VITE_API_BASE_URL=http://localhost:8080
VITE_USE_MOCK_VENUE=false
VITE_USE_MOCK_MABAR=false
VITE_USE_MOCK_AUTH=false
```

Frontend run:

```bash
cd apps/web
npm run dev
```

### A2. Customer Flow Yang Wajib Dites

1. Login/register sebagai customer.
2. Buka homepage.
3. Pastikan venue dari backend tampil.
4. Pastikan Mabar dari backend tampil lewat `GET /open-matches`.
5. Buka detail venue.
6. Pilih court.
7. Buka availability.
8. Pilih slot valid.
9. Buat booking.
10. Masuk ke `/bookings`.
11. Klik `Konfirmasi Bayar`.
12. Pastikan status berubah menjadi `CONFIRMED`.
13. Klik `Jadikan Mabar`.
14. Isi form Mabar.
15. Pastikan Mabar berhasil dibuat.
16. Buka detail Mabar.
17. Login sebagai customer lain.
18. Join Mabar.
19. Leave Mabar.

### A3. Owner Flow Yang Wajib Dites

1. Login sebagai owner seed.
2. Pastikan diarahkan ke `/owner/dashboard`.
3. Buka `/owner/venues`.
4. Pastikan venue owner tampil.
5. Klik `Kelola Court`.
6. Pastikan daftar court tampil.
7. Kembali ke venue list.
8. Klik `Lihat Pesanan`.
9. Pastikan booking customer yang sudah dibuat muncul di `/owner/venues/:id/bookings`.

### A4. Hal Yang Harus Dicatat Antigravity

Antigravity harus membuat laporan dengan format:

- Flow yang berhasil.
- Flow yang gagal.
- Screenshot error jika ada.
- Endpoint yang gagal jika ada.
- Status lint/build.
- Gap backend yang ditemukan.
- Gap frontend yang ditemukan.

---

## B. Step Besok - Backend Contract Enrichment

Step ini bertujuan membuat frontend lebih realistis dan mengurangi placeholder/UUID.

### B1. Enrich Customer Booking Response

Masalah saat ini:

- `GET /bookings` dan `GET /bookings/:id` masih minim.
- Frontend sering harus fallback ke `Lapangan #UUID`.

Target backend:

Tambahkan summary data ke `BookingResponse`:

```json
{
  "id": "...",
  "customer_id": "...",
  "court_id": "...",
  "court": {
    "id": "...",
    "name": "Lapangan A",
    "sport_name": "Mini Soccer"
  },
  "venue": {
    "id": "...",
    "name": "GBK Alpha Field",
    "address": "...",
    "city": "Jakarta"
  },
  "booking_date": "2026-06-25",
  "start_time": "19:00",
  "end_time": "21:00",
  "total_price": 450000,
  "status": "CONFIRMED"
}
```

Frontend update:

- Hilangkan fallback UUID jika `court` dan `venue` tersedia.
- Tampilkan venue/court/sport dengan rapi di `/bookings`.
- Tampilkan info yang sama di owner booking page jika relevant.

### B2. Enrich Open Match Detail Response

Masalah saat ini:

- Frontend mendeteksi host dengan `user.name === match.host_name`.
- Ini tidak aman karena nama tidak unik.

Target backend:

Tambahkan `host_user_id` ke Open Match response:

```json
{
  "id": "...",
  "host_user_id": "...",
  "host_name": "Bima Aditya"
}
```

Frontend update:

- `isHost = user?.id === match.host_user_id`.
- Tombol `Batalkan Mabar (Host)` hanya muncul untuk host asli.

### B3. Align Level Badge Values

Masalah saat ini:

- Backend memakai:
  - `Beginner / Fun`
  - `Intermediate`
  - `Advanced`
  - `All Levels`
- Frontend badge color masih sebagian membaca enum lama.

Target:

- Update color mapping agar sesuai value backend.
- Pastikan UI tidak fallback gray untuk level valid.

---

## C. Step Besok - Owner Management Real Actions

Step ini jangan membuat UI palsu. Jika endpoint belum ada, catat sebagai backend gap.

### C1. Owner Court Management

Target ideal:

- Owner bisa melihat court.
- Owner bisa edit basic court info jika endpoint tersedia.
- Owner bisa melihat operating hours.
- Owner bisa melihat blocked slots.

Jika endpoint belum tersedia:

- Jangan aktifkan tombol `Edit Info` dan `Atur Jadwal` sebagai aksi palsu.
- Jadikan disabled dengan label `Segera Hadir`, atau hapus dari MVP.
- Catat kebutuhan backend endpoint.

### C2. Owner Booking Management

Target:

- Owner bisa melihat booking per venue.
- Tambahkan filter status:
  - `PENDING_PAYMENT`
  - `CONFIRMED`
  - `PAID`
  - `CANCELLED`
- Tambahkan filter tanggal.
- Pastikan query mengikuti backend `GET /owner/venues/:id/bookings?date=YYYY-MM-DD&status=...`.

### C3. Owner Dashboard Metrics

Jangan tampilkan angka palsu.

Jika backend belum punya aggregate endpoint:

- Tampilkan CTA yang benar.
- Atau hitung sederhana dari data venue/bookings yang benar-benar di-fetch.
- Catat kebutuhan endpoint dashboard metrics.

---

## D. Step Besok - Frontend UX Polish

### D1. Replace Browser Alert

Masalah:

- Beberapa flow masih memakai `alert()` dan `window.confirm()`.

Target:

- Ganti dengan modal/confirm UI internal.
- Error action tampil sebagai inline banner/toast.
- UX cancel booking dan cancel mabar terasa profesional.

### D2. Mobile Navigation

Pastikan navbar usable di mobile:

- Customer bisa akses `Pesanan Saya`.
- Owner bisa akses `Dashboard` dan `Kelola Venue`.
- Tidak hanya hidden desktop nav.

### D3. Empty/Loading/Error Consistency

Semua page wajib punya:

- Loading state.
- Empty state.
- Error state dengan retry.
- Unauthorized redirect.

### D4. Visual QA

Antigravity perlu screenshot minimal:

- Homepage desktop.
- Homepage mobile.
- Customer bookings desktop.
- Mabar detail desktop.
- Owner venues desktop.
- Owner bookings desktop.

---

## E. Step Besok - Automated E2E Readiness

Jika memungkinkan, mulai siapkan automated E2E smoke test.

Target minimal:

- Test login customer.
- Test browse venue.
- Test create booking.
- Test confirm payment.
- Test create mabar.
- Test owner opens venue bookings.

Jika Playwright belum tersedia:

- Buat dokumen manual QA step-by-step dulu.
- Catat command dan data login seed yang digunakan.

---

## F. Output Report Yang Harus Dikirim Besok

Antigravity harus mengirim laporan baru:

```text
docs/ANTIGRAVITY_FRONTEND_MVP_LIVE_QA_AND_HARDENING_REPORT_FOR_CODEX.md
```

Isi wajib:

1. Ringkasan status.
2. Apa saja yang dites dari Step Sekarang.
3. Hasil live backend smoke test.
4. Apa saja yang dikerjakan di Step Besok.
5. File yang berubah.
6. Endpoint backend yang dipakai/diubah.
7. Hasil:
   - npm run lint
   - npm run build
   - go test ./... jika backend diubah
8. Screenshot path jika ada.
9. Gap yang masih tersisa.
10. Klaim status akhir:
   - `APPROVED FOR DEMO`
   - `REQUEST REVIEW`
   - atau `BLOCKED`

---

## Prompt Siap Kirim Ke Antigravity

```text
Lanjutkan pekerjaan setelah Frontend MVP mendapat status APPROVED FOR MVP DEMO dari Codex.

Besok saya ingin mereview dua hal sekaligus:
1. Step sekarang: live demo QA atas Frontend MVP yang sudah dibuat.
2. Step besok: hardening dan peningkatan kontrak data agar demo makin realistis.

Kerjakan dengan urutan berikut:

A. Live Demo QA
- Jalankan backend dengan demo seed besar.
- Jalankan frontend dengan semua mock false:
  VITE_USE_MOCK_VENUE=false
  VITE_USE_MOCK_MABAR=false
  VITE_USE_MOCK_AUTH=false
- Test customer flow:
  venue browse -> court availability -> create booking -> confirm payment -> create mabar -> open mabar detail -> join/leave.
- Test owner flow:
  login owner -> dashboard -> owner venues -> courts -> venue bookings.
- Catat semua hasil sukses/gagal.

B. Backend Contract Enrichment
- Jika memungkinkan, update BookingResponse agar GET /bookings dan GET /bookings/:id menyertakan court dan venue summary.
- Jika memungkinkan, update OpenMatch response agar menyertakan host_user_id.
- Update frontend agar tidak bergantung pada UUID panjang dan tidak mendeteksi host dari nama.

C. Owner Management Hardening
- Jangan biarkan tombol owner edit/schedule menjadi aksi palsu.
- Jika endpoint belum ada, disable/hapus tombol dan catat sebagai backend gap.
- Tambahkan filter status/tanggal di owner venue bookings jika backend mendukung.

D. Frontend Polish
- Ganti alert/window.confirm dengan UI modal/banner/toast internal jika sempat.
- Pastikan mobile navigation usable.
- Pastikan semua halaman punya loading, empty, error, unauthorized state.

E. Verification
- npm run lint
- npm run build
- Jika backend berubah, jalankan go test ./...
- Lakukan manual smoke test live backend.

Output akhir:
buat laporan di:
docs/ANTIGRAVITY_FRONTEND_MVP_LIVE_QA_AND_HARDENING_REPORT_FOR_CODEX.md

Laporan harus berisi:
- ringkasan status
- flow yang dites
- file berubah
- endpoint dipakai/diubah
- hasil lint/build/test
- gap/blocker tersisa
- instruksi QA ulang
- screenshot path jika ada

Jangan klaim production-ready jika belum benar-benar lolos live backend smoke test.
```

