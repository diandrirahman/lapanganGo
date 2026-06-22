# Laporan Penyelesaian Task 3: Initial Database Migration untuk Bookings

Berikut adalah ringkasan skema migrasi tabel *bookings* yang telah selesai dibuat. Tidak ada perubahan pada kode Golang; fokus sepenuhnya hanya pada persiapan fondasi di level *database*. Silakan jadikan laporan ini sebagai basis referensi jika kamu akan merancang *flow API* reservasi berikutnya.

## File yang Dibuat
1. `db/migrations/004_bookings.sql`

## Struktur SQL Tabel & Relasi

File SQL tersebut mendefinisikan tabel `bookings` beserta serangkaian *constraints* dan *indexes* sebagai berikut:

```sql
CREATE TABLE IF NOT EXISTS bookings (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  customer_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  court_id UUID NOT NULL REFERENCES courts(id) ON DELETE RESTRICT,
  booking_date DATE NOT NULL,
  start_time TIME NOT NULL,
  end_time TIME NOT NULL,
  total_price NUMERIC(12, 2) NOT NULL,
  status VARCHAR(30) NOT NULL DEFAULT 'PENDING_PAYMENT',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT bookings_time_valid CHECK (start_time < end_time),
  CONSTRAINT bookings_total_price_non_negative CHECK (total_price >= 0)
);

CREATE INDEX IF NOT EXISTS idx_bookings_customer_id ON bookings(customer_id);
CREATE INDEX IF NOT EXISTS idx_bookings_court_id ON bookings(court_id);
CREATE INDEX IF NOT EXISTS idx_bookings_booking_date ON bookings(booking_date);
CREATE INDEX IF NOT EXISTS idx_bookings_court_date ON bookings(court_id, booking_date);
```

## Poin-Poin Penting untuk Diperhatikan

1. **Relasi (Foreign Keys):** 
   - Merujuk pada tabel `users` (sebagai `customer_id`) dan `courts` (sebagai `court_id`).
   - Keduanya menggunakan `ON DELETE RESTRICT` agar data historis pemesanan tetap aman dan tidak ikut terhapus walau pelanggan menonaktifkan akun atau lapangan ditiadakan.
2. **Standardisasi Tipe Data:**
   - Kolom-kolom waktu dipisahkan spesifik (*booking_date: DATE*, *start_time: TIME*, *end_time: TIME*) agar selaras dengan tabel ketersediaan (*court_operating_hours*).
   - *Timestamp audit* (created_at, updated_at) konsisten menggunakan `TIMESTAMPTZ`.
3. **Database-Level Constraint:**
   - `bookings_time_valid` memastikan nilai `start_time` tidak pernah melampaui `end_time`.
   - `bookings_total_price_non_negative` menolak input harga pemesanan bernilai negatif (`total_price >= 0`).
4. **Optimasi Pencarian (Indexing):**
   - Indeks disiapkan pada titik temu vital aplikasi di masa depan: `customer_id` (untuk fitur riwayat pemesanan pelanggan) dan pencarian ganda `court_id + booking_date` (untuk cek persilangan/tumpang tindih jadwal secara sangat cepat).
5. **Status Tipe:**
   - Menggunakan `VARCHAR(30)` dengan default value `PENDING_PAYMENT` ketimbang `ENUM` agar lebih dinamis jika kelak ada status tambahan tanpa perlu mendefinisikan ulang objek *enum* PostgreSQL.

**Status Laporan:** Fondasi DDL aman. Backend API (`go build` / `go test`) dipastikan tidak ada yang regresi/rusak usai file migrasi ditambahkan.
