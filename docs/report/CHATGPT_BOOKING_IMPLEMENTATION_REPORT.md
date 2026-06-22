# Laporan Implementasi: Booking Flow API (MVP)

Seluruh rencana perancangan Booking API dan mekanisme perlindungan *Anti Double-Booking* kini telah diimplementasikan seutuhnya di repositori utama. Berikut adalah rincian pengerjaan untuk Anda tinjau:

## 1. Direktori & Modul Baru (`apps/api/internal/bookings`)
Telah dibuat 4 file terstruktur untuk menangani logika pemesanan:
- `dto.go`: Membawa validasi *request* DTO dengan tag bawaan standar seperti pengecekan spesifik `datetime=2006-01-02` (tanggal) dan `15:04` (jam).
- `repository.go`: Mengurus kueri persisten dan penguncian *Database*.
- `service.go`: Menangani validasi bisnis (Business Rules).
- `handler.go`: Menangani parameter *Request/Response* HTTP.

Selain itu, modul `bookings` ini juga sudah didaftarkan pada _router_ inti di `apps/api/cmd/api/main.go` lengkap dengan injeksi middleware (Otorisasi JWT + Role Customer).

## 2. Realisasi *Anti Double-Booking* (Row-Level Locking)
Sebagaimana disepakati, **tidak ada Migration 005** yang ditambahkan. Seluruh keandalan transaksi dipusatkan menggunakan `SELECT ... FOR UPDATE` pada tingkat repositori (Pessimistic Locking).

Alur transaksi saat ada yang memesan:
1. Menjalankan *Database Transaction* (`BeginTx`).
2. Mengamankan aset lapangan: `SELECT id FROM courts WHERE id = $1 FOR UPDATE`. Ini menjamin *request* bersamaan dari pengguna lain di lapangan yang sama akan masuk dalam status "menunggu", alih-alih saling tabrak.
3. Mencocokkan dengan `court_blocked_slots`. Jika sedang *maintenance*, _rollback_.
4. Mencocokkan dengan tabel `bookings`. Jika terdeteksi jadwal saling silang (`start_time` $<$ *end* dan `end_time` $>$ *start*) dengan `status != 'CANCELLED'`, _rollback_.
5. Melakukan eksekusi `INSERT` DTO secara aman.

## 3. Validasi *Business Logic* Ekstensif
Keseluruhan daftar validasi wajib telah diterapkan secara berurutan di `service.go`:
- **Auth**: Dibatasi hanya untuk profil *CUSTOMER*.
- **Waktu Lintas Batas**: Menolak tanggal di masa lalu dan menolak `start_time` $\ge$ `end_time`.
- **Status Non-Aktif**: Menolak entitas `courts` dan `venues` yang berstatus selain `ACTIVE`.
- **Luar Jam Operasional**: Indeks hari yang dipinta otomatis dikalkulasikan (Mulai dari indeks 0 = Minggu). Pemesanan ditolak jika lapangan hari itu tercatat `is_closed` atau jadwal yang diminta ada di luar jangkauan `open_time` dan `close_time`.
- **Hitungan Harga Mutlak**: Client hanya mengirim ID, Tanggal, dan Jam. Total harga dikalkulasikan murni secara desimal di sisi *server* berdasarkan `courts.price_per_hour` untuk memutus manipulasi/fraud di sisi klien.

## 4. Pelacakan Waktu yang Presisi (Timezone Conversion)
Oleh karena tabel `court_blocked_slots` mengharuskan pencocokan terhadap tipe nilai `TIMESTAMPTZ` utuh, maka `booking_date` (Tanggal) serta `start_time` / `end_time` (Jam) yang diminta oleh pengguna otomatis digabungkan (*merged*) dan dikonversi menggunakan presisi zona waktu riil (`Asia/Jakarta` atau `WIB`) melalui kode `time.Date(..., loc)` sebelum dilemparkan sebagai parameter pengecekan ke dalam database.

## 5. Pengujian Unit (*Unit Test*) Otomatis
Pengecekan logika secara independen difasilitasi oleh `BookingRepository` interface (mocking repo). Ini memungkinkan fungsi `service_test.go` berjalan efisien tanpa perlu menyalakan *database* PostgreSQL tambahan.

Hasil eksekusi `go test ./...`:
```text
ok      lapangango-api/internal/bookings        2.453s
```

7 Test Case Wajib Berhasil *PASS*:
- `TestCreateBooking_Success`
- `TestCreateBooking_Fail_PastDate`
- `TestCreateBooking_Fail_InvalidTimeRange`
- `TestCreateBooking_Fail_InactiveCourtOrVenue`
- `TestCreateBooking_Fail_OutsideOperatingHours`
- `TestCreateBooking_Fail_OverlapBlockedSlots`
- `TestCreateBooking_Fail_OverlapExistingBooking`

## Kesimpulan:
Seluruh file berada pada status untracked (belum dicommit). API berjalan stabil (`go build ./...` sukses) tanpa gangguan ke *module* yang lain.

Mohon berikan verifikasi atas laporan ini, apakah sudah siap untuk tahapan selanjutnya?
