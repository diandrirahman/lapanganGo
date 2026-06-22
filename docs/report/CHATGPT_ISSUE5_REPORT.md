# Laporan Penyelesaian Issue 5: Validasi Input Phone & Password

**Status:** Selesai (Fixed)

## Penjelasan Masalah
Sebelumnya, sistem memiliki aturan validasi DTO yang sangat longgar untuk otentikasi pendaftaran publik:
- Nomor telepon (`Phone`) hanya dibatasi dengan panjang maksimal 30 karakter tanpa mengecek apakah isinya angka atau bukan.
- Kata sandi (`Password`) hanya dibatasi dengan `min=8`, artinya pengguna bisa mendaftar dengan kata sandi selemah `"password"` atau `"12345678"`. Hal ini rentan terhadap *brute force* dan *credential stuffing*.

## Tindakan Perbaikan

1. **Memperkuat Aturan DTO (`apps/api/internal/auth/dto.go`):**
   - Atribut `Phone` sekarang dilengkapi dengan binding tag `numeric` serta batas kewajaran `min=10,max=15`. Sistem seketika akan menolak input berupa huruf.

2. **Memperkuat Validasi di Service (`apps/api/internal/auth/service.go`):**
   - Dibuatkan fungsi internal khusus bernama `isPasswordStrong`.
   - Proses registrasi (*Register*) akan memanggil fungsi ini terlebih dahulu sebelum memanggil `bcrypt.GenerateFromPassword`.
   - Syarat kata sandi yang dikategorikan kuat: memiliki minimal 1 huruf kapital, 1 huruf kecil, 1 angka numerik, dan 1 simbol (spesial karakter).

3. **Memetakan Error ke Handler (`apps/api/internal/auth/handler.go`):**
   - Mendaftarkan konstanta ralat `ErrWeakPassword`.
   - Mengubah *switch case* pendaftaran agar melempar balikan HTTP `400 Bad Request` yang berisi instruksi ramah pengguna jika kata sandi mereka belum memenuhi kriteria kekuatan keamanan di atas.

Kualitas data kontak dan integritas kata sandi pengguna sekarang dipastikan terjaga sejak dari lapisan luar (API Input).

---
*(Laporan ini digunakan sebagai catatan log penyelesaian bug untuk AI Agent atau dokumentasi tim)*
