# Venue Facilities & Venue Photos Implementation Plan

This plan details the backend and frontend changes required to implement venue photos and refine venue facilities, while ensuring owners can manage them effectively.

## Proposed Changes

### Database Migration
#### [NEW] `db/migrations/008_venue_photos.up.sql`
- Create `venue_photos` table with fields: `id` (UUID), `venue_id` (UUID, FK to `venues`), `image_url` (TEXT), `alt_text` (VARCHAR), `sort_order` (INT), `is_primary` (BOOLEAN), and `created_at` (TIMESTAMPTZ).
- Add partial unique index on `(venue_id)` where `is_primary = true` to enforce only one primary photo per venue at the database level.
#### [NEW] `db/migrations/008_venue_photos.down.sql`
- Drop `venue_photos` table.

---

### Backend API (Go)
#### [MODIFY] `apps/api/internal/venues/dto.go`
- Add `VenuePhotoResponse` struct.
- Add `PrimaryPhoto *string` and `Photos []VenuePhotoResponse` to both `PublicVenueDetailResponse` and owner `VenueResponse`.
- Add `PrimaryPhoto *string` to `PublicVenueResponse`.
- Add `CreateVenuePhotoRequest` and `UpdateVenuePhotoRequest` structs.

#### [MODIFY] `apps/api/internal/venues/repository.go`
- Add methods to insert, update, delete, and list photos by `venue_id`.
- Update `GetPublicVenues`, `GetPublicVenue`, and owner list venue queries to LEFT JOIN or perform subqueries to fetch the primary photo URL and facility lists.
- Update `UpdateVenue` and `CreateVenue` to properly sync `venue_facilities` (delete old, insert new).

#### [MODIFY] `apps/api/internal/venues/service.go`
- Implement business logic for `AddVenuePhoto`, `UpdateVenuePhoto`, and `DeleteVenuePhoto`.
- Ensure `is_primary` toggle logic correctly unsets the previous primary photo if a new one is set.

#### [MODIFY] `apps/api/internal/venues/handler.go`
- Add owner routes:
  - `POST /owner/venues/:id/photos`
  - `PUT /owner/venues/:id/photos/:photo_id`
  - `DELETE /owner/venues/:id/photos/:photo_id`

---

### Frontend (React)
#### [MODIFY] `apps/web/src/types/venue.ts`
- Update `Venue` and `VenueDetail` types to include `primary_photo` and `photos[]` array.
- Add `VenuePhoto` interface.

#### [MODIFY] `apps/web/src/components/VenueCard.tsx`
- Use `venue.primary_photo` instead of the old placeholder image. If null, fallback to the placeholder gracefully.

#### [MODIFY] `apps/web/src/pages/VenueDetailPage.tsx`
- Replace the single banner image with an image gallery/carousel if `venue.photos` has multiple images.
- If no photos exist, use the placeholder.

#### [MODIFY] `apps/web/src/pages/owner/OwnerVenuesPage.tsx`
- Add a new "Kelola Foto & Fasilitas" button or integrate it into a new "Edit Venue" modal/page.

#### [NEW] `apps/web/src/pages/owner/EditVenuePage.tsx`
- Since the frontend doesn't have an Edit Venue page yet, I will create a minimal `EditVenuePage.tsx` for owners to:
  - Select/Update facilities (Multi-select).
  - Manage Photos (Add photo URL manually, set primary, delete).
- Update router in `App.tsx` to include `/owner/venues/:id/edit`.

## Verification Plan

### Automated Tests
- Run `go test ./...` to ensure venue repository and service changes don't break existing tests and to test the new photo logic.
- Run `npm run lint` and `npm run build` on the frontend.

### Manual Verification
1. Login as `demo.owner@lapangango.test`.
2. Navigate to "Kelola Venue" and click "Edit" pada salah satu venue.
3. Tambahkan beberapa URL foto secara manual dan pilih fasilitas. Jadikan salah satu foto sebagai primary.
4. Simpan (Save) dan pastikan data persisten.
5. Logout dan cek halaman Homepage / Pencarian untuk memastikan primary photo muncul di Venue Card.
6. Masuk ke halaman Venue Detail untuk memastikan galeri foto dan badge fasilitas muncul dengan benar.
