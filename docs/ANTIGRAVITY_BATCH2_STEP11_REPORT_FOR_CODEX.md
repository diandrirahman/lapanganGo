# Report Antigravity - Batch 2, Step 11 (Add Rate Limiting)

**To:** Codex
**From:** Antigravity
**Task:** Step 11 - Add Rate Limiting

## 1. File yang Diubah / Dibuat
- `apps/api/internal/middleware/rate_limiter.go` (File Baru)
- `apps/api/internal/middleware/rate_limiter_test.go` (File Baru)
- `apps/api/internal/auth/handler.go`
- `apps/api/cmd/api/main.go`

## 2. Ringkasan Implementasi
- **In-Memory Rate Limiter**: 
  - Membuat _struct_ `RateLimiter` di dalam *package* `middleware` yang sepenuhnya mengandalkan _in-memory thread-safe map_ menggunakan `sync.Mutex` serta konsep _sliding/fixed window_ sederhana berbasis pergerakan waktu.
  - Rate limiter membersihkan rekam jejak secara otomatis di *background goroutine* (`cleanup()`) di dalam rentang waktu yang sesuai (tidak membocorkan _memory_, terhapus _by timeout_).
  - Ketika batasan request per rentang waktu terlampaui, request akan ditolak dengan JSON *response HTTP 429 Too Many Requests*. Pesan *error* dibuat tidak mengekspos _internal logic_.
- **Dual Policy Limits (Auth vs General)**:
  - Sesuai dengan batasan MVP, saya mendeklarasikan dua instansi limiter yang berbeda di `apps/api/cmd/api/main.go`.
  - **General Endpoints**: Seluruh rute secara global di-_inject_ rate limiter berkapasitas **100 req/menit**.
  - **Auth Endpoints**: Hanya pada rute `POST /auth/login` dan `POST /auth/register` (pada `auth/handler.go`), saya menimpali filter _strict_ limiter sebesar **10 req/menit**.
- **Unit Testing**:
  - Menyusun `TestRateLimiter` lengkap di _package_ `middleware` yang menyimulasikan tembakan permintaan HTTP (_HTTP Recorder_) dari _IP Address_ yang sama untuk mendemonstrasikan kelulusan _request_ beruntun hingga akhirnya di-blok dengan status kode *HTTP 429*, lalu diterima kembali saat jendela waktunya (*window*) sudah selesai. 

## 3. Cara Testing
1. **Automated Testing**:
   Jalankan Go test seluruh package untuk memastikan _dependency_ middleware tak merusak fungsi _endpoint_ lain.
   ```powershell
   cd apps/api
   go test ./internal/middleware -v
   go test ./...
   ```
   **Hasil**: Ujian `TestRateLimiter` berstatus **PASS** dalam _suite_ `middleware`. *Build root level* `go test ./...` juga bersih dan sukses tanpa eror (**ok**).

2. **Manual Verification**:
   - Jika endpoint publik seperti `/health` dihajar 100 kali dalam 60 detik dari IP yang sama, tembakan ke-101 akan mendapati HTTP 429.
   - Endpoint sensitif `/auth/login` akan langsung memblokir percobaan paksa *bruteforce* ke-11 jika dikirimkan di bawah rentang waktu yang sama (1 menit).

## 4. Risiko atau Catatan Lanjutan
- **Skalabilitas MVP**: Limitasi ini *thread-safe* untuk di _single-instance_ dan super-ringan karena berjalan di memori lokal RAM. Perlu dipahami bahwa jika ke depannya Anda men-_deploy_ LapanganGo di _multi-container load balancer_, in-memory map per-instance bisa menjadi tidak akurat; sehingga pemindahan ke _Redis_ nantinya tetap akan diperlukan (bisa menggunakan *abstraction interface* pada _struct_ yang sama). Namun untuk skenario _MVP_, perlindungan Rate Limiting ini sudah sangat matang dan siap tayang.

---
Tugas awal di Batch 2 telah selesai! Implementasi pencegahan DDOS/bruteforce dasar sudah berhasil diaplikasikan. Saya akan menantikan pengarahan selanjutnya untuk Step 12.
