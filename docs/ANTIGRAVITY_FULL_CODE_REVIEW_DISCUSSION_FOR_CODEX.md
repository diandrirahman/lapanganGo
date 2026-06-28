# Full Code Review Discussion — LapanganGo

**To:** Codex (Product Manager / Expert Reviewer)
**From:** Antigravity (Full-Stack Code Review)
**Date:** 27 Juni 2026

## Konteks

Antigravity telah melakukan full code review terhadap seluruh codebase LapanganGo (backend Go API + frontend React + database migrations). Dokumen ini merangkum temuan dan meminta Codex untuk:

1. Review dan validasi prioritas temuan.
2. Tentukan mana yang harus diperbaiki sekarang vs nanti.
3. Buat step-by-step implementation prompt untuk Antigravity.

## Cara Baca Dokumen Ini

- **CRITICAL** = harus diperbaiki sebelum production. Bug atau security issue.
- **HIGH** = sangat disarankan diperbaiki. Code quality atau UX impact besar.
- **MEDIUM** = improvement yang bagus tapi bisa ditunda.
- **LOW** = nice to have.

---

## Bagian A: Temuan Backend

### A1. [CRITICAL] VerifyPayment Authorization Bypass

**File:** `apps/api/internal/bookings/service.go` (sekitar line 186-208)

**Masalah:** Ada komentar: *"We will skip strict owner-booking validation for now due to demo constraints"*. Artinya **ANY OWNER** bisa approve/reject payment untuk **ANY booking**, bukan hanya booking di venue miliknya.

**Expected behavior:** Owner hanya boleh verify payment untuk booking yang ada di venue miliknya. Validasi chain: booking → court → venue → owner_profile_id == current owner.

**Solusi yang disarankan:**
1. Di `VerifyPayment`, setelah fetch booking, query court → venue → owner_profile_id.
2. Bandingkan dengan owner_profile_id dari authenticated user.
3. Return `ErrForbidden` jika tidak cocok.

---

### A2. [CRITICAL] Booking Expiry Tidak Ada

**Masalah:** Booking dengan status `PENDING_PAYMENT` memblokir slot **tanpa batas waktu**. Tidak ada mekanisme auto-cancel.

**Dampak:** Slot "hantu" — availability menunjukkan BOOKED tapi tidak pernah dibayar. Revenue loss untuk owner.

**Solusi yang disarankan:**
1. Tambah kolom `expires_at TIMESTAMPTZ` di tabel `bookings`.
2. Saat create booking, set `expires_at = NOW() + INTERVAL '30 minutes'`.
3. Buat background goroutine atau cron job yang:
   - Query bookings WHERE status = 'PENDING_PAYMENT' AND expires_at < NOW().
   - Update status menjadi 'CANCELLED'.
4. Availability endpoint juga harus ignore booking yang sudah expired.

**Pertanyaan untuk Codex:**
- TTL 30 menit cukup? Atau perlu configurable via env var?
- Background job cukup pakai goroutine + ticker, atau perlu Redis TTL?

---

### A3. [HIGH] Rate Limiting Tidak Ada

**Masalah:** Semua endpoint tanpa rate limiting. Auth endpoints (`/auth/login`, `/auth/register`) vulnerable terhadap brute force dan credential stuffing.

**Solusi yang disarankan:**
- Redis sudah ada di docker-compose tapi belum digunakan.
- Implementasi sliding window rate limiter pakai Redis.
- Atau pakai in-memory rate limiter (golang.org/x/time/rate) sebagai MVP.
- Target: 10 req/menit untuk auth, 100 req/menit general.

**Pertanyaan untuk Codex:**
- Pakai Redis rate limiter atau in-memory dulu untuk MVP?

---

### A4. [HIGH] Code Duplication — Utility Functions

**Masalah:** Fungsi-fungsi berikut copy-paste di 6+ package secara independen:

```
getAuthenticatedUserID()
isUUID()
isHex()
getUUIDParam()
optionalString()
IsNotFound()
```

**Solusi:** Ekstrak ke `apps/api/internal/common/` atau `apps/api/internal/httputil/`.

---

### A5. [HIGH] Repository Interface Tidak Konsisten

**Masalah:** Hanya `bookings` dan `mabar` yang define repository interface. Package lain (auth, availability, courts, owners, venues, schedules, blockedslots) pakai concrete `*Repository` sehingga tidak bisa unit test tanpa database.

**Solusi:** Definisikan repository interface di semua package agar service bisa di-mock saat testing.

---

### A6. [MEDIUM] Minor Backend Issues

| # | Issue | File | Detail |
|---|---|---|---|
| 1 | `/db-health` context salah | `cmd/api/main.go` | Pakai `context.Background()` bukan `c.Request.Context()` |
| 2 | `ListByCustomerID` missing column | `bookings/repository.go` | Tidak scan `payment_reference`, potential runtime error |
| 3 | Occupancy rate hardcoded | `bookings/repository.go` | `GetOwnerMetrics` return 75% hardcoded |
| 4 | Error message leakage | Beberapa handler | `err.Error()` di-expose ke client response |
| 5 | No pagination | Semua list endpoint | Return semua data tanpa LIMIT |
| 6 | No structured logging | Seluruh backend | Hanya `log.Println` |
| 7 | No API versioning | Router | Tidak ada `/api/v1/` prefix |
| 8 | No Swagger docs | - | API undocumented beyond README |
| 9 | `bookings.status` VARCHAR | Migration 004 | Inkonsisten, table lain pakai ENUM |

---

## Bagian B: Temuan Frontend

### B1. [CRITICAL] VenueCard Menampilkan Data Hardcoded

**File:** `apps/web/src/components/VenueCard.tsx`

**Masalah:** VenueCard menampilkan data yang **tidak sesuai kenyataan**:

```tsx
<span>Rp 150K / Jam</span>        // ← hardcoded, bukan harga court real
<span>🔥 Sedang Ramai</span>       // ← badge palsu
<span>19:00  20:00  21:00</span>   // ← slot dekoratif, bukan availability real
```

**Dampak:** User melihat informasi harga dan ketersediaan yang salah. Merusak trust.

**Solusi yang disarankan:**
- Tampilkan range harga real dari data court (min-max price_per_hour).
- Hapus badge "Sedang Ramai" atau ganti dengan data real (jumlah booking hari ini).
- Hapus slot dekoratif atau ganti dengan jumlah court available.

---

### B2. [CRITICAL] Hero Section Search Bar Disabled

**File:** `apps/web/src/components/HeroSection.tsx`

**Masalah:** Search bar dan sport category chips di hero section semuanya `disabled` dengan `cursor-not-allowed`. Terlihat interaktif tapi tidak berfungsi.

**Dampak:** First impression negatif. User mengira fitur rusak.

**Solusi:**
- Buat search bar functional: redirect ke `/venues?search=<query>`.
- Buat sport chips functional: redirect ke `/venues?sport=<sport>`.
- Atau hapus elemen disabled dan ganti dengan CTA yang jelas.

---

### B3. [HIGH] TypeScript `any` Pervasive

**Masalah:** 

| Pattern | Count | Contoh |
|---|---|---|
| `catch (err: any)` | 20+ | Harusnya `unknown` + type narrow |
| API function return `any` | 12+ | `fetchSports(): Promise<any[]>` |
| Component props `any` | 5+ | `court?: any \| null` |
| `useState<any[]>` | 5+ | Harusnya typed state |

**Solusi:**
- Replace semua `catch (err: any)` dengan `catch (err: unknown)`.
- Type semua API function returns sesuai interface di `types/`.
- Type semua component props.

---

### B4. [HIGH] Code Duplication Frontend

| Fungsi Duplikat | Duplikasi di | Solusi |
|---|---|---|
| `formatRupiah()` | 4 files | Pindah ke `lib/utils.ts` |
| `formatDate()` | 4 files | Pindah ke `lib/utils.ts` |
| `getStatusBadge()` | 4 files | Pindah ke `lib/utils.ts` atau `components/StatusBadge.tsx` |
| `hashString()` + placeholder | 2 files | Pindah ke `lib/utils.ts` |
| Auth guard pattern | 6+ pages | Buat `<ProtectedRoute>` wrapper |
| `API_BASE_URL` | 3-4 files | Pakai satu sumber dari `lib/api.ts` |
| Inline input styling | 30+ occurrences | Pakai `<Input>` component yang sudah ada |

---

### B5. [HIGH] Missing 404 Route

**File:** `apps/web/src/App.tsx`

**Masalah:** Tidak ada catch-all route. URL invalid = blank page.

**Solusi:** Tambah `<Route path="*" element={<NotFoundPage />} />`.

---

### B6. [HIGH] No 401 Interceptor

**File:** `apps/web/src/lib/api.ts`

**Masalah:** Jika JWT expire, frontend tetap di state "authenticated". API calls gagal 401 tapi user tidak di-redirect ke login.

**Solusi:** Di `api.ts`, tambah global response check:

```typescript
if (response.status === 401) {
  localStorage.removeItem('auth_token');
  window.location.href = '/login';
}
```

---

### B7. [MEDIUM] Dead Code

| Dead Code | File | Aksi |
|---|---|---|
| `fetchCustomerBookingDetail()` | `lib/api.ts` | Hapus (tidak dipanggil) |
| `confirmBookingPayment()` | `lib/api.ts` | Hapus (tidak dipanggil) |
| `getCityCoordinates()` | `lib/api.ts` | Hapus (tidak dipanggil) |
| `Select.tsx` component | `components/ui/Select.tsx` | Hapus atau gunakan (semua page pakai raw `<select>`) |

---

### B8. [MEDIUM] No Toast/Notification System

**Masalah:** Setelah aksi sukses (booking, payment, join mabar, cancel), user hanya di-navigate tanpa feedback. Tidak ada toast/snackbar.

**Solusi:** Buat simple toast system atau pakai library ringan.

---

### B9. [MEDIUM] Missing Frontend Features

| Feature | Status | Impact |
|---|---|---|
| User profile editing | ❌ | Customer tidak bisa update data |
| Venue editing (owner) | ❌ | Owner bisa create tapi tidak edit |
| Court deletion (owner) | ❌ | Owner tidak bisa hapus court |
| Booking confirmation page | ❌ | Tidak ada halaman konfirmasi setelah booking |
| Route code splitting | ❌ | Semua pages eagerly loaded |
| Pagination UI | ❌ | List tanpa pagination |

---

## Bagian C: Database

### C1. [MEDIUM] `bookings.status` Pakai VARCHAR Bukan ENUM

**File:** `db/migrations/004_bookings.sql`

**Masalah:** Table lain (`users`, `venues`, `courts`, `owner_profiles`) pakai PostgreSQL ENUM type. Tapi `bookings.status` dan `open_matches.status` pakai VARCHAR + CHECK constraint. Inkonsisten.

**Catatan:** Ini bukan bug, hanya inkonsistensi. VARCHAR + CHECK sebenarnya lebih mudah di-migrate daripada ENUM. Bisa dibiarkan.

---

## Bagian D: Rekomendasi Prioritas untuk Codex

### Batch 1 — Critical Fixes (target: 3-5 hari)

| # | Task | Tipe | Effort |
|---|---|---|---|
| 1 | Fix VerifyPayment authorization bypass (A1) | Security | ½ hari |
| 2 | Fix VenueCard hardcoded data (B1) | UX Trust | ½ hari |
| 3 | Fix Hero section disabled elements (B2) | UX | ½ hari |
| 4 | Add 404 route (B5) | UX | 1 jam |
| 5 | Add 401 interceptor (B6) | Auth | 1 jam |
| 6 | Fix `/db-health` context (A6.1) | Bug | 15 menit |
| 7 | Fix `ListByCustomerID` column mismatch (A6.2) | Bug | 15 menit |
| 8 | Clean up dead code (B7) | Hygiene | 30 menit |

### Batch 2 — Data Integrity & Security (target: 1-2 minggu)

| # | Task | Tipe | Effort |
|---|---|---|---|
| 9 | Implement booking expiry (A2) | Data Integrity | 2-3 hari |
| 10 | Add rate limiting (A3) | Security | 1-2 hari |
| 11 | Extract shared backend utils (A4) | Code Quality | 1 hari |
| 12 | Extract shared frontend utils — formatRupiah, formatDate, getStatusBadge, hashString (B4) | Code Quality | ½ hari |
| 13 | Create ProtectedRoute wrapper (B4) | Code Quality | ½ hari |
| 14 | Add pagination backend + frontend (A6.5) | Performance | 2 hari |

### Batch 3 — Code Quality & Testing (target: 2-3 minggu)

| # | Task | Tipe | Effort |
|---|---|---|---|
| 15 | Fix TypeScript `any` types (B3) | Code Quality | 1-2 hari |
| 16 | Define repository interfaces semua packages (A5) | Testability | 2 hari |
| 17 | Fix frontend input styling — pakai `<Input>` component (B4) | Consistency | 1 hari |
| 18 | Implement toast/notification system (B8) | UX | 1 hari |
| 19 | Sanitize error messages di backend (A6.4) | Security | ½ hari |
| 20 | Implement real occupancy rate (A6.3) | Feature | 1 hari |
| 21 | Add structured logging (A6.6) | Observability | 1 hari |

### Batch 4 — Product Features (future)

Lihat `docs/CODEX_NEXT_STEPS_PROPOSAL.md` untuk roadmap fitur selanjutnya (payment gateway, image upload, review system, dll).

---

## Pertanyaan untuk Codex

1. **Prioritas:** Apakah urutan batch di atas sudah tepat? Ada yang perlu dipindah?
2. **Booking expiry:** TTL 30 menit cukup? Pakai goroutine ticker atau Redis TTL?
3. **Rate limiting:** Pakai Redis atau in-memory rate limiter untuk MVP?
4. **VenueCard:** Tampilkan range harga court (Rp X - Rp Y), atau harga terendah?
5. **Hero search:** Buat functional search atau ganti dengan CTA ke `/venues`?
6. **Pagination:** Cursor-based atau offset-based? Default page size berapa?
7. **`bookings.status` VARCHAR vs ENUM:** Biarkan VARCHAR + CHECK atau migrate ke ENUM?

---

## Request untuk Codex

Setelah review temuan di atas:

1. Validasi dan koreksi prioritas jika perlu.
2. Jawab pertanyaan di atas.
3. Buat prompt implementasi Step-by-Step untuk Antigravity, dimulai dari **Batch 1**.
4. Setiap step harus cukup kecil untuk satu sesi Antigravity.
5. Setiap step harus punya acceptance criteria yang jelas.
6. Jangan gabungkan banyak perubahan di satu step kecuali memang tightly coupled.

Workflow tetap sama:
1. Codex buat prompt untuk Antigravity.
2. Antigravity kerjakan satu step.
3. User kirim report ke Codex.
4. Codex review.
5. Kalau approved, lanjut step berikutnya.
