# Implementasi LapanganGo Fase 2: Venue Photos & Facilities

## 1. Overview
Fase ini menyelesaikan integrasi pengelolaan fasilitas venue (facilities) di UI Owner, serta menambahkan fungsionalitas manajemen foto venue asli dengan kemampuan menandai satu foto utama (`primary_photo`) per venue.

**Tugas yang telah diselesaikan:**
- Database migration untuk tabel `venue_photos`.
- Backend endpoints CRUD untuk foto (Add, Update, Delete).
- Backend repository + service layer untuk query foto secara massal (batch mapping) guna menghindari N+1 query.
- Modifikasi endpoint Create/Update/List Venue untuk menyertakan foto dan memastikan fasilitas ter-assign.
- Pembuatan frontend `EditVenuePage.tsx` untuk pengelolaan detail, fasilitas, dan foto secara langsung oleh owner.
- Modifikasi frontend public (`VenueCard.tsx`, `VenueDetailPage.tsx`) agar menggunakan `primary_photo` dan galeri foto.
- Update `demo-seed` command agar meng-generate sample photo secara otomatis dan aman (menggunakan placeholder URL).

## 2. File Terubah (Modified & Created)

### Backend (Golang)
- `[NEW]` `db/migrations/008_venue_photos.up.sql`: Skema tabel foto venue dengan index unik parsial.
- `[NEW]` `db/migrations/008_venue_photos.down.sql`: Script rollback tabel.
- `[MODIFY]` `apps/api/internal/venues/dto.go`: Penambahan `VenuePhotoResponse`, `CreateVenuePhotoRequest`, dan field pada response DTO Venue/PublicVenue.
- `[MODIFY]` `apps/api/internal/venues/repository.go`: Implementasi CRUD foto, logic `UnsetOtherPrimaryPhotos`, dan helper batch `FindPhotosByVenueIDs`.
- `[MODIFY]` `apps/api/internal/venues/service.go`: Update mapping fungsi respons, injeksi daftar foto ke venue, fungsi CRUD `AddVenuePhoto`, `UpdateVenuePhoto`, `DeleteVenuePhoto`.
- `[MODIFY]` `apps/api/internal/venues/handler.go`: Expose router `POST/PUT/DELETE /owner/venues/:id/photos`.
- `[MODIFY]` `apps/api/cmd/demo-seed/main.go`: Tambahkan query cleanup foto venue dan loop seeding dummy photos.

### Frontend (React/TypeScript)
- `[NEW]` `apps/web/src/pages/owner/EditVenuePage.tsx`: UI bagi Owner untuk mengedit data venue, memilih fasilitas multi-select, menambah/hapus foto, dan set foto utama.
- `[MODIFY]` `apps/web/src/types/venue.ts`: Penambahan tipe statis `VenuePhoto` dan update interface `Venue`.
- `[MODIFY]` `apps/web/src/lib/api.ts`: Penambahan client HTTP call untuk endpoint `updateOwnerVenue` dan manajemen foto.
- `[MODIFY]` `apps/web/src/App.tsx`: Pendaftaran routing `/owner/venues/:id/edit`.
- `[MODIFY]` `apps/web/src/pages/owner/OwnerVenuesPage.tsx`: Tampilkan foto utama sebagai cover venue card, tambahkan tombol "Edit Detail & Foto".
- `[MODIFY]` `apps/web/src/components/VenueCard.tsx`: Tampilkan cover foto utama di listing pencarian.
- `[MODIFY]` `apps/web/src/pages/VenueDetailPage.tsx`: Gunakan foto utama sebagai banner, serta tambahkan grid gallery di bawah section "Tentang Venue".

## 3. Hasil Pengujian (Verifications)
- **Backend Tests:** `cd apps/api && go test ./...` => **PASS** (100% build ok, no regressions in routing/services).
- **Frontend Lint:** `cd apps/web && npm run lint` => **PASS** (Resolved unused variables/imports warnings).
- **Frontend Build:** `cd apps/web && npm run build` => **PASS** (110 modules transformed successfully).
- **Demo Seed:** `cd apps/api && go run cmd/demo-seed/main.go` => **SUCCESS**. Cleanup berjalan bersih, seeding 11 venue kini mencakup dummy photos per venue.

## 4. Manual QA Guide (Testing by User)
Untuk mengetes fitur ini, silakan jalankan aplikasi seperti biasa dan buka browser:

1. **Owner View (Edit & Fasilitas)**
   - Login menggunakan **`demo.owner01@lapangango.test`** (pass: `password123`).
   - Masuk ke menu `Venue Saya`, perhatikan foto venue sudah tidak berupa placeholder icon.
   - Klik tombol **Edit Detail & Foto**.
   - **Tes Fasilitas:** Anda dapat menceklis/un-ceklis chip fasilitas di sana (misal: "Toilet", "Kantin"), lalu simpan.
   - **Tes Foto:** Di panel "Kelola Foto", Anda dapat meng-input URL gambar (bisa mencari di google images dan copy link address), hapus foto, atau jadikan foto sebagai **PRIMARY** (bintang).

2. **Public View (Gallery & Fasilitas)**
   - Logout, atau buka tab Incognito.
   - Buka beranda, perhatikan **VenueCard** sekarang memuat foto banner sesuai primary photo.
   - Klik salah satu venue.
   - Di halaman Detail, banner utama akan menggunakan foto primary.
   - Cek bagian bawah deskripsi, terdapat grid "Galeri Foto" yang menampilkan seluruh foto dari venue tersebut.

Semua spesifikasi dari Phase 2 (Facilities + Photos) ini telah selesai diimplementasikan dengan clean design sesuai identitas LapanganGo.
