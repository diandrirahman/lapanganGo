import { execFileSync } from 'node:child_process';
import { writeFileSync, mkdirSync } from 'node:fs';

const baseUrl = 'http://localhost:8080';
const outDir = 'D:/project/lapangGo/outputs/qa/staff-roles-v1.3';
mkdirSync(outDir, { recursive: true });

const runId = `${Date.now()}`;
const password = 'QaStaffV13!2026';
const report = {
  run_id: runId,
  started_at: new Date().toISOString(),
  base_url: baseUrl,
  checks: [],
  ids: {},
  limitations_v1_3: [
    'Password staff masih dibuat langsung oleh owner.',
    'Belum ada forced reset password saat login pertama.',
    'Belum ada endpoint reset password staff oleh owner.',
    'Belum ada forgot-password flow khusus staff selain flow auth umum yang sudah ada.',
  ],
};

function record(name, status, details = {}) {
  report.checks.push({ name, status, ...details });
  const icon = status === 'PASS' ? 'PASS' : status === 'WARN' ? 'WARN' : 'FAIL';
  console.log(`[${icon}] ${name}`);
  if (details.message) console.log(`      ${details.message}`);
}

function sqlEscape(value) {
  return String(value).replaceAll("'", "''");
}

function psql(sql) {
  const output = execFileSync(
    'docker',
    ['exec', 'lapangango_postgres', 'psql', '-q', '-U', 'lapangango_user', '-d', 'lapangango_db', '-t', '-A', '-c', sql],
    { encoding: 'utf8' },
  ).trim();
  return output
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean)
    .find((line) => !line.startsWith('INSERT') && !line.startsWith('UPDATE') && !line.startsWith('DELETE')) || '';
}

async function request(method, path, { token, body, expected = [200], name } = {}) {
  const res = await fetch(`${baseUrl}${path}`, {
    method,
    headers: {
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(body !== undefined ? { 'Content-Type': 'application/json' } : {}),
    },
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  const text = await res.text();
  let data = null;
  try {
    data = text ? JSON.parse(text) : null;
  } catch {
    data = { raw: text };
  }
  if (!expected.includes(res.status)) {
    const label = name || `${method} ${path}`;
    throw new Error(`${label} expected ${expected.join('/')} got ${res.status}: ${text}`);
  }
  return { status: res.status, data };
}

async function register(email, name) {
  await request('POST', '/auth/register', {
    expected: [201, 409],
    body: { name, email, password, phone: '' },
    name: `register ${email}`,
  });
}

async function login(email) {
  const res = await request('POST', '/auth/login', {
    body: { email, password },
    name: `login ${email}`,
  });
  return res.data;
}

function makeOwner(email, businessName) {
  psql(`
    UPDATE users SET role = 'OWNER', status = 'ACTIVE' WHERE email = '${sqlEscape(email)}';
    INSERT INTO owner_profiles (user_id, business_name, verification_status)
    SELECT id, '${sqlEscape(businessName)}', 'APPROVED'
    FROM users WHERE email = '${sqlEscape(email)}'
    ON CONFLICT (user_id) DO UPDATE
      SET business_name = EXCLUDED.business_name,
          verification_status = 'APPROVED';
  `);
}

function userId(email) {
  return psql(`SELECT id::text FROM users WHERE email = '${sqlEscape(email)}'`);
}

function ownerProfileId(email) {
  return psql(`
    SELECT op.id::text
    FROM owner_profiles op
    JOIN users u ON u.id = op.user_id
    WHERE u.email = '${sqlEscape(email)}'
  `);
}

function createPaidBooking({ customerID, courtID, ownerID, venueID, date, start, end, amount }) {
  return psql(`
    WITH b AS (
      INSERT INTO bookings (
        customer_id, court_id, booking_date, start_time, end_time,
        total_price, original_price, final_price, status
      )
      VALUES (
        '${customerID}', '${courtID}', '${date}', '${start}', '${end}',
        ${amount}, ${amount}, ${amount}, 'PAID'
      )
      RETURNING id
    )
    INSERT INTO owner_finance_transactions (
      owner_id, venue_id, booking_id, created_by_user_id,
      type, source, category, amount, transaction_date, description
    )
    SELECT
      '${ownerID}', '${venueID}', b.id, '${ownerID}',
      'INCOME', 'BOOKING', 'BOOKING_PAYMENT', ${amount}, CURRENT_DATE, 'QA booking income'
    FROM b
    RETURNING booking_id::text;
  `);
}

function createWaitingBooking({ customerID, courtID, date, start, end, amount }) {
  return psql(`
    INSERT INTO bookings (
      customer_id, court_id, booking_date, start_time, end_time,
      total_price, original_price, final_price, status, payment_reference
    )
    VALUES (
      '${customerID}', '${courtID}', '${date}', '${start}', '${end}',
      ${amount}, ${amount}, ${amount}, 'WAITING_VERIFICATION', 'qa-proof-${runId}'
    )
    RETURNING id::text;
  `);
}

async function main() {
  const ownerAEmail = `qa.e2e.owner.a.${runId}@lapanggo.test`;
  const ownerBEmail = `qa.e2e.owner.b.${runId}@lapanggo.test`;
  const customerEmail = `qa.e2e.customer.${runId}@lapanggo.test`;
  const staffEmail = `qa.e2e.staff.${runId}@lapanggo.test`;

  const health = await request('GET', '/health', { name: 'health' });
  record('API healthcheck', health.data.status === 'ok' ? 'PASS' : 'FAIL', { response: health.data });

  await register(ownerAEmail, 'QA Owner A');
  await register(ownerBEmail, 'QA Owner B');
  await register(customerEmail, 'QA Customer');
  makeOwner(ownerAEmail, `QA Owner A ${runId}`);
  makeOwner(ownerBEmail, `QA Owner B ${runId}`);

  const ownerALogin = await login(ownerAEmail);
  const ownerBLogin = await login(ownerBEmail);
  const customerLogin = await login(customerEmail);
  const ownerToken = ownerALogin.token;
  const ownerBToken = ownerBLogin.token;
  const customerToken = customerLogin.token;
  const ownerID = userId(ownerAEmail);
  const customerID = userId(customerEmail);
  const ownerProfileID = ownerProfileId(ownerAEmail);
  report.ids.owner_user_id = ownerID;
  report.ids.owner_profile_id = ownerProfileID;
  record('Smoke test owner login + /auth/me owner profile', ownerALogin.user.owner_profile ? 'PASS' : 'FAIL', {
    user_role: ownerALogin.user.role,
    owner_profile: ownerALogin.user.owner_profile,
  });

  const sports = await request('GET', '/sports', { name: 'sports' });
  const futsal = sports.data.sports.find((s) => s.name.toLowerCase().includes('futsal')) || sports.data.sports[0];

  const venueA = (await request('POST', '/owner/venues', {
    token: ownerToken,
    body: { name: `QA Venue A ${runId}`, address: 'Jl QA A', city: 'Bandung', description: 'QA venue A' },
    expected: [201],
    name: 'create venue A',
  })).data.venue;
  const venueB = (await request('POST', '/owner/venues', {
    token: ownerToken,
    body: { name: `QA Venue B ${runId}`, address: 'Jl QA B', city: 'Bandung', description: 'QA venue B' },
    expected: [201],
    name: 'create venue B',
  })).data.venue;
  const venueOtherOwner = (await request('POST', '/owner/venues', {
    token: ownerBToken,
    body: { name: `QA Other Owner Venue ${runId}`, address: 'Jl QA Other', city: 'Bandung' },
    expected: [201],
    name: 'create other owner venue',
  })).data.venue;

  await request('PATCH', `/owner/venues/${venueA.id}/status`, { token: ownerToken, body: { status: 'ACTIVE' }, name: 'activate venue A' });
  await request('PATCH', `/owner/venues/${venueB.id}/status`, { token: ownerToken, body: { status: 'ACTIVE' }, name: 'activate venue B' });
  await request('PATCH', `/owner/venues/${venueOtherOwner.id}/status`, { token: ownerBToken, body: { status: 'ACTIVE' }, name: 'activate other venue' });

  const courtA = (await request('POST', `/owner/venues/${venueA.id}/courts`, {
    token: ownerToken,
    expected: [201],
    body: { sport_id: futsal.id, name: `QA Court A ${runId}`, location_type: 'INDOOR', surface_type: 'Vinyl', price_per_hour: 100000 },
    name: 'create court A',
  })).data.court;
  const courtB = (await request('POST', `/owner/venues/${venueB.id}/courts`, {
    token: ownerToken,
    expected: [201],
    body: { sport_id: futsal.id, name: `QA Court B ${runId}`, location_type: 'INDOOR', surface_type: 'Vinyl', price_per_hour: 100000 },
    name: 'create court B',
  })).data.court;

  const permissions = [
    'DASHBOARD_VIEW', 'ANALYTICS_READ',
    'VENUES_READ', 'VENUES_WRITE', 'COURTS_READ', 'COURTS_WRITE',
    'SCHEDULE_READ', 'SCHEDULE_WRITE',
    'BLOCKED_SLOTS_READ', 'BLOCKED_SLOTS_WRITE',
    'BOOKINGS_READ', 'BOOKINGS_WRITE', 'OFFLINE_BOOKINGS_CREATE', 'PAYMENT_VERIFY',
    'REFUNDS_READ', 'REFUNDS_WRITE',
    'FINANCE_READ', 'FINANCE_WRITE',
    'PROMOS_READ', 'PROMOS_WRITE',
  ];
  const staff = (await request('POST', '/owner/staff', {
    token: ownerToken,
    expected: [201],
    body: {
      name: 'QA Staff Scope',
      email: staffEmail,
      password,
      role: 'MANAGER',
      permissions,
      venue_ids: [venueA.id],
    },
    name: 'create staff',
  })).data;
  report.ids.staff_id = staff.id;
  const invalidVenueAssign = await request('PUT', `/owner/staff/${staff.id}/venues`, {
    token: ownerToken,
    expected: [400],
    body: { venue_ids: [venueOtherOwner.id] },
    name: 'reject foreign venue assignment',
  });
  record('Owner tidak bisa assign venue milik owner lain ke staff', invalidVenueAssign.status === 400 ? 'PASS' : 'FAIL');
  await request('PUT', `/owner/staff/${staff.id}/venues`, {
    token: ownerToken,
    body: { venue_ids: [venueA.id] },
    name: 'restore staff venue A',
  });

  const staffLogin = await login(staffEmail);
  const staffToken = staffLogin.token;
  record('Smoke test login staff', staffLogin.user.role === 'STAFF' && staffLogin.user.staff_memberships?.length === 1 ? 'PASS' : 'FAIL', {
    user_role: staffLogin.user.role,
    memberships: staffLogin.user.staff_memberships,
  });
  const staffCannotManageStaff = await request('GET', '/owner/staff', {
    token: staffToken,
    expected: [403],
    name: 'staff cannot open staff management',
  });
  record('Staff tidak bisa akses /owner/staff', staffCannotManageStaff.status === 403 ? 'PASS' : 'FAIL');

  const tomorrow = new Date(Date.now() + 24 * 60 * 60 * 1000).toISOString().slice(0, 10);
  const bookingA1 = createPaidBooking({ customerID, courtID: courtA.id, ownerID, venueID: venueA.id, date: tomorrow, start: '09:00', end: '10:00', amount: 100000 });
  const bookingA2 = createPaidBooking({ customerID, courtID: courtA.id, ownerID, venueID: venueA.id, date: tomorrow, start: '10:00', end: '11:00', amount: 100000 });
  const bookingA3 = createPaidBooking({ customerID, courtID: courtA.id, ownerID, venueID: venueA.id, date: tomorrow, start: '11:00', end: '12:00', amount: 100000 });
  const bookingB1 = createPaidBooking({ customerID, courtID: courtB.id, ownerID, venueID: venueB.id, date: tomorrow, start: '09:00', end: '10:00', amount: 100000 });
  const waitingA = createWaitingBooking({ customerID, courtID: courtA.id, date: tomorrow, start: '12:00', end: '13:00', amount: 100000 });
  const waitingB = createWaitingBooking({ customerID, courtID: courtB.id, date: tomorrow, start: '12:00', end: '13:00', amount: 100000 });
  report.ids.bookings = { bookingA1, bookingA2, bookingA3, bookingB1, waitingA, waitingB };

  const staffVenueABookings = await request('GET', `/owner/venues/${venueA.id}/bookings?date=${tomorrow}`, { token: staffToken, name: 'staff list venue A bookings' });
  const staffVenueBBookings = await request('GET', `/owner/venues/${venueB.id}/bookings?date=${tomorrow}`, { token: staffToken, name: 'staff list venue B bookings' });
  const venueABookingRows = staffVenueABookings.data.data || staffVenueABookings.data.bookings || [];
  const venueBBookingRows = staffVenueBBookings.data.data || staffVenueBBookings.data.bookings || [];
  const venueAPass = venueABookingRows.length > 0;
  const venueBPass = venueBBookingRows.length === 0;
  record('Test akses venue: staff melihat Venue A dan tidak melihat Venue B', venueAPass && venueBPass ? 'PASS' : 'FAIL', {
    venue_a_count: venueABookingRows.length,
    venue_b_count: venueBBookingRows.length,
  });

  const txA = await request('POST', '/owner/finance/transactions', {
    token: staffToken,
    expected: [201],
    body: { venue_id: venueA.id, type: 'INCOME', category: 'QA_STAFF_ALLOWED', amount: 12000, transaction_date: tomorrow, description: 'staff allowed' },
    name: 'staff create finance venue A',
  });
  const txNoVenue = await request('POST', '/owner/finance/transactions', {
    token: staffToken,
    expected: [403],
    body: { type: 'INCOME', category: 'QA_NO_VENUE', amount: 12000, transaction_date: tomorrow },
    name: 'staff create finance no venue',
  });
  const txVenueB = await request('POST', '/owner/finance/transactions', {
    token: staffToken,
    expected: [403],
    body: { venue_id: venueB.id, type: 'INCOME', category: 'QA_FORBIDDEN', amount: 12000, transaction_date: tomorrow },
    name: 'staff create finance venue B',
  });
  const ownerTxB = await request('POST', '/owner/finance/transactions', {
    token: ownerToken,
    expected: [201],
    body: { venue_id: venueB.id, type: 'INCOME', category: 'QA_OWNER_B_TX', amount: 22000, transaction_date: tomorrow },
    name: 'owner create finance venue B',
  });
  const staffDeleteTxB = await request('DELETE', `/owner/finance/transactions/${ownerTxB.data.id}`, {
    token: staffToken,
    expected: [403],
    name: 'staff delete venue B finance',
  });
  record('Test finance staff scope', txA.status === 201 && txNoVenue.status === 403 && txVenueB.status === 403 && staffDeleteTxB.status === 403 ? 'PASS' : 'FAIL', {
    allowed_tx_id: txA.data.id,
    no_venue_status: txNoVenue.status,
    venue_b_status: txVenueB.status,
    delete_venue_b_status: staffDeleteTxB.status,
  });

  const verifyA = await request('PATCH', `/owner/bookings/${waitingA}/verify-payment`, {
    token: staffToken,
    body: { is_approved: true },
    name: 'staff verify venue A payment',
  });
  const verifyB = await request('PATCH', `/owner/bookings/${waitingB}/verify-payment`, {
    token: staffToken,
    expected: [403],
    body: { is_approved: true },
    name: 'staff verify venue B payment',
  });
  const cancelRefundA = await request('PATCH', `/owner/bookings/${bookingA1}/cancel-refund`, {
    token: staffToken,
    body: { reason: 'QA staff cancel refund allowed' },
    name: 'staff cancel refund venue A booking',
  });
  const cancelRefundB = await request('PATCH', `/owner/bookings/${bookingB1}/cancel-refund`, {
    token: staffToken,
    expected: [403],
    body: { reason: 'QA staff cancel refund forbidden' },
    name: 'staff cancel refund venue B booking',
  });
  record('Test booking staff scope', verifyA.status === 200 && verifyB.status === 403 && cancelRefundA.status === 200 && cancelRefundB.status === 403 ? 'PASS' : 'FAIL', {
    verify_a_status: verifyA.status,
    verify_b_status: verifyB.status,
    cancel_refund_a_status: cancelRefundA.status,
    cancel_refund_b_status: cancelRefundB.status,
  });

  const refundReq = await request('POST', `/bookings/${bookingA2}/refund-request`, {
    token: customerToken,
    expected: [201],
    body: { reason: 'QA customer requests refund because schedule changed.' },
    name: 'customer request refund venue A',
  });
  const staffRefundList = await request('GET', '/owner/refund-requests', {
    token: staffToken,
    name: 'staff list refund requests',
  });
  const refundRequestID = (refundReq.data.refund_request || refundReq.data).id;
  const approveRefund = await request('PATCH', `/owner/refund-requests/${refundRequestID}/approve`, {
    token: staffToken,
    body: { owner_note: 'QA approved by staff' },
    name: 'staff approve refund venue A',
  });
  record('Test refund staff scope', refundReq.status === 201 && (staffRefundList.data.data || []).length > 0 && approveRefund.status === 200 ? 'PASS' : 'FAIL', {
    refund_request_id: refundRequestID,
    list_count: (staffRefundList.data.data || []).length,
    approve_status: approveRefund.status,
  });

  await request('PUT', `/owner/staff/${staff.id}/venues`, {
    token: ownerToken,
    body: { venue_ids: [] },
    name: 'set staff no venues',
  });
  const noVenueBookings = await request('GET', '/owner/bookings', { token: staffToken, name: 'staff no venue bookings' });
  const noVenueFinance = await request('GET', '/owner/finance/transactions', { token: staffToken, name: 'staff no venue finance' });
  const noVenueRefunds = await request('GET', '/owner/refund-requests', { token: staffToken, name: 'staff no venue refunds' });
  const noVenueAnalytics = await request('GET', '/owner/analytics/bookings', { token: staffToken, name: 'staff no venue analytics' });
  const noVenuePass =
    (noVenueBookings.data.data || []).length === 0 &&
    (noVenueFinance.data.transactions || []).length === 0 &&
    (noVenueRefunds.data.data || []).length === 0 &&
    (noVenueAnalytics.data.trend || []).length === 0;
  record('Test staff tanpa venue: semua data venue kosong', noVenuePass ? 'PASS' : 'FAIL', {
    bookings_count: (noVenueBookings.data.data || []).length,
    finance_count: (noVenueFinance.data.transactions || []).length,
    refund_count: (noVenueRefunds.data.data || []).length,
    analytics_trend_count: (noVenueAnalytics.data.trend || []).length,
  });

  await request('PUT', `/owner/staff/${staff.id}/venues`, {
    token: ownerToken,
    body: { venue_ids: [venueA.id] },
    name: 'restore staff venue A after empty scope test',
  });
  await request('PATCH', `/owner/staff/${staff.id}/status`, {
    token: ownerToken,
    body: { status: 'INACTIVE' },
    name: 'deactivate staff',
  });
  const inactiveStaffAccess = await request('GET', `/owner/venues/${venueA.id}/bookings?date=${tomorrow}`, {
    token: staffToken,
    expected: [403],
    name: 'inactive staff denied',
  });
  await request('PATCH', `/owner/staff/${staff.id}/status`, {
    token: ownerToken,
    body: { status: 'ACTIVE' },
    name: 'reactivate staff',
  });
  const reactivatedStaffAccess = await request('GET', `/owner/venues/${venueA.id}/bookings?date=${tomorrow}`, {
    token: staffToken,
    name: 'reactivated staff allowed',
  });
  record('Test status staff inactive/active', inactiveStaffAccess.status === 403 && reactivatedStaffAccess.status === 200 ? 'PASS' : 'FAIL', {
    inactive_status: inactiveStaffAccess.status,
    reactivated_status: reactivatedStaffAccess.status,
  });

  psql(`UPDATE users SET status = 'SUSPENDED' WHERE id = '${ownerID}'`);
  const suspendedOwnerStaffAccess = await request('GET', `/owner/venues/${venueA.id}/bookings?date=${tomorrow}`, {
    token: staffToken,
    expected: [403],
    name: 'staff denied when owner user suspended',
  });
  psql(`UPDATE users SET status = 'ACTIVE' WHERE id = '${ownerID}'`);
  const restoredOwnerStaffAccess = await request('GET', `/owner/venues/${venueA.id}/bookings?date=${tomorrow}`, {
    token: staffToken,
    name: 'staff allowed when owner user active again',
  });
  record('Test status owner suspend/active cascade ke staff', suspendedOwnerStaffAccess.status === 403 && restoredOwnerStaffAccess.status === 200 ? 'PASS' : 'FAIL', {
    suspended_status: suspendedOwnerStaffAccess.status,
    restored_status: restoredOwnerStaffAccess.status,
  });

  report.finished_at = new Date().toISOString();
  report.summary = {
    passed: report.checks.filter((c) => c.status === 'PASS').length,
    failed: report.checks.filter((c) => c.status === 'FAIL').length,
    warnings: report.checks.filter((c) => c.status === 'WARN').length,
  };
  const reportPath = `${outDir}/e2e-staff-roles-v13-report-${runId}.json`;
  writeFileSync(reportPath, JSON.stringify(report, null, 2));
  console.log(`REPORT_PATH=${reportPath}`);
  if (report.summary.failed > 0) process.exitCode = 1;
}

main().catch((err) => {
  record('E2E runner crashed', 'FAIL', { message: err.stack || err.message });
  report.finished_at = new Date().toISOString();
  report.summary = {
    passed: report.checks.filter((c) => c.status === 'PASS').length,
    failed: report.checks.filter((c) => c.status === 'FAIL').length,
    warnings: report.checks.filter((c) => c.status === 'WARN').length,
  };
  const reportPath = `${outDir}/e2e-staff-roles-v13-report-${runId}.json`;
  writeFileSync(reportPath, JSON.stringify(report, null, 2));
  console.error(`REPORT_PATH=${reportPath}`);
  process.exit(1);
});
