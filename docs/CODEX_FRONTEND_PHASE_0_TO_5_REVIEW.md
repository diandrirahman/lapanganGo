# Review Codex: Frontend Phase 0.1 sampai Step 5

Codex sudah mereview:

```text
docs/CODEX_FRONTEND_SUMMARY_REPORT_PHASE_0_TO_5.md
apps/web
apps/api routes/dto terkait
```

Verifikasi command:

```text
npm.cmd run lint  -> warning
npm.cmd run build -> PASS
```

Keputusan:

```text
REQUEST CHANGES
```

Secara struktur frontend sudah bergerak bagus, tetapi belum bisa dianggap selesai untuk Phase 0-5 karena ada mismatch kontrak API backend yang akan membuat live backend mode gagal.

## Yang Sudah Baik

- `apps/web` sudah punya routing dasar.
- Auth UI sudah tersedia.
- Homepage, venue card, venue detail, availability page, dan create booking flow sudah mulai terbentuk.
- Build production lulus.
- Struktur komponen mulai rapi: page, component, feedback, ui, context.

## Finding 1: Frontend Memanggil Endpoint Court Public Yang Tidak Ada

Prioritas:

```text
P0 - blocker live backend
```

Frontend:

```ts
GET /venues/:id/courts
```

Lokasi:

```text
apps/web/src/lib/api.ts
apps/web/src/pages/VenueDetailPage.tsx
```

Backend public route yang ada:

```http
GET /venues
GET /venues/:id
```

Tidak ada public route:

```http
GET /venues/:id/courts
```

Court list untuk public venue detail sudah dikirim melalui response:

```go
type PublicVenueDetailResponse struct {
    PublicVenueResponse
    Courts []PublicCourtResponse `json:"courts"`
}
```

Arahan:

- Jangan fetch `/venues/:id/courts`.
- Ubah `fetchVenueById` agar type-nya mencerminkan response detail:

```ts
VenueDetail = Venue & { courts: PublicCourt[] }
```

- `VenueDetailPage` cukup fetch `GET /venues/:id`, lalu ambil `venueData.courts`.

## Finding 2: Type `Court` Frontend Tidak Cocok Dengan Backend Public Court

Prioritas:

```text
P0 - blocker rendering court dari data nyata
```

Frontend saat ini:

```ts
export interface Court {
  id: string;
  venue_id: string;
  name: string;
  type: string;
  price_per_hour: number;
  status: string;
}
```

Backend public court response:

```json
{
  "id": "...",
  "sport": { "id": "...", "name": "Futsal" },
  "name": "...",
  "description": "...",
  "location_type": "INDOOR",
  "surface_type": "...",
  "price_per_hour": 150000,
  "created_at": "...",
  "updated_at": "..."
}
```

Tidak ada:

```text
type
venue_id
status
```

Arahan:

- Buat type `PublicCourt`.
- `CourtCard` harus memakai:

```text
court.sport.name
court.location_type
court.price_per_hour
```

- Jangan tampilkan status `ACTIVE` jika field tidak ada di public response.

## Finding 3: Availability Response Shape Salah

Prioritas:

```text
P0 - blocker availability page
```

Frontend menganggap response:

```ts
SlotAvailability[]
```

dengan field:

```ts
time
status
price
```

Backend sebenarnya mengirim:

```json
{
  "court_id": "...",
  "date": "YYYY-MM-DD",
  "status": "OPEN",
  "slots": [
    {
      "start_at": "...",
      "end_at": "...",
      "status": "AVAILABLE"
    }
  ]
}
```

Lokasi frontend terdampak:

```text
apps/web/src/lib/api.ts
apps/web/src/types/booking.ts
apps/web/src/pages/CourtAvailabilityPage.tsx
```

Akibatnya:

- `setSlots(data)` akan menyimpan object, bukan array.
- `slots.map(...)` bisa crash.
- `slot.time` tidak ada.
- `slot.price` tidak ada.
- Create booking bisa mengirim `start_time` invalid.

Arahan:

- Buat type sesuai backend:

```ts
interface AvailabilityResponse {
  court_id: string;
  date: string;
  status: 'OPEN' | 'CLOSED';
  slots: {
    start_at: string;
    end_at: string;
    status: 'AVAILABLE' | 'BOOKED' | 'BLOCKED';
  }[];
}
```

- `fetchCourtAvailability` return `AvailabilityResponse`.
- Di UI, map dari `availability.slots`.
- Tampilkan waktu dari `start_at` dan `end_at`.
- Untuk create booking, convert:

```text
start_at -> HH:mm
end_at -> HH:mm
```

- Jangan tampilkan `slot.price` kecuali sumber harga tersedia dari court detail/route state.

## Finding 4: Login Page Masih Ada Mojibake

Prioritas:

```text
P1
```

Lokasi:

```text
apps/web/src/pages/LoginPage.tsx
```

Ada placeholder:

```text
â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢
```

Arahan:

Ganti ke ASCII:

```text
********
```

atau:

```text
Masukkan password
```

## Finding 5: Masih Ada Browser `alert()`

Prioritas:

```text
P2
```

Lokasi:

```text
apps/web/src/pages/CourtAvailabilityPage.tsx
```

Saat belum login:

```ts
alert("Silakan login terlebih dahulu untuk melakukan pemesanan.")
```

Arahan:

- Hindari browser `alert()` untuk UX utama.
- Tampilkan inline message/toast-like state, lalu redirect ke `/login`.
- Kalau tetap redirect, beri query/route state agar user tahu kenapa diarahkan.

## Finding 6: Lint Masih Warning

Prioritas:

```text
P2
```

Command:

```text
npm.cmd run lint
```

Output:

```text
src/contexts/AuthContext.tsx:87:14 warning react(only-export-components)
```

Report mengklaim lint tanpa teguran, tetapi real output masih ada warning.

Arahan:

- Pindahkan `useAuth` ke file terpisah, misalnya:

```text
src/contexts/useAuth.ts
```

atau sesuaikan struktur agar lint clean.

Acceptance harus:

```text
Found 0 warnings and 0 errors.
```

## Finding 7: Mock Env Naming Kurang Jelas

Prioritas:

```text
P3
```

Frontend memakai:

```text
VITE_USE_MOCK_VENUE
```

untuk venue, court, availability, bahkan auth mock fallback.

Ini tidak fatal, tetapi membingungkan karena efeknya bukan venue saja.

Arahan opsional:

- Ganti menjadi:

```text
VITE_USE_MOCK_PUBLIC_DATA
```

atau pisahkan:

```text
VITE_USE_MOCK_VENUE
VITE_USE_MOCK_AUTH
```

Jika tidak diganti sekarang, minimal dokumentasikan bahwa flag itu memengaruhi public flow mock.

## Acceptance Criteria Revisi

Phase 0-5 bisa di-approve jika:

1. `VenueDetailPage` tidak memanggil endpoint public yang tidak ada.
2. Type court frontend cocok dengan backend public court response.
3. Availability page memakai `response.slots` dan field `start_at/end_at/status`.
4. Create booking mengirim `start_time/end_time` valid dari slot backend.
5. Tidak ada mojibake.
6. Tidak ada browser `alert()` untuk auth guard booking.
7. `npm.cmd run lint` output clean tanpa warning.
8. `npm.cmd run build` lulus.
9. Report diperbarui dengan klaim yang sesuai real output.

## Expected Follow-up

Kirim ulang:

```text
docs/CODEX_FRONTEND_SUMMARY_REPORT_PHASE_0_TO_5.md
git status --short
npm.cmd run lint output
npm.cmd run build output
```

Jika memungkinkan, sertakan juga catatan manual test dengan backend demo seed:

```text
VITE_USE_MOCK_MABAR=false
VITE_USE_MOCK_VENUE=false
VITE_API_BASE_URL=http://localhost:8080
```
