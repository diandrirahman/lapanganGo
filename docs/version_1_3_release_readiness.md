# LapangGo Release Readiness: Version 1.3

**Target Milestone**: `v1.3.0` (Staff Roles & Audit Trail)
**Commit HEAD**: `eca5d05e docs: trim transient agent reports`
**Branch Target**: `master`
**Date**: July 2026

## 1. Scope v1.3
Rilis versi 1.3 ini meliputi fitur utama berikut:
- **Staff Roles v1.3**: Memungkinkan Owner untuk membuat akun staf (kasir/admin lapangan) dengan akses spesifik ke venue tertentu. Staf memiliki hak akses terbatas (hanya mengelola finance, booking, court schedule, refund sesuai venue yang diizinkan) dan terisolasi sepenuhnya dari menu administratif Owner lainnya.
- **Audit Trail v1.3.1**: Modul rekam jejak sistem untuk mencatat aksi krusial yang dilakukan oleh Owner maupun Staf. Aksi dicatat dengan `actor_role` yang sesuai (`OWNER` atau `STAFF`) untuk tujuan transparansi dan sekuritas.

## 2. Migration Baseline
Seluruh struktur database diatur menggunakan `golang-migrate`. Untuk rilis v1.3, baseline skema database telah stabil hingga versi migrasi ke-17:
- `016_staff_roles.up.sql`
- `017_owner_audit_logs.up.sql`
*(Diuji mulus dari awal hingga akhir pada lingkungan fresh database)*

## 3. Hasil Automated Verification
Seluruh automated check telah dieksekusi dari master branch yang bersih, dengan hasil berikut:
- **Backend Test (`go test ./...`)**: `PASS` (Seluruh test di 26 packages internal berhasil).
- **Frontend Build (`npm run build`)**: `PASS` (Build sukses, ukuran chunk sesuai toleransi).
- **Frontend Lint (`npm run lint`)**: `PASS` (1 warning `exhaustive-deps` diabaikan sesuai SOP).
- **E2E Staff Script (`e2e_staff_roles_v13.mjs`)**: `PASS` (12/12 skenario pengujian isolasi role dan venue berjalan sempurna).

## 4. Hasil Manual QA (Browser Sanity Check)
Pengujian simulasi UI telah dilakukan (Environment: Frontend Docker Web `http://localhost:3000` dan Vite Dev `http://localhost:5174`, Backend `http://localhost:8080`).

| Skenario | Status | Keterangan |
| :--- | :---: | :--- |
| **Customer Booking Flow** | `PASS` | Customer berhasil menelusuri ketersediaan lapangan dan membuat booking (termasuk open match). |
| **Owner Verify Payment** | `PASS` | Owner dapat melihat booking berstatus WAITING_VERIFICATION dan menyetujuinya. |
| **Staff Login & Venue Scope** | `PASS` | Staff berhasil login. UI navbar terisolasi. Staff hanya melihat list venue A (sesuai assign) dan tidak bisa melihat venue B. Jika tak punya venue, list kosong. |
| **Owner Audit Logs Only** | `PASS` | Akses ke halaman `/owner/audit-logs` lancar bagi Owner dan ditolak (`403 Forbidden`) saat diakses menggunakan akun Staff. Record `STAFF_CREATED` & `FINANCE_CREATED` tercatat sempurna. |

## 5. Known Limitations
Keterbatasan sementara yang dibawa ke versi rilis ini (telah disetujui dalam plan awal v1.3):
- Password staff masih dibuat/ditentukan oleh owner saat akun staff dibuat.
- Belum ada *forced reset password* saat login pertama bagi staf.
- Belum ada endpoint *reset password* staff oleh owner.
- Belum ada flow *forgot-password* khusus staff (hanya mengikuti auth flow umum).

## 6. Go/No-Go Decision
**Decision: GO**
Seluruh baris kode termutakhir telah diverifikasi stabil tanpa adanya regresi maupun kebocoran permission. Aplikasi siap dirilis.
