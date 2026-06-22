# Laporan Revisi Task 1: Endpoint Public Listing Venues

Berikut adalah ringkasan dari revisi kode yang baru saja ditambahkan untuk menyempurnakan Task 1. Silakan gunakan informasi ini untuk ChatGPT/Codex sebagai landasan pengerjaan Task selanjutnya.

## Ringkasan Perubahan

1. **DTO (`dto.go`)**
   - Menambahkan struktur balasan baru bernama `PublicVenueResponse`. Struktur ini identik dengan `VenueResponse`, namun membuang atribut `owner_profile_id` dan `status` agar informasi rahasia tidak bocor ke publik.
   - Mengubah maksimum limit pada parameter pagination dari yang tadinya 100 menjadi `max=50` (`binding:"omitempty,min=1,max=50"`).

2. **Service (`service.go`)**
   - Menambahkan fungsi pembantu (helper) `toPublicVenueResponse` untuk memetakan objek dari database ke DTO `PublicVenueResponse`.
   - Menyesuaikan tipe *return* dari fungsi `GetPublicVenues` yang sebelumnya mengembalikan `[]VenueResponse` sekarang menjadi `[]PublicVenueResponse`.

## Contoh Respons JSON Setelah Revisi
Endpoint `GET /venues?page=1&limit=10` sekarang lebih bersih dari atribut internal:

```json
{
  "limit": 10,
  "page": 1,
  "venues": [
    {
      "id": "e58ed763-928c-4155-bee9-fdbaaadc15f3",
      "name": "Arena Olahraga Makmur",
      "description": "Tempat olahraga strategis",
      "address": "Jl. Kemerdekaan No.10",
      "city": "Jakarta Selatan",
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

**Status:** Kode telah di-*build* dan lolos pengecekan `go test ./...`. Siap untuk lanjut ke Task 2.
