# Laporan Eksekusi Demo Seed Big

Skrip `demo-seed` skala besar telah diimplementasikan untuk menyediakan data demonstrasi yang kaya bagi aplikasi LapangGo. Skrip ini dibangun dengan memperhatikan idempotensi agar tidak mengacaukan struktur data saat dijalankan berkali-kali.

## Cara Menjalankan

Buka terminal (*Command Prompt* / PowerShell) dan arahkan ke direktori backend Anda, lalu jalankan:

```bash
cd apps/api
go run ./cmd/demo-seed
```

> **Catatan Windows:** Jika komputer Anda memblokir `go run` karena *Application Control/Device Guard policy* pada *Temporary Folder*, maka lakukan *build* terlebih dahulu dan jalankan dari folder *project*:
> ```bash
> go build -o demo-seed.exe ./cmd/demo-seed
> .\demo-seed.exe
> ```
> *(Bila file .exe masih diblokir, hubungi administrator IT Anda atau jalankan via elevated Command Prompt sesuai instruksi internal project).*

## Mode Pembersihan (Cleanup)

Untuk mereset atau membersihkan seluruh data *Demo* tanpa membuat data baru, gunakan perintah:

```bash
go run ./cmd/demo-seed --cleanup
```
Skrip akan menghapus semua partisipan, pertandingan, booking, lapangan, *venue*, *owner profile*, dan *user* yang memiliki *email* berawalan `demo.` (proses dilakukan dengan sistem *cascading manual*).

## Rincian Data yang Dibuat

Secara acak dengan *seed* deterministik, skrip akan menyuntikkan data ke *database*:

1. **Olahraga (Sports):** 6 olahraga utama (Futsal, Badminton, Mini Soccer, Basket, Tenis, Voli).
2. **Akun Pengguna (Users):** 32 pengguna.
   - 2 Owners (`demo.owner01@lapangango.test` dst).
   - 10 Hosts (`demo.host01...`).
   - 20 Participants (`demo.customer01...`).
3. **Owner Profiles:** 2 Bisnis.
4. **Venues:** 11 *Venue* olahraga premium (tersebar di Jakarta, Tangerang, Depok, Bekasi).
5. **Courts:** 2-5 lapangan untuk tiap venue (~30+ total lapangan).
6. **Operating Hours:** Rutinitas jam operasional 08:00-22:00 (dan 07:00-23:00 untuk akhir pekan).
7. **Blocked Slots:** ~15-25 jadwal tertutup acak (*maintenance, private event*).
8. **Bookings:** ~60-100 pemesanan lapangan bervariasi antara `CONFIRMED`, `PENDING_PAYMENT`, dan `CANCELLED`.
9. **Open Matches (Mabar):** ~20-30 pertandingan terbuka berdasarkan *booking* berstatus *Confirmed*.
10. **Match Participants:** ~60-120 data peserta yang bergabung ke ajang Mabar (dijamin tanpa duplikat).

## Contoh Output Token

Saat skrip berjalan dengan sukses, token JWT berikut akan dicetak secara langsung di konsol (token aslinya akan jauh lebih panjang dan tidak disimpan di file sumber apa pun untuk alasan keamanan):

```text
--- DEMO TOKENS ---
DEMO_OWNER_TOKEN=eyJhbGciOiJIUzI1NiIsInR5c...
DEMO_HOST_TOKEN=eyJhbGciOiJIUzI1NiIsInR5c...
DEMO_CUSTOMER_TOKEN=eyJhbGciOiJIUzI1NiIsInR...
-------------------
```

## Menjalankan Frontend dengan Demo Data

Untuk membuat UI Front-End Anda memakai koneksi data yang baru saja dibentuk ini, sesuaikan pengaturan pada berkas `.env` dari direktori `apps/web`:

1. Buka `apps/web/.env`.
2. Pastikan variabel berikut dikonfigurasi:
   ```env
   VITE_API_BASE_URL=http://localhost:8080
   VITE_USE_MOCK_MABAR=false
   ```
3. Jalankan `npm run dev` pada `apps/web`.

Kini antarmuka web Anda akan menarik data nyata dari *database PostgreSQL* yang sangat bervariasi!
