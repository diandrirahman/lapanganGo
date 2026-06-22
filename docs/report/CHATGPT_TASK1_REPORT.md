# Laporan Penyelesaian Task 1: Endpoint Public Listing Venues

Berikut adalah ringkasan implementasi Task 1 yang baru saja diselesaikan. Silakan gunakan informasi ini sebagai konteks tambahan sebelum mengerjakan task selanjutnya.

## Daftar File yang Dimodifikasi

1. `apps/api/internal/venues/dto.go`
2. `apps/api/internal/venues/repository.go`
3. `apps/api/internal/venues/service.go`
4. `apps/api/internal/venues/handler.go`

## Ringkasan Perubahan

1. **DTO (`dto.go`)**
   - Menambahkan struct `ListPublicVenuesQuery` untuk menangkap query param `limit` dan `page`. Keduanya menggunakan binding validasi (misal: minimum 1).

2. **Repository (`repository.go`)**
   - Membuat fungsi `ListPublicVenues(ctx, limit, offset)` yang mengeksekusi SQL: `SELECT ... FROM venues WHERE status = 'ACTIVE' ORDER BY created_at DESC LIMIT $1 OFFSET $2`.

3. **Service (`service.go`)**
   - Membuat fungsi `GetPublicVenues`. Fungsi ini mengatur nilai default pagination (limit = 10, page = 1) jika parameter kosong.
   - Mengambil data *venue* dari repository dan melakukan *looping* untuk mengambil detail *facilities* agar data yang direturn konsisten dengan format `VenueResponse`.

4. **Handler (`handler.go`)**
   - Mendaftarkan rute publik `router.GET("/venues", h.GetPublicVenues)` di fungsi `RegisterRoutes` (di luar jangkauan *middleware auth* pemilik).
   - Mengembalikan data `venues`, `page`, dan `limit` dalam format JSON dengan gaya *snake_case*.

## Contoh Output JSON
Ketika memanggil endpoint `GET /venues?page=1&limit=10`, API akan memberikan respons dengan payload seperti ini:

```json
{
  "limit": 10,
  "page": 1,
  "venues": [
    {
      "id": "e58ed763-928c-4155-bee9-fdbaaadc15f3",
      "owner_profile_id": "c0448f72-9602-45e5-aa04-d5799a9a3b68",
      "name": "Arena Olahraga Makmur",
      "description": "Tempat olahraga strategis",
      "address": "Jl. Kemerdekaan No.10",
      "city": "Jakarta Selatan",
      "status": "ACTIVE",
      "facilities": [
        {
          "id": "71329a1d-85fa-4a65-99d9-df7f29bb2221",
          "name": "Toilet",
          "icon": "fa-toilet"
        }
      ],
      "created_at": "2026-06-21T09:10:00Z",
      "updated_at": "2026-06-21T09:10:00Z"
    }
  ]
}
```

**Status Laporan:** Kode telah lolos proses kompilasi (`go build ./...`) dan unit test (`go test ./...`) tanpa ada error. Tidak ada arsitektur utama yang dirubah.
