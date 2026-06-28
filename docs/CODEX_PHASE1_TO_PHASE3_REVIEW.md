# Codex Review: Antigravity Phase 1-3

Tanggal review: 26 Juni 2026

Peran review: Product Manager dan Expert Software Developer.

## Kesimpulan

Status: **REQUEST CHANGES / belum layak dianggap selesai penuh**.

Secara umum, Antigravity sudah menambah banyak fondasi yang benar: halaman detail booking, halaman search venue, create venue, modal court, modal operating hours, blocked slots, dan struktur owner metrics. Namun masih ada beberapa masalah kontrak API dan UX inti yang membuat Phase 2 dan Phase 3 belum bisa diterima sebagai selesai.

Build dan lint memang lolos, tetapi beberapa alur runtime kemungkinan gagal ketika dipakai pengguna.

## Temuan Prioritas Tinggi

### 1. Endpoint `GET /owner/metrics` belum terdaftar

Frontend sudah memanggil:

- `apps/web/src/lib/api.ts` -> `GET /owner/metrics`

Backend memang punya handler:

- `apps/api/internal/owners/handler.go` -> `GetMetrics`

Namun route owner saat ini hanya mendaftarkan:

- `POST /owner/profile`
- `GET /owner/profile`
- `PUT /owner/profile`

Tidak ada pendaftaran `GET /owner/metrics` di `RegisterRoutes` atau `main.go`.

Dampak:

- Dashboard owner akan gagal mengambil metrics.
- Phase 3 belum bisa dianggap selesai.

Catatan tambahan:

- Handler `GetMetrics` membaca context key `user_id`, sedangkan middleware auth menyimpan `auth_user_id`.
- Jadi walaupun route ditambahkan, handler masih berpotensi mengembalikan Unauthorized kecuali key-nya disamakan.

Rekomendasi:

- Daftarkan `ownerGroup.GET("/metrics", h.GetMetrics)`.
- Ganti `c.Get("user_id")` menjadi helper yang sama dengan profile, yaitu `getAuthenticatedUserID(c)`.
- Tambahkan test route/handler untuk memastikan owner token valid bisa mengambil metrics.

### 2. Payload update operating hours tidak cocok dengan backend

Frontend mengirim:

```json
{
  "operating_hours": [...]
}
```

Backend mengharapkan:

```json
{
  "days": [...]
}
```

Dampak:

- Modal `OperatingHoursModal` kemungkinan gagal menyimpan jam operasional dengan response 400.
- Ini memblokir salah satu fitur utama Owner Self-Service.

Rekomendasi:

- Ubah `updateOperatingHours()` agar mengirim `{ "days": data }`.
- Tambahkan test/manual verification eksplisit untuk save operating hours.

### 3. Owner court management memakai public venue detail

`OwnerCourtsPage.tsx` mengambil data via `fetchVenueById(venueId)`, yaitu endpoint publik.

Dampak:

- Owner management tidak mengambil data dari perspektif owner.
- Venue/court yang belum public/active bisa tidak muncul.
- Status court owner seperti inactive/maintenance berpotensi tidak tersedia.
- Setelah owner membuat venue baru, halaman kelola lapangan bisa gagal atau kosong jika venue belum memenuhi filter publik.

Rekomendasi:

- Gunakan owner endpoint untuk detail venue/courts.
- Jika belum ada endpoint detail owner venue, tambahkan `GET /owner/venues/:id`.
- Untuk list court gunakan endpoint owner: `GET /owner/venues/:id/courts`.

### 4. Court modal meminta raw `sport_id` UUID

UI saat ini meminta owner mengisi “ID Olahraga (Sport ID)” dan bahkan mencantumkan instruksi fetch manual dari database.

Dampak:

- Tidak layak untuk MVP booking website yang bisa dipakai owner.
- Owner biasa tidak mungkin tahu UUID olahraga.
- Risiko input error sangat tinggi.

Rekomendasi:

- Tambahkan endpoint/list source `GET /sports`.
- Ubah field `sport_id` menjadi dropdown/combobox nama olahraga.
- Jangan tampilkan konsep UUID ke user non-teknis.

## Temuan Prioritas Menengah

### 5. Klaim search/filter belum sesuai UI

Laporan menyebut filter City, Sport, Facilities, Price. Kode frontend saat ini hanya menyediakan:

- City
- Harga minimum
- Harga maksimum

Backend/API wrapper memang mendukung `sport_id` dan `facility_ids`, tetapi UI belum menyediakan kontrol sport/facility.

Rekomendasi:

- Lengkapi filter sport dan fasilitas, atau revisi laporan agar tidak overclaim.

### 6. Payment proof flow masih berupa simulasi konfirmasi

Halaman detail booking menampilkan instruksi pembayaran manual dan tombol konfirmasi, tetapi belum ada upload bukti bayar, nomor referensi, atau proses verifikasi owner/admin.

Rekomendasi:

- Untuk demo boleh disebut “manual payment confirmation simulation”.
- Jangan disebut “proof flow” sampai ada bukti pembayaran yang benar-benar tersimpan.

### 7. Masih memakai `window.confirm()` dan `alert()`

File terkait:

- `CustomerBookingDetailPage.tsx`
- `BlockedSlotsModal.tsx`

Dampak:

- Tidak konsisten dengan polish UI sebelumnya yang sudah memakai modal/toast custom.
- Pengalaman mobile/desktop terasa belum matang.

Rekomendasi:

- Gunakan `ConfirmModal` untuk cancel booking, confirm payment, dan delete blocked slot.
- Gunakan inline error/toast, bukan `alert()`.

### 8. Create venue mengirim latitude/longitude default `0`

`CreateVenuePage.tsx` menginisialisasi:

- `latitude: 0`
- `longitude: 0`

Padahal tidak ada input koordinat di UI.

Dampak:

- Semua venue baru bisa tersimpan di koordinat 0,0.
- Data lokasi menjadi salah.

Rekomendasi:

- Kirim `null`/omit field jika user tidak mengisi koordinat.
- Tambahkan input koordinat atau integrasi map/geocoding di fase berikutnya.

### 9. Halaman `/venues` belum discoverable dari navbar

Route `/venues` sudah ada, tetapi link “Temukan Venue” di navbar masih mengarah ke `/`.

Rekomendasi:

- Ubah link “Temukan Venue” ke `/venues`, atau pastikan homepage memang menjadi entry search utama.

## Catatan Product Manager

Untuk standar website booking lapangan, urutan prioritas setelah perbaikan blocker:

1. Customer harus bisa menemukan venue dan court dengan filter yang masuk akal.
2. Customer harus bisa booking, melihat detail, membayar/konfirmasi, dan membatalkan sesuai aturan.
3. Owner harus bisa membuat venue, menambah lapangan, mengatur jam buka, memblokir jadwal, dan melihat booking masuk.
4. Dashboard boleh sederhana, tetapi jangan menampilkan data palsu atau endpoint yang belum tersambung.

Antigravity sudah bergerak ke arah yang benar, tetapi Phase 2 dan Phase 3 perlu diperbaiki sebelum lanjut ke polish lebih jauh.

## Verification Codex

Perintah yang dijalankan:

```bash
cd apps/api && go test ./...
cd apps/web && npm run lint
cd apps/web && npm run build
```

Hasil:

- Backend test: lulus setelah `GOCACHE` diarahkan ke workspace lokal.
- Frontend lint: lulus.
- Frontend build: lulus.

Catatan:

- Lulus build/test belum cukup membuktikan fitur runtime benar, karena bug route dan payload di atas tidak tertangkap oleh test yang ada.

## Prompt Untuk Antigravity

Tolong lanjutkan perbaikan Phase 1-3 berdasarkan review Codex berikut.

Prioritas wajib:

1. Perbaiki `GET /owner/metrics`:
   - Daftarkan route `GET /owner/metrics`.
   - Gunakan context key auth yang benar, yaitu `auth_user_id`, atau helper `getAuthenticatedUserID`.
   - Pastikan dashboard owner tidak lagi gagal fetch metrics.

2. Perbaiki save operating hours:
   - Frontend harus mengirim payload `{ "days": [...] }`, sesuai kontrak backend.
   - Verifikasi manual bahwa modal jam buka berhasil menyimpan perubahan.

3. Perbaiki OwnerCourtsPage:
   - Jangan gunakan public `fetchVenueById` untuk halaman owner.
   - Gunakan owner endpoint untuk venue/courts.
   - Jika endpoint detail owner venue belum ada, tambahkan endpoint yang sesuai.

4. Perbaiki CourtModal:
   - Jangan meminta owner mengisi raw UUID `sport_id`.
   - Tambahkan list sport dan tampilkan dropdown nama olahraga.

5. Rapikan klaim dan UI:
   - Lengkapi filter sport/facility di `/venues`, atau revisi laporan jika belum dikerjakan.
   - Ganti `window.confirm()` dan `alert()` dengan `ConfirmModal`/toast/inline error.
   - Jangan kirim latitude/longitude default `0` jika user tidak mengisi koordinat.

Setelah selesai, kirim report baru yang berisi:

- File yang diubah.
- Bukti route metrics sudah registered.
- Bukti payload operating hours sesuai backend.
- Hasil `go test ./...`.
- Hasil `npm run lint`.
- Hasil `npm run build`.
- Manual verification untuk owner dashboard, owner court management, operating hours, blocked slots, customer booking detail, dan venue search.
