# Laporan Penyelesaian Task 2: Endpoint Public Detail Venue & Court

Berikut adalah ringkasan implementasi Task 2 yang baru saja diselesaikan. Silakan gunakan informasi ini untuk dianalisis sebagai referensi pengembangan atau audit (*Code Review*).

## Daftar File yang Dimodifikasi

Semua perubahan berada dalam satu domain (*venues*) untuk menjaga konsistensi dan alur dari API publik:
1. `apps/api/internal/venues/dto.go`
2. `apps/api/internal/venues/repository.go`
3. `apps/api/internal/venues/service.go`
4. `apps/api/internal/venues/handler.go`

## Ringkasan Perubahan

1. **DTO (`dto.go`)**
   - Menambahkan struktur balasan baru `PublicSportResponse` dan `PublicCourtResponse`.
   - Menambahkan struktur utama `PublicVenueDetailResponse` yang melakukan *embedding* dari `PublicVenueResponse` ditambah atribut `Courts []PublicCourtResponse`. Format ini memastikan tidak ada data internal (seperti `owner_profile_id` atau `status`) yang terekspos.

2. **Repository (`repository.go`)**
   - Menambahkan *method* `FindPublicVenueByID` untuk menjalankan *single query* (via `SELECT ... WHERE id = $1 AND status = 'ACTIVE' LIMIT 1`).
   - Menambahkan *method* `FindActiveCourtsByVenueID` untuk menjalankan *single query* yang menggabungkan (`JOIN`) tabel `courts` dan `sports`, dengan parameter klausul `WHERE c.venue_id = $1 AND c.status = 'ACTIVE'`.
   - Pola ini memastikan **bebas dari masalah N+1 queries**; kita cukup mengeksekusi tepat 2 kueri: satu untuk Venue, satu untuk Courts-nya.

3. **Service (`service.go`)**
   - Menambahkan *method* `GetPublicVenue` untuk memanggil repository: (1) Ambil detail venue, (2) Ambil fasilitas, (3) Ambil courts.
   - Menggunakan *mapper* `toPublicCourtResponses` untuk melakukan pemetaan data SQL murni ke bentuk struktur DTO publik.
   - *Error handling* disertakan di mana `pgx.ErrNoRows` (ditangkap sebagai `IsNotFound`) akan diteruskan sebagai status HTTP `404 Not Found`.

4. **Handler (`handler.go`)**
   - Mendaftarkan *route* baru `router.GET("/venues/:id", h.GetPublicVenue)` ke dalam fungsi pendaftaran rute di level root/publik, sepenuhnya lepas dari *middleware authentication*.

## Penanganan Edge Cases (Behavior Khusus)

- **Venue ACTIVE:** Mengembalikan kode `200 OK` dengan payload detail beserta daftar *courts* (berstatus *ACTIVE*).
- **Venue tidak ditemukan:** Mengembalikan kode `404 Not Found`.
- **Venue INACTIVE / DRAFT:** Mengembalikan kode `404 Not Found` (karena filter query ketat hanya di `ACTIVE`).
- **Venue ACTIVE tapi semua courts INACTIVE (atau tidak punya):** Mengembalikan kode `200 OK` dengan properti `courts: []` (array kosong).

## Contoh Respons JSON
Saat melakukan hit `GET /venues/:id` untuk venue aktif, format datanya adalah:

```json
{
  "id": "e58ed763-928c-4155-bee9-fdbaaadc15f3",
  "name": "Arena Olahraga Makmur",
  "description": "Tempat olahraga lengkap dan nyaman di selatan kota.",
  "address": "Jl. Kemerdekaan No.10",
  "district": "Cilandak",
  "city": "Jakarta Selatan",
  "province": "DKI Jakarta",
  "postal_code": "12430",
  "latitude": -6.2941,
  "longitude": 106.8042,
  "facilities": [
    {
      "id": "71329a1d-85fa-4a65-99d9-df7f29bb2221",
      "name": "Parkir Luas",
      "icon": "fa-parking"
    }
  ],
  "created_at": "2026-06-21T09:10:00Z",
  "updated_at": "2026-06-21T09:10:00Z",
  "courts": [
    {
      "id": "f5f1df83-1234-5678-abcd-1234567890ab",
      "sport": {
        "id": "s1f1df83-0000-0000-abcd-1234567890ab",
        "name": "Badminton"
      },
      "name": "Lapangan 1",
      "description": "Lapangan dengan karpet standar",
      "location_type": "INDOOR",
      "surface_type": "Karet",
      "price_per_hour": 75000,
      "created_at": "2026-06-21T09:15:00Z",
      "updated_at": "2026-06-21T09:15:00Z"
    }
  ]
}
```

**Status Kelulusan:** Kode ini berhasil di-*build* (`go build ./...`) dan telah melewati seluruh verifikasi *tests* (`go test ./...`) tanpa gagal. Kualitas arsitektur dipertahankan seperti pedoman awal.
