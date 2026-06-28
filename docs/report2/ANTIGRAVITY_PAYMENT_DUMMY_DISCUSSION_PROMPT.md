# AntiGravity Discussion Prompt: Dummy Payment / Confirm Booking Flow

```text
Kamu bertindak sebagai Product-minded Senior Backend Engineer untuk project LapangGo.

Konteks status backend saat ini:
- Auth JWT sudah ada.
- Customer bisa membuat booking.
- Booking create flow sudah punya anti double-booking berbasis row-level locking.
- Availability sudah sinkron dengan booking aktif:
  - `AVAILABLE`
  - `BLOCKED`
  - `BOOKED`
- Booking `CANCELLED` tidak memblokir availability.
- Customer bisa cancel booking miliknya sendiri, tetapi hanya jika status masih `PENDING_PAYMENT`.
- Cancellation sudah atomic:
  - update dibatasi `id + customer_id + status = 'PENDING_PAYMENT'`
  - race fallback/refetch sudah ada.
- Owner sudah bisa melihat booking venue miliknya lewat:
  - `GET /owner/venues/:id/bookings`

Masalah produk berikutnya:
Booking masih berhenti di status `PENDING_PAYMENT`.
Belum ada cara untuk customer menandai booking sebagai sudah dibayar / confirmed.

Tujuan diskusi:
Tolong evaluasi next feature berikut sebelum implementasi:
Dummy Payment / Confirm Booking Flow.

Usulan endpoint:
`POST /bookings/:id/pay`

Pertanyaan desain yang perlu kamu jawab:

1. Status akhir sebaiknya apa untuk MVP?
   Pilihan:
   - `PAID`
   - `CONFIRMED`
   - atau transisi `PENDING_PAYMENT -> PAID -> CONFIRMED`

   Rekomendasi awal Codex:
   Untuk MVP dummy payment, gunakan satu status akhir saja: `CONFIRMED`.
   Alasannya: belum ada payment gateway atau payment settlement, sehingga yang paling penting adalah booking dianggap valid oleh owner.

2. Apakah endpoint ini perlu tabel payment baru?
   Rekomendasi awal Codex:
   Tidak perlu untuk MVP.
   Cukup update kolom `bookings.status`.
   Payment table baru ditunda sampai integrasi payment sungguhan.

3. Business rules apa yang aman?
   Rekomendasi awal:
   - Wajib Bearer token.
   - Wajib role `CUSTOMER`.
   - Customer hanya bisa pay booking miliknya sendiri.
   - Hanya status `PENDING_PAYMENT` yang bisa diproses.
   - Jika status `CANCELLED`, return `409 Conflict`.
   - Jika status `PAID` atau `CONFIRMED`, return `409 Conflict`.
   - Update harus atomic:
     `WHERE id = $1 AND customer_id = $2 AND status = 'PENDING_PAYMENT'`
   - Jika atomic update gagal karena race, lakukan refetch dan map error dengan jelas, mirip cancellation flow.

4. Endpoint response sebaiknya seperti apa?
   Usulan:
   - HTTP 200
   - JSON:
     ```json
     {
       "message": "Booking payment confirmed successfully",
       "booking": { ... }
     }
     ```

5. Apa dampak ke availability?
   Ekspektasi:
   Booking yang berubah dari `PENDING_PAYMENT` ke `CONFIRMED` tetap memblokir slot karena availability mengecualikan hanya `CANCELLED`.

6. Test apa saja yang wajib?
   Minimal:
   - success: `PENDING_PAYMENT -> CONFIRMED`
   - fail not found / not owned
   - fail already cancelled
   - fail already paid/confirmed
   - race: status berubah saat pay, atomic update return `pgx.ErrNoRows`, refetch status `CANCELLED` atau `CONFIRMED`, service return conflict error yang tepat

7. Apakah ada risiko produk?
   Tolong jelaskan:
   - Karena ini dummy payment, tidak ada validasi uang nyata.
   - Endpoint ini hanya simulasi untuk menutup MVP flow.
   - Jangan klaim sebagai payment real.

Batasan diskusi:
- Jangan implementasi kode dulu.
- Jangan ubah file source.
- Jangan tambah migration.
- Jangan refactor.
- Buat laporan/rekomendasi desain saja.

Output yang diminta:
Buat report di `docs/CHATGPT_PAYMENT_DUMMY_DESIGN_REVIEW.md` berisi:
1. Rekomendasi status final: `PAID` atau `CONFIRMED`
2. Endpoint final
3. Business rules final
4. Repository/service/handler approach
5. Error mapping HTTP
6. Test plan
7. Risiko dan batasan MVP
8. Apakah fitur ini siap diimplementasikan setelah approval Codex
```
