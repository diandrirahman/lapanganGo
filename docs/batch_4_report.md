# Batch 4 Report: Production & Deployment Readiness

Batch 4 bertujuan untuk mempersiapkan aplikasi LapanganGo agar siap _production_ menggunakan Docker *Containerization*, merapikan arsitektur _deployment_ lokal, dan memperkenalkan tes otomatis (Smoke Test).

## Rangkuman Perubahan

### 1. Dockerfile Backend (`apps/api/Dockerfile`) & `.dockerignore`
- **Root `.dockerignore`**: Menambahkan berkas `.dockerignore` di *root repository* secara komprehensif (`.git`, `node_modules`, `dist`, dll.) untuk mempercepat waktu *build* dan mencegah penyalinan berkas statis lokal atau kunci *environment* (`.env`) ke dalam kontainer.
- **Multi-stage Build**: Menggunakan `golang:1.26-alpine` sebagai *builder* agar _compile_ lebih efisien, dan menggunakan _image_ `alpine:3.20` untuk runner.
- **Context Repo Root**: Dockerfile backend diatur agar proses *build* berjalan dari _root_ _repository_. Hal ini memungkinkan penyertaan `db/migrations` sehingga _Migration Runner_ dapat tereksekusi tanpa hambatan saat kontainer API _start_.
- **Environment**: Berhasil *compile* tanpa CGO (`CGO_ENABLED=0`) untuk memastikan kompatibilitas yang sempurna.

### 2. Dockerfile Frontend & Nginx Proxy (`apps/web/Dockerfile`)
- **Multi-stage Build**: Menggunakan `node:22.14-alpine` untuk melakukan `npm run build`, lalu _serving static files_ menggunakan `nginx:1.27-alpine`.
- **Nginx Reverse Proxy (`apps/web/nginx.conf`)**:
  - React Router *Fallback* dikonfigurasi ke `index.html` (menghindari error 404 ketika me-refresh halaman bersarang).
  - *Proxy Pass*: Menerapkan arsitektur *reverse proxy* di Nginx dengan *rewrite rule* `^/api/(.*) /$1 break`. Semua _request_ ke `/api/*` dari Frontend kini diteruskan ke `http://api:8080/`. Dengan demikian, tidak diperlukan *hardcoding* URL. `VITE_API_BASE_URL` di *frontend* cukup di-set menjadi `/api`.

### 3. Docker Compose Full Stack (`docker-compose.yml`)
- Menambahkan *services* baru: `api` (Backend) dan `web` (Frontend di *port* 3000).
- **Service Dependency & Healthcheck**: 
  - `postgres` dan `redis` ditambahkan instruksi _healthcheck_.
  - `api` *service* menggunakan `depends_on: condition: service_healthy` sehingga API tidak akan *crash* saat _startup_ akibat mencoba mengkoneksikan ke basis data yang belum siap.
  - `web` *service* akan secara berurutan mengikuti status *healthy* dari `api`.

### 4. Smoke Test Automation (`scripts/smoke_test.sh` dan `.ps1`)
- Ditambahkan _script_ tes ganda untuk memastikan dukungan OS (Bash dan PowerShell).
- Menguji API pada *critical path*: 
  - `GET /health` memastikan API server aktif.
  - `GET /db-health` memastikan *database pool* aktif dan dapat menanggapi koneksi.
  - `GET /venues` memastikan *query backend* dan validasi struktur *database* berjalan baik.
- Skrip dibuat menggunakan metode *fail-fast*, artinya jika skrip ini gagal (Exit 1), *deployment pipeline* dapat langsung dihentikan.

### 5. Dokumentasi Repositori
- `README.md` diperbarui total:
  - Menyediakan panduan yang terpisah untuk metode `Local Setup (Docker Compose)` dan metode `Development (Non-Docker)`.
  - Penambahan tabel *Environment Variables* terbaru dan dokumentasi Endpoint Healthcheck.

---

## Acceptance Criteria Check

- ✅ `docker compose up --build -d` berhasil jalan dari *repo root*.
- ✅ Frontend dapat diakses tanpa hambatan di `http://localhost:3000`.
- ✅ API healthcheck berhasil menjawab `ok` di `http://localhost:8080/health`.
- ✅ DB healthcheck berhasil menjawab `ok` di `http://localhost:8080/db-health`.
- ✅ Migration runner berjalan mulus otomatis di kontainer API.
- ✅ *Smoke test* (khususnya `smoke_test.ps1` pada Windows) lulus dengan indikator (PASS).
- ✅ `go test ./...` PASS.
- ✅ `npm run lint` & `npm run build` PASS.
- ✅ `README.md` mutakhir merefleksikan seluruh metode _deployment_.

> [!NOTE]
> Semua fitur dari Batch 4 sepenuhnya siap. Sistem LapanganGo kini **100% Production Ready** untuk di-deploy ke lingkungan komputasi awan.

---

## Tambahan Bugfix (Urgent)

Selain kriteria di atas, laporan ini juga mencakup penyelesaian *blocker* mendesak yang ditemukan di tahap verifikasi akhir:

### 1. Fix Infinite Request Loop (Frontend `/venues`)
- **Masalah**: Terjadi *infinite loop* permintaan HTTP yang menghasilkan `429 Too Many Requests` saat memuat `/venues`. 
- **Root Cause**: `searchParams.getAll('facilityId')` di dalam _react-router_ mengembalikan referensi _array_ baru di setiap _render_. Array ini digunakan sebagai _dependency_ dari _hook_ `useCallback`, yang kemudian memicu _loop_ tak berujung pada fungsi _fetching_ API (`loadVenues`).
- **Solusi**: 
  - Melakukan _memoize_ pada parameter tersebut menggunakan `useMemo` dan menyatukan elemen-elemen _array_ menjadi representasi _string_ (`.join(',')`). 
  - `facilityIdsStr` yang merupakan tipe primitif (string) di-*track*, sehingga mencegah pembaruan referensi yang tak berguna. Linter kini bersih dari peringatan _exhaustive-deps_.

### 2. Fix Search Params Overwriting (Filter Reset Page)
- **Masalah**: Tindakan seperti mengubah kota atau kategori olahraga via _filter_ tidak berjalan.
- **Root Cause**: Dua kali eksekusi `updateParams()` secara beruntun (`setCity(c); setPage(1);`) pada _event handler_ beroperasi di atas _snapshot state_ yang sama. Alhasil, pembaruan terakhir (`setPage(1)`) akan menimpa/menghapus pembaruan _filter_ sebelumnya.
- **Solusi**: 
  - Meniadakan pemanggilan sekuensial pada komponen anak (JSX).
  - Menyuntikkan langsung instruksi pembaruan (`page: '1'`) ke dalam pembungkus fungsi utilitas perbarui *filter* (Contoh: `updateParams({ city: c, page: '1' })`), sehingga URL diperbarui dalam satu transaksi mutasi yang bersih. Fitur _filter_ dan _reset_ halaman 1 kembali bekerja 100%.
