# AI Handoff - LapanganGo

## 1. Nama project dan tujuan project

Nama project: **LapanganGo**.

Tujuan project: API booking venue/lapangan olahraga. Project saat ini fokus pada backend untuk autentikasi, profil owner, manajemen venue, manajemen court/lapangan, jadwal operasional, blocked slot, dan cek availability court.

Catatan penamaan teknis:

- Root folder: `lapangGo`.
- Go module backend: `lapangango-api`.
- Nama container/database memakai ejaan `lapangango`.

## 2. Tech stack yang digunakan

- Backend: Go `1.26.4`.
- HTTP framework: Gin.
- Database: PostgreSQL 16.
- Driver/database access: `github.com/jackc/pgx/v5`.
- Auth: JWT `github.com/golang-jwt/jwt/v5`.
- Password hashing: bcrypt dari `golang.org/x/crypto`.
- Env loader: `github.com/joho/godotenv`.
- Docker: Docker Compose untuk PostgreSQL dan Redis.
- Redis: tersedia di `docker-compose.yml`, tetapi belum terlihat dipakai oleh kode backend.
- Frontend: belum ada aplikasi frontend runnable. Folder `apps/web` ada tetapi kosong. Ada preview UI statis di `docs/design/lapangango-ui-preview.html`.

## 3. Struktur folder penting

```text
.
|-- apps/
|   |-- api/
|   |   |-- cmd/api/main.go              # entrypoint HTTP API
|   |   |-- internal/auth/               # register, login, JWT, user lookup
|   |   |-- internal/availability/       # public court availability
|   |   |-- internal/blockedslots/       # blocked slot owner
|   |   |-- internal/config/             # env config
|   |   |-- internal/courts/             # owner court management
|   |   |-- internal/database/           # PostgreSQL pool
|   |   |-- internal/middleware/         # auth and role middleware
|   |   |-- internal/owners/             # owner profile
|   |   |-- internal/schedules/          # court operating hours
|   |   `-- internal/venues/             # owner venue management
|   `-- web/                             # currently empty
|-- db/migrations/                       # SQL migrations
|-- docs/design/lapangango-ui-preview.html
|-- docker-compose.yml
`-- README.md
```

## 4. Cara menjalankan frontend

Belum ada frontend runnable.

Kondisi saat ini:

- `apps/web` kosong.
- Tidak ada `package.json`, Vite, Next.js, atau tooling frontend lain.
- Preview desain bisa dibuka langsung dari file:

```bash
docs/design/lapangango-ui-preview.html
```

## 5. Cara menjalankan backend

Pastikan PostgreSQL sudah berjalan dan env sudah tersedia.

```bash
cd apps/api
cp .env.example .env
go run ./cmd/api
```

Default port dari config adalah `8080`, sehingga API berjalan di:

```text
http://localhost:8080
```

Health check:

```bash
curl http://localhost:8080/health
curl http://localhost:8080/db-health
```

## 6. Cara menjalankan database/docker jika ada

Docker Compose tersedia di root project.

```bash
docker compose up -d
```

Service yang dibuat:

- PostgreSQL 16 di port `5432`.
- Redis 7 di port `6379`.

Migrations ada di `db/migrations` dan perlu dijalankan berurutan secara manual dengan tool PostgreSQL/migration tool pilihan. Belum ada migration runner khusus di repo.

Urutan migration saat ini:

1. `db/migrations/001_init_core.sql`
2. `db/migrations/002_owner_profiles.sql`
3. `db/migrations/003_venues_and_courts.sql`

## 7. Environment variable yang dibutuhkan

Backend membaca env dari `apps/api/.env` atau environment system. Jangan commit value secret.

| Variable | Required | Default | Catatan |
| --- | --- | --- | --- |
| `APP_PORT` | Tidak | `8080` | Port HTTP API. |
| `DATABASE_URL` | Ya | Tidak ada | PostgreSQL connection string. Value lokal ada di `.env.example`, tetapi untuk production harus disediakan sendiri. |
| `JWT_SECRET` | Ya | Tidak ada | Secret JWT. Jangan tampilkan atau commit value real. |
| `JWT_EXPIRES_IN_HOURS` | Tidak | `24` | Harus angka positif jika diisi. |

## 8. Endpoint backend yang sudah ada

Health:

- `GET /health`
- `GET /db-health`

Authentication:

- `POST /auth/register`
- `POST /auth/login`
- `GET /auth/me`

Public availability:

- `GET /courts/:id/availability?date=YYYY-MM-DD`

Owner profile, perlu Bearer token dan role `OWNER`:

- `POST /owner/profile`
- `GET /owner/profile`
- `PUT /owner/profile`

Owner venues, perlu Bearer token dan role `OWNER`:

- `POST /owner/venues`
- `GET /owner/venues`
- `GET /owner/venues/:id`
- `PUT /owner/venues/:id`
- `PATCH /owner/venues/:id/status`

Owner courts, perlu Bearer token dan role `OWNER`:

- `POST /owner/venues/:id/courts`
- `GET /owner/venues/:id/courts`
- `GET /owner/courts/:id`
- `PUT /owner/courts/:id`
- `PATCH /owner/courts/:id/status`

Owner schedules, perlu Bearer token dan role `OWNER`:

- `GET /owner/courts/:id/operating-hours`
- `PUT /owner/courts/:id/operating-hours`

Owner blocked slots, perlu Bearer token dan role `OWNER`:

- `POST /owner/courts/:id/blocked-slots`
- `GET /owner/courts/:id/blocked-slots`
- `DELETE /owner/blocked-slots/:id`

## 9. Fitur yang sudah selesai

- Health check API dan database.
- Registrasi customer publik.
- Login dengan JWT.
- Endpoint `GET /auth/me`.
- Middleware autentikasi Bearer token.
- Middleware role, termasuk proteksi route owner dengan role `OWNER`.
- CRUD owner profile.
- CRUD venue milik owner, termasuk facility IDs dan update status.
- CRUD court milik owner, termasuk sport, location type, surface, price, dan update status.
- Pengaturan operating hours per court untuk 7 hari.
- Blocked slot per court untuk maintenance atau alasan lain.
- Public availability court berbasis tanggal, operating hours, status venue/court, dan blocked slot.
- Schema database core: users, sports, facilities, owner profiles, venues, venue facilities, courts, operating hours, blocked slots.
- Seed master data untuk sports dan facilities.
- Unit test untuk auth, JWT, role middleware, venues, courts, schedules, blocked slots, dan availability.

## 10. Fitur yang sedang dikerjakan

Indikasi dari working tree saat dokumentasi ini dibuat:

- Modul `apps/api/internal/availability/` masih untracked.
- `apps/api/cmd/api/main.go` sedang berubah untuk mendaftarkan route availability.
- `README.md` sedang berubah untuk menambahkan dokumentasi endpoint availability.

Artinya public court availability tampaknya fitur terbaru yang sedang/baru saja dikerjakan. Test dan build saat ini lulus, tetapi perubahan tersebut belum terlihat sebagai commit bersih.

Selain itu:

- `docs/design/lapangango-ui-preview.html` ada sebagai preview UI statis.
- `apps/web` masih kosong, jadi frontend aplikasi sebenarnya belum berjalan.

## 11. Fitur yang belum dikerjakan

- Frontend runnable di `apps/web`.
- Public listing/search venue.
- Public listing/search court.
- Public detail venue/court selain availability by court ID.
- Booking/reservation flow.
- Order/payment/invoice.
- Customer booking history.
- Owner dashboard frontend.
- Owner registration/onboarding publik. Saat ini public registration hanya menerima role `CUSTOMER`; owner account perlu dibuat lewat jalur lain.
- Admin/super admin endpoints.
- Verifikasi owner profile oleh admin.
- Integrasi Redis di kode aplikasi.
- Migration runner/CLI khusus.
- CI pipeline.
- Lint command/config formal.

## 12. Aturan coding style yang sedang dipakai

- Standard Go formatting dengan `gofmt`.
- Struktur domain konsisten: `dto.go`, `repository.go`, `service.go`, `handler.go`.
- Handler Gin bertanggung jawab untuk request binding, membaca param/query, dan response JSON.
- Service berisi business validation dan sentinel error.
- Repository berisi SQL langsung memakai `pgx`.
- Error domain dideklarasikan sebagai package-level sentinel errors, lalu dipetakan ke HTTP response di handler.
- Request validation banyak memakai tag Gin binding di DTO.
- ID UUID divalidasi manual di beberapa handler sebelum masuk service.
- Response JSON memakai snake_case field tags.
- Test memakai package `testing` bawaan Go, tanpa test framework tambahan.

## 13. Masalah/error yang sedang ada

Tidak ada test/build error terdeteksi saat dokumentasi ini dibuat.

Hasil verifikasi lokal:

```bash
cd apps/api
go test ./...
go build ./cmd/api
```

Keduanya lulus.

Catatan masalah/gap yang perlu diperhatikan:

- Public registration sengaja hanya mendukung `CUSTOMER`; endpoint owner butuh user role `OWNER`, sehingga perlu jalur seed/manual/admin untuk membuat owner sampai onboarding dibuat.
- Redis ada di Docker Compose tetapi belum dipakai kode.
- Belum ada migration runner; migration SQL harus dijalankan manual.
- Tidak ada frontend runnable.
- File preview UI `docs/design/lapangango-ui-preview.html` terlihat memiliki teks judul dengan karakter mojibake di sekitar dash pada hasil baca terminal, kemungkinan isu encoding pada file preview.

## 14. Perintah test, lint, dan build

Test backend:

```bash
cd apps/api
go test ./...
```

Build backend:

```bash
cd apps/api
go build ./cmd/api
```

Format Go:

```bash
cd apps/api
gofmt -w .
```

Lint:

- Belum ada command/config lint formal di repo.
- Jika hanya memakai tool Go bawaan, bisa mulai dari:

```bash
cd apps/api
go vet ./...
```

Frontend:

- Belum ada command test/lint/build karena `apps/web` masih kosong.

## 15. Catatan penting agar AI lain tidak salah memahami project

- Jangan anggap frontend sudah ada. `apps/web` kosong; preview HTML di `docs/design` bukan aplikasi frontend.
- Jangan tampilkan value dari `apps/api/.env`; gunakan `.env.example` hanya untuk nama variable dan contoh lokal non-production.
- Jangan mengubah logic ketika hanya diminta dokumentasi.
- Jangan menghapus perubahan existing di working tree. Saat file ini dibuat, availability module dan perubahan route availability belum committed.
- Route owner wajib Bearer token dan role `OWNER`.
- Public registration saat ini menolak role selain `CUSTOMER`, walaupun DTO mencantumkan opsi `OWNER`.
- Availability menggunakan timezone Asia/Jakarta dan slot default 1 jam.
- Court dianggap closed jika court atau venue tidak `ACTIVE`, operating hours tidak ada, atau day tersebut closed.
- Blocked slot memakai datetime dan memblokir slot yang overlap.
- Redis belum menjadi dependency runtime aplikasi walaupun container disediakan.
- Migration perlu dijalankan sebelum API dapat bekerja penuh terhadap database kosong.
