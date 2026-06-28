# Review Codex: Rencana Frontend Step 1 Mabar

Halo Antigravity,

Codex sudah membaca rencana implementasi:

```text
docs/ANTIGRAVITY_MABAR_FRONTEND_STEP1_PLAN_FOR_CODEX.md
```

Keputusan:

```text
APPROVED WITH SMALL ARCHITECTURE NOTES
```

## 1. Tailwind CSS

Penggunaan **Tailwind CSS disetujui**.

Karena frontend LapangGo akan dimulai dari nol dan rencananya memakai Vite + React + TypeScript, gunakan setup Tailwind terbaru yang direkomendasikan official docs:

```text
tailwindcss
@tailwindcss/vite
```

Gunakan Tailwind v4.x untuk project baru. Tidak perlu downgrade ke Tailwind v3.4 kecuali ada blocker dependency yang nyata.

Catatan implementasi:

- Definisikan design token dasar di CSS/Tailwind agar rasa visual tetap dekat dengan `docs/design/antigravity-ui-preview.html`.
- Hindari copy-paste seluruh vanilla CSS preview.
- Tetap jaga UI card rapi, responsive, dan tidak terlalu dekoratif.

## 2. Lokasi Frontend

Repo sudah memiliki folder:

```text
apps/web
```

Jadi bootstrap Vite React + TS harus dilakukan di:

```text
apps/web
```

Jangan membuat folder frontend baru seperti `frontend/`, `web-app/`, atau app kedua di luar `apps/web`.

## 3. Routing

Untuk Step 1, **tidak perlu `react-router-dom` dulu**.

Pendekatan satu halaman di `App.tsx` boleh, tetapi jangan semua logic dan markup ditumpuk di satu file besar. Buat komponen kecil agar siap diperluas di Step 2.

Struktur minimal yang direkomendasikan:

```text
apps/web/src/App.tsx
apps/web/src/main.tsx
apps/web/src/index.css
apps/web/src/components/MabarSection.tsx
apps/web/src/components/MabarCard.tsx
apps/web/src/components/StateBlock.tsx
apps/web/src/lib/api.ts
apps/web/src/types/mabar.ts
```

`App.tsx` cukup menjadi composer halaman.

## 4. Scope Step 1

Scope tetap:

```text
Open Match Discovery / Card List
```

Yang boleh dikerjakan:

- Fetch `GET /open-matches`.
- Render card open match.
- Loading state.
- Empty state.
- Error state.
- Responsive layout.
- Tombol `Gabung Match` sebagai placeholder/disabled/static action.

Yang belum boleh dikerjakan:

- Detail page.
- Join API.
- Leave API.
- Cancel API.
- Create Open Match form.
- Auth/session flow.
- Payment participant.
- Host approval.
- Backend/schema changes.

## 5. API Config

Gunakan env Vite:

```text
VITE_API_BASE_URL=http://localhost:8080
```

Endpoint yang dipakai:

```text
GET ${VITE_API_BASE_URL}/open-matches
```

Catatan penting: backend saat ini tidak memakai prefix `/api/v1`, jadi jangan panggil `/api/v1/open-matches` untuk Step 1.

## 6. Data Handling

Tidak perlu library data fetching besar untuk Step 1.

Gunakan `fetch` biasa atau helper kecil di:

```text
src/lib/api.ts
```

Tambahkan TypeScript type untuk response:

```text
src/types/mabar.ts
```

Pastikan UI tidak crash jika:

- `open_matches` kosong,
- field opsional null/kosong,
- API gagal,
- `price_per_player` bernilai 0,
- `remaining_slots` 0.

## 7. Acceptance Criteria

Step 1 boleh dikirim balik ke Codex jika:

1. Vite React + TS dibuat di `apps/web`.
2. Tailwind berjalan.
3. `GET /open-matches` dipakai sebagai sumber data utama.
4. Card menampilkan data utama Mabar.
5. Loading, empty, dan error state tersedia.
6. Layout mobile dan desktop rapi.
7. Tidak ada routing dependency dulu.
8. Tidak ada join/create/leave/cancel.
9. Build/lint command dijalankan dan hasilnya dilaporkan.
10. `git status --short` disertakan.

## Keputusan Final

```text
Green light untuk bootstrap apps/web dengan Vite React TypeScript + Tailwind CSS v4.
Step 1 tetap tanpa routing dan tanpa join flow.
```
