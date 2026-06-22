# Laporan Penyelesaian Issue 1: Fix Role OWNER Self-Register

**Status:** Selesai (Fixed)

## Penjelasan Masalah
Sebelumnya, sistem publik mengizinkan siapa saja yang mendaftar (Register) melalui *endpoint* publik untuk mendapatkan akses sebagai `OWNER` hanya dengan menambahkan atribut `"role": "OWNER"` di JSON *payload* (karena didukung oleh aturan validasi `binding:"omitempty,oneof=CUSTOMER OWNER"` di `RegisterRequest` DTO).

Lebih berbahaya lagi, tidak ada *filter* di tingkat `Service` untuk memblokir permintaan jika *role* yang dimasukkan adalah selain `CUSTOMER`.

## Tindakan Perbaikan

1. **Memperketat DTO (`apps/api/internal/auth/dto.go`):**
   - Aturan `binding` untuk atribut `Role` diubah menjadi `binding:"omitempty,oneof=CUSTOMER"`.
   - Ini berarti secara *framework*, Gin akan langsung me-reject request (400 Bad Request) jika klien secara iseng mengisikan *role* lain seperti `OWNER` maupun `SUPER_ADMIN`.

2. **Memaksa Hardcode di Service (`apps/api/internal/auth/service.go`):**
   - Untuk menghindari kemungkinan data lolos dari DTO dan mengotori database, logika penentuan peran (*role*) di *service layer* diubah menjadi paksa mutlak (`role := "CUSTOMER"`).
   - Seluruh pengguna yang mendaftar via *public endpoint* kini otomatis dan pasti akan berstatus `CUSTOMER`.

Dengan perbaikan ini, jalur masuk utama publik sudah aman dari ancaman eskalasi akses (*Privilege Escalation*).

---
*(Laporan ini digunakan sebagai catatan log penyelesaian bug untuk AI Agent atau dokumentasi tim)*
