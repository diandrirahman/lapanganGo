# Hasil Laporan Code Review Proyek LapanganGo

Secara arsitektur, proyek ini sudah mengadopsi standar *Clean Architecture* (Handler -> Service -> Repository) yang rapi, modular, dan konsisten. Penggunaan *dependency injection* dan Go-Gin untuk *routing* juga terimplementasi dengan baik.

Namun, dari hasil *code review* mendalam terhadap *codebase* saat ini (`auth`, `owners`, `venues`, `middleware`, dan `main.go`), ditemukan beberapa temuan risiko isu, mulai dari yang kritikal hingga minor:

## 🚨 1. Risiko Kritikal (Keamanan): Celah Privilege Escalation
- **Lokasi Terkait:** `apps/api/internal/auth/dto.go` & `apps/api/internal/auth/service.go`
- **Penjelasan:** Pada saat proses registrasi publik, tipe DTO masih mengizinkan pengiriman nilai *role* berupa `OWNER` (`binding:"omitempty,oneof=CUSTOMER OWNER"`). Lebih berbahaya lagi, di level *service*, sama sekali tidak ada pemblokiran jika *role* yang di-request adalah `OWNER` (hanya ada pengecekan jika *role* kosong maka diisi `CUSTOMER`).
- **Dampak:** Siapa saja (bahkan pengguna iseng/hacker) bisa *by-pass* menjadi seorang `OWNER` dengan menyisipkan `{"role": "OWNER"}` di *payload* registrasi. Ini tidak aman untuk API publik.

## ⚠️ 2. Risiko Tinggi (Integrasi): Middleware CORS Tidak Ada
- **Lokasi Terkait:** `apps/api/cmd/api/main.go`
- **Penjelasan:** Server Gin dijalankan secara standar tanpa injeksi modul CORS.
- **Dampak:** Saat proyek ini mulai disambungkan dengan antarmuka web (seperti React.js, Next.js, atau Vue) di beda *domain* atau *port*, semua request akan otomatis ditolak (*blocked*) oleh fitur keamanan *browser*.

## ⚠️ 3. Risiko Menengah (Performa): N+1 Query pada List Venues
- **Lokasi Terkait:** `apps/api/internal/venues/service.go` (Method `GetPublicVenues` dan `ListVenues`)
- **Penjelasan:** Fitur pengambilan banyak *venue* menggunakan logika *looping* untuk mengambil detail *facilities* per satu venue: `s.repository.FindFacilitiesByVenueID`.
- **Dampak:** Jika terdapat limit 50 venue, sistem akan membuang waktu menembak kueri terpisah sebanyak 50 kali ke database. Ini akan menjadi sumber *bottleneck* performa di masa mendatang. Seharusnya menggunakan *bulk load* (contoh: `SELECT * WHERE venue_id IN (...)`).

## ⚠️ 4. Risiko Menengah (Infrastruktur): Tidak Ada Graceful Shutdown
- **Lokasi Terkait:** `apps/api/cmd/api/main.go`
- **Penjelasan:** Server aplikasi dijalankan secara pasif menggunakan fungsi sinkronus `r.Run()`.
- **Dampak:** Jika aplikasi terhenti, di-*restart*, atau di-*deploy* ulang, *server* akan diputus mendadak tanpa ada proses pengakhiran *pool* koneksi database yang rapi atau menunda penghentian *request* yang masih aktif (misalnya transaksi *booking*).

## ℹ️ 5. Risiko Rendah (Kualitas Data): Validasi Input Longgar
- **Lokasi Terkait:** `apps/api/internal/auth/dto.go`
- **Penjelasan:** 
  - `Phone`: Hanya mengandalkan batas *string* `max=30`. Validasi *regex* atau cek numerik hilang. 
  - `Password`: Hanya mengandalkan batas `min=8`. Validasi kekuatan *password* (kombinasi angka, huruf, dan spesial karakter) masih absen.
- **Dampak:** Kemungkinan masuknya barisan *data sampah* seperti nomor HP berupa huruf atau kata sandi yang terlalu lemah.

---
Silakan gunakan poin-poin *review* ini untuk dianalisa dan dibuatkan perencanaan perbaikannya selanjutnya!
