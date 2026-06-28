# Laporan Perbaikan Final Review MVP Mabar

Halo Codex & Tim! Sesuai dengan instruksi final dari dokumen ulasan, saya telah merampungkan iterasi perbaikan terakhir agar modul *Open Match* (Mabar) ini 100% matang untuk fase integrasi.

Berikut laporan dari seluruh perbaikan yang telah diterapkan:

## 1. Ringkasan Perubahan Teknis
- **Sistem Pembatasan Booking (P2):** Seluruh logika *List* dan *Join* kini dilarang beroperasi jika *booking* tidak berstatus murni `CONFIRMED`.
- **Integritas UI Payload (P2):** Validasi ketat pada *Payload* di titik masuk (`handler` & `dto`), memastikan variabel krusial seperti `Title` terhindar dari isian kosong atau hanya mengandung spasi.
- **Konsistensi Zona Waktu (P3):** Proses perhitungan batas kedaluwarsa pertandingan kini terkunci di zona waktu `Asia/Jakarta` tanpa terpengaruh oleh konfigurasi server.
- **Unit Test Perluasan (P3):** Peningkatan *coverage test* pada cabang validasi baru dan *unique constraint violation*.

## 2. Keputusan Error Domain untuk Booking Non-Confirmed
Sebelumnya sistem hanya menolak status `CANCELLED`. Kini saya memperkenalkan satu *domain error* spesifik baru:
```go
ErrBookingNotConfirmed = errors.New("booking for this open match is not confirmed")
```
Jika ada partisipan mencoba `JoinOpenMatch` pada lapangan yang *booking*-nya masih *PENDING* atau telah kedaluwarsa, servis akan melemparkan `ErrBookingNotConfirmed`.
Kesalahan ini secara otomatis akan dipetakan oleh *handler* (`apps/api/internal/mabar/handler.go`) untuk membalas respons HTTP **`409 Conflict`** (mengingat status ini bersifat konflik internal sumber daya, sama halnya dengan *Booking Cancelled*).

## 3. Validasi Payload yang Ditambahkan
Di level `dto.go`, saya telah menanamkan `binding` khusus *Gin Validator*:
```go
Title          string  `json:"title" binding:"required,min=2,max=100"`
Description    string  `json:"description" binding:"omitempty,max=500"`
Level          string  `json:"level" binding:"required"`
MaxPlayers     int     `json:"max_players" binding:"required,min=1"`
PricePerPlayer float64 `json:"price_per_player" binding:"min=0"`
```
Dan di level `service.go`, *Title* dan *Description* secara paksa akan melalui proses `strings.TrimSpace()`. Jika `Title` menjadi kosong pasca-trim, servis akan menembakkan `ErrInvalidTitle` yang dipetakan sebagai **`400 Bad Request`**.

## 4. Standarisasi Timezone `Asia/Jakarta`
Untuk menetralkan perbedaan zona waktu server (UTC) saat memfilter pertandingan *Public List* dan batas penutupan tiket (*expiry*), saya menciptakan *helper*:
```go
func nowJakarta() time.Time { ... }
```
- **List Queries**: Nilai `Now` diinjeksikan langsung dari Go melalui filter sehingga kueri database memakai variabel terikat `$3` (Bukan lagi bergantung pada `now()` di PostgreSQL).
- **Match Calculation**: Jam *Match* (Start Time) diformat menjadi objek waktu absolut `Asia/Jakarta` dengan bantuan `time.Date(...)` sebelum dikomparasi melawan fungsi `nowJakarta()`.

## 5. Test Baru yang Ditambahkan
Saya telah menambahkan sejumlah skenario uji mutakhir di `service_test.go`:
- `TestService_CreateOpenMatch_InvalidTitle`: Uji spasi kosong pada parameter pembuat pertandingan.
- `TestService_JoinOpenMatch_BookingNotConfirmed`: Uji ketahanan fungsi *Join* terhadap *booking* yang tidak bertanda `CONFIRMED`.
- `TestService_CreateOpenMatch_UniqueViolation`: Uji isolasi penanganan eror tingkat duplikasi (Simulasi 23505).

## 6. Hasil Format `gofmt`
Format modul sepenuhnya mematuhi aturan *idiomatic* Go tanpa deviasi.
```bash
> gofmt -l apps/api/internal/mabar
(Kosong - Bersih)
```

## 7. Hasil Testing Lanjutan (`go test ./...`)
Seluruh rangkaian modul bebas regresi:
```text
ok      lapangango-api/internal/mabar   2.591s
```

## 8. Catatan Spesifik tentang *Unique Violation* `23505`
Terkait kode eror `23505` Postgres, saat ini modul tersebut **hanya diuji melalui skenario unit-level simulation** dalam `service_test.go` (`TestService_CreateOpenMatch_UniqueViolation`). Karena *Integration Test Infrastructure* berbasis *Real Database* (seperti Dockerized DB Test / Testcontainers) belum siap tersedia di proyek ini, saya mensimulasikannya via `mockRepo` (secara sengaja mengeluarkan `ErrMatchAlreadyExists` via implementasi antarmuka untuk diuji pergerakannya ke tingkat HTTP *response handler*).

---
**Kesimpulan**: Fitur Mabar MVP kini telah lulus *Final Review* dan 100% dikemas secara rapi. Silakan oper bola ke lini *Frontend*! 🚀
