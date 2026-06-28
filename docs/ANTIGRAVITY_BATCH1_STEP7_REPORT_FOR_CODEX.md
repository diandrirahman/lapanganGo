# Report Antigravity - Batch 1A, Step 7 (Add Global 401 Handler)

**To:** Codex
**From:** Antigravity
**Task:** Step 7 - Add Global 401 Handler

## 1. File yang Diubah
- `apps/web/src/lib/api.ts`

## 2. Ringkasan Perubahan
- **Membuat custom wrapper `apiFetch`**: 
  - Membuat fungsi global `apiFetch` di dalam modul `api.ts` yang berfungsi membungkus `fetch` native.
  - Setiap kali ada _response_ yang mereturn `status === 401` (Unauthorized), wrapper akan mengecek apakah url saat ini ada di halaman `/login`. 
  - Jika user **belum** di halaman login, fungsi ini akan langsung membersihkan key `auth_token` dari `localStorage` dan melakukan redirect (_hard reload_) ke halaman `/login` dengan aman untuk mencegah _infinite redirect loop_.
- **Mengganti pilar koneksi API**: 
  - Sebanyak 33 _instances_ `await fetch(...)` di seluruh penjuru `api.ts` (baik endpoint untuk Auth, Bookings, Venues, OpenMatches, maupun Owner dashboard) secara masif namun akurat diganti menjadi `await apiFetch(...)`.
  - Hal ini menjamin bahwa perlindungan 401 sekarang **universal** tanpa harus menggunakan React Hooks (yang mana ilegal dilakukan di luar komponen React).

## 3. Cara Testing
1. **Automated Testing**:
   Menjalankan frontend build TypeScript checker dan Vite build.
   ```powershell
   cd apps/web
   npm run build
   ```
   **Hasil**: Kompilasi berhasil dan bersih dari error.

2. **Manual Verification**:
   - Jika _token_ yang tersimpan di _local storage_ sudah *expired* dan request apa pun menghasilkan *HTTP 401 Unauthorized*, pengguna tidak akan melihat aplikasi bengong (error fetch hening di console), melainkan akan langsung ditarik ke layar login dan sesinya dibersihkan.
   - Pengecekan non-401 (`!response.ok`) tetap mem-passing error ke pemanggil aslinya (berikut _message_ error) tanpa terganggu.

## 4. Risiko atau Catatan Lanjutan
- **Aman**: Pendekatan _intercepting_ langsung via `window.location.href` memastikan bahwa di mana pun modul API ini diinisiasi (_Redux thunk_, _React Query_, state biasa), sistem otentikasinya bertindak selayaknya _single source of truth_. 

---
Dengan ini seluruh tahapan di **Batch 1A** (Immediate Critical & Quick Fixes) telah rampung dengan sukses. Silakan direview secara utuh, dan kami siap lanjut ke pengarahan Batch berikutnya!
