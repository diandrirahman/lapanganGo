# Review & Rencana Final Implementasi: Booking Flow API (MVP)

**Status Evaluasi:** `PASS`
Rencana implementasi aman untuk dilanjutkan. Struktur repositori sangat siap, dan perbaikan penamaan tabel (`court_operating_hours` dan `court_blocked_slots`) sudah digunakan.

## 1. Analisis Khusus: Race Condition & Anti Double-Booking

Pencegahan *Double Booking* sangat krusial karena kita memisahkan kolom `booking_date` (DATE), `start_time` (TIME), dan `end_time` (TIME).

**A. Pendekatan Migration 005 (Database EXCLUDE Constraint)**
- **Apakah butuh `btree_gist`?** Ya. Tabel PostgreSQL standar tidak bisa melakukan validasi rentang waktu secara *overlap* tanpa ekstensi ini.
- **Kecocokan Schema:** Karena PostgreSQL tidak memiliki tipe `timerange`, kita harus menggunakan casting data `tsrange` dengan menggabungkan `booking_date` dan waktu `TIME`.
- **Kekurangan:** Jika terjadi bentrok, database melempar *fatal constraint error*. Aplikasi harus menangkap *error* level DB ini untuk mengirim HTTP 409 Conflict. Implementasinya kaku.

**B. Pendekatan Row-Level Locking (Rekomendasi Utama untuk MVP)**
Untuk tahap MVP, **TIDAK PERLU** migration 005 dan `btree_gist`. Jauh lebih sederhana dan efisien menggunakan **Pessimistic Locking (`SELECT ... FOR UPDATE`)** pada transaksi *Service*.
- **Mekanisme:** Saat request masuk, mulai DB Transaction dan kunci baris data lapangannya: `SELECT id FROM courts WHERE id = $X FOR UPDATE`.
- **Keuntungan:** Request pemesanan kedua untuk lapangan yang sama di waktu yang sama akan dihentikan sesaat (mengantre/*wait*) secara elegan di sisi *Postgres* sampai transaksi pertama berhasil menyimpan *booking*. Tidak ada error "Collision", sistem secara otomatis memproses antrean. Sangat cocok dan aman.

**C. Kenapa Bukan SERIALIZABLE?**
Level `SERIALIZABLE` terlalu ketat dan kasar. Sering memicu *Serialization Failure* (Error 40001) sehingga aplikasi harus dipaksa membangun *retry-loop*. `FOR UPDATE` jauh lebih bersahabat untuk MVP.

**D. Query Overlap yang Benar (di dalam blok FOR UPDATE):**
Rumus overlap yang mencakup segala irisan waktu adalah `A.start < B.end AND A.end > B.start`.
```sql
SELECT COUNT(*) FROM bookings 
WHERE court_id = $1 
  AND booking_date = $2 
  AND start_time < $4 -- request.EndTime
  AND end_time > $3   -- request.StartTime
  AND status != 'CANCELLED';
```

## 2. File dan Modul yang Perlu Dibuat
Semua dikerjakan di `apps/api/internal/bookings/`:
1. `dto.go`: Berisi `CreateBookingRequest`, `BookingResponse`.
2. `repository.go`: Mengurus tabel `bookings`, `court_operating_hours`, `court_blocked_slots`, dan *locking* transaksi.
3. `service.go`: Pusat kontrol validasi dan perantara aliran data.
4. `handler.go`: Penerima request HTTP.
5. `main.go`: Registrasi *routing*.

## 3. Validasi Wajib (Business Logic)
1. **Otorisasi:** Token JWT *customer*. Role wajib `CUSTOMER`.
2. **Tanggal Valid:** `booking_date` $\ge$ hari ini. `start_time` $<$ `end_time`.
3. **Court & Venue ACTIVE:** Join ke tabel referensi.
4. **Operating Hours:** Pengecekan interval terhadap `court_operating_hours`.
5. **Blocked Slots:** Pengecekan overlap terhadap `court_blocked_slots`.
6. **Existing Booking:** Pengecekan overlap terhadap `bookings` (dengan mekanisme `FOR UPDATE`).
7. **Total Price Server-Side:** Harga = `courts.price_per_hour` $\times$ selisih jam durasi *booking*.

## 4. Test Case Wajib (`bookings_test.go`)
- `TestCreateBooking_Success` (Kasus Normal)
- `TestCreateBooking_Fail_PastDate` (Waktu Kadaluwarsa)
- `TestCreateBooking_Fail_OutsideOperatingHours` (Jam Tutup)
- `TestCreateBooking_Fail_OverlapExistingBooking` (Bentrok Jadwal)
- `TestCreateBooking_Fail_OverlapBlockedSlots` (Sedang Pemeliharaan)

Semua perancangan sudah solid, API Booking LapanganGo siap diketik!
