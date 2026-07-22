# LapangGo v1.7 Known Limitations

Dokumen ini mencatat batasan rilis `version_1.7`. Platform Finance pada rilis ini adalah diagnostics dan proyeksi operasional dalam `MODE SIMULASI`; dokumen ini bukan laporan pajak dan bukan persetujuan aktivasi LIVE.

## 1. Pembayaran

- Payment gateway asli belum tersedia.
- Pembayaran masih memakai alur manual: customer transfer di luar sistem, lalu mengunggah bukti pembayaran.
- Owner memverifikasi bukti pembayaran secara manual.
- Sistem belum mendukung webhook bank/payment gateway.
- Batas pembayaran saat ini diset `10 menit` untuk MVP/QA. Nilai ini dapat dinaikkan lewat konfigurasi jika dibutuhkan untuk operasional nyata.

## 2. Refund

- Refund approval belum mentransfer uang otomatis ke customer.
- Saat owner menyetujui refund, sistem:
  - mengubah booking menjadi `CANCELLED`,
  - mencatat ledger `EXPENSE / REFUND`,
  - mempertahankan ledger income awal.
- Proses pengembalian uang tetap dilakukan owner di luar sistem sesuai kebijakan venue.

## 3. Keuangan & Payout

- Payout/penarikan dana owner dari platform belum tersedia.
- Untuk MVP, uang pembayaran diasumsikan masuk langsung ke rekening owner.
- Dashboard Superadmin untuk analytics Platform Finance, snapshot commercial term, projected commission, reconciliation read-only, dan pencatatan OPEX/journal internal sudah tersedia dalam mode simulasi.
- Payment gateway, actual commission collection, customer service fee, owner payable, settlement, payout, dan pemotongan saldo owner belum tersedia.
- Angka projected commission dan projected operating result bukan kas aktual atau laporan pajak.
- Export laporan PDF/Excel belum tersedia.
- Manual finance transaction hanya mencatat kas, tidak membuat booking dan tidak memblokir jadwal.

## 4. Notifikasi

- In-app notification sudah tersedia untuk event utama booking, payment, refund, payment reminder, dan completed booking.
- Email notification belum tersedia.
- WhatsApp notification belum tersedia.
- Push notification realtime/WebSocket belum tersedia; notifikasi diambil melalui API saat user membuka aplikasi/dropdown.

## 5. Owner Offline / Walk-in Booking

- Owner offline booking sudah tersedia dan membuat booking sungguhan.
- Offline booking memblokir availability dan mencatat ledger `INCOME / BOOKING`.
- Offline price override sudah tersedia dengan audit harga sistem, harga final, dan alasan perubahan.
- Receipt/struk cetak dan invoice PDF untuk walk-in belum tersedia.
- Deposit/down payment offline belum tersedia.

## 6. Promo

- Promo sudah mendukung validasi berdasarkan tanggal main booking.
- Promo sudah menyimpan snapshot harga booking: harga awal, diskon, harga final, promo id, dan promo code.
- Promo yang sudah pernah dipakai booking tidak boleh hard delete; owner harus menonaktifkan promo.
- Fitur promo lanjutan belum tersedia:
  - kuota promo,
  - batas pemakaian per customer,
  - minimum transaksi,
  - auto-apply promo,
  - stacking beberapa promo,
  - campaign budget.

## 7. Staff Roles

- Staff Roles belum tersedia.
- Semua akses owner masih memakai akun owner utama.
- Multi-user operasional venue, kasir, dan permission matrix akan menjadi fase terpisah.

## 8. Mabar / Open Match

- Mabar tersedia sebagai fitur sosial dasar.
- Split bill/patungan otomatis belum tersedia.
- Pembayaran antar peserta Mabar masih dilakukan di luar sistem.

## 9. Admin Platform

- Superadmin dashboard dan Platform Finance diagnostics tersedia dengan feature flag default `false` serta role `SUPER_ADMIN`.
- Approval venue oleh admin platform belum menjadi flow lengkap untuk produksi.
- Moderasi konten, laporan penyalahgunaan, dan audit admin platform belum tersedia.

## 10. Scheduler & Operasional

- Auto-complete scheduler sudah tersedia untuk mengubah booking `PAID` yang sudah lewat menjadi `COMPLETED`.
- Jika scheduler/worker tidak berjalan, owner masih dapat menyelesaikan booking secara manual.
- Sistem belum memiliki dashboard monitoring worker.

## 11. Technical Notes

- Warning Vite terkait chunk size masih non-blocking.
- Belum ada optimasi code-splitting frontend untuk bundle besar.
- Untuk QA v1.7, gunakan disposable database dan jalankan migration sampai `024`; jangan menjalankan destructive down migration pada shared database yang sudah memiliki finance facts.
- Hindari menjalankan data backfill massal tanpa backup database.
