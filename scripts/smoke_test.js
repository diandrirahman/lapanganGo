const API_URL = 'http://localhost:8080';

async function request(path, options = {}) {
  const url = `${API_URL}${path}`;
  const response = await fetch(url, {
    headers: {
      'Content-Type': 'application/json',
      ...options.headers,
    },
    ...options,
  });

  if (!response.ok) {
    const errorText = await response.text();
    throw new Error(`Request failed: ${response.status} ${response.statusText} - ${errorText}`);
  }
  return response.json();
}

async function runSmokeTests() {
  console.log('🚀 Starting Final Release QA Smoke Test...\n');
  try {
    // 1. Healthcheck
    console.log('Testing /health...');
    const health = await request('/health');
    if (health.status !== 'ok') throw new Error('Healthcheck failed');
    console.log('✅ /health OK');

    // 2. DB Health
    console.log('Testing /db-health...');
    const dbHealth = await request('/db-health');
    if (dbHealth.status !== 'ok') throw new Error('DB Healthcheck failed');
    console.log('✅ /db-health OK');

    // 3. User & Owner Auth
    console.log('Testing Authentication (Register/Login)...');
    const customerEmail = `customer_${Date.now()}@test.com`;
    const password = 'Password123!';
    await request('/auth/register', {
      method: 'POST',
      body: JSON.stringify({ name: 'Test Customer', email: customerEmail, password, role: 'CUSTOMER', phone: '0812' + Math.floor(Math.random() * 10000000) })
    });

    const ownerEmail = `owner_${Date.now()}@test.com`;
    await request('/auth/register', {
      method: 'POST',
      body: JSON.stringify({ name: 'Test Owner', email: ownerEmail, password, role: 'CUSTOMER', phone: '0813' + Math.floor(Math.random() * 10000000) })
    });
    
    // Upgrade to owner via DB since API only allows CUSTOMER registration
    const { execSync } = require('child_process');
    execSync(`docker-compose exec -T postgres psql -U lapangango_user -d lapangango_db -c "UPDATE users SET role='OWNER' WHERE email='${ownerEmail}'"`, { stdio: 'ignore' });

    const customerLogin = await request('/auth/login', {
      method: 'POST',
      body: JSON.stringify({ email: customerEmail, password })
    });
    const customerToken = customerLogin.token;

    const ownerLogin = await request('/auth/login', {
      method: 'POST',
      body: JSON.stringify({ email: ownerEmail, password })
    });
    const ownerToken = ownerLogin.token;
    console.log('✅ Auth (Register & Login) OK');

    // 3.5 Create Owner Profile
    console.log('Testing Owner Profile Creation...');
    await request('/owner/profile', {
      method: 'POST',
      headers: { 'Authorization': `Bearer ${ownerToken}` },
      body: JSON.stringify({
        business_name: 'Smoke Test Business',
        phone: '081399998888',
        address: 'Jl. Test No. 1'
      })
    });
    console.log('✅ Create Owner Profile OK');

    // 3.8 Fetch Facilities
    console.log('Testing Fetch /facilities...');
    const facRes = await request('/facilities');
    if (!facRes.facilities || facRes.facilities.length < 2) {
      throw new Error('Not enough facilities found');
    }
    const facilityIds = [facRes.facilities[0].id, facRes.facilities[1].id];
    console.log('✅ Fetch Facilities OK');

    // 4. Create Venue (Owner)
    console.log('Testing Owner Venue Creation...');
    const venueRes = await request('/owner/venues', {
      method: 'POST',
      headers: { 'Authorization': `Bearer ${ownerToken}` },
      body: JSON.stringify({
        name: 'Smoke Test Arena',
        address: 'Jl. Test No. 1',
        city: 'Jakarta Selatan',
        facility_ids: facilityIds
      })
    });
    const venueId = venueRes.venue.id;
    console.log('✅ Create Venue OK');

    // 4.5 Set Venue to ACTIVE via DB
    execSync(`docker-compose exec -T postgres psql -U lapangango_user -d lapangango_db -c "UPDATE venues SET status='ACTIVE' WHERE id='${venueId}'"`, { stdio: 'ignore' });

    // 5. Search Venues
    console.log('Testing Search /venues...');
    const venuesRes = await request('/venues?city=Jakarta%20Selatan');
    if (!venuesRes.data || !Array.isArray(venuesRes.data)) throw new Error('Invalid venues search response');
    const found = venuesRes.data.find(v => v.id === venueId);
    if (!found) throw new Error('Newly created venue not found in search');
    console.log('✅ Search /venues OK');

    // 6. Detail Venue
    console.log('Testing Detail /venues/:id...');
    const venueDetail = await request(`/venues/${venueId}`);
    if (venueDetail.id !== venueId) throw new Error('Invalid venue detail response');
    if (!venueDetail.facilities || venueDetail.facilities.length < 2) throw new Error('Facilities not saved/returned correctly');
    console.log('✅ Detail Venue OK');

    // 7. Create Court
    const sportsRes = await request('/sports');
    if (!sportsRes.sports || sportsRes.sports.length === 0) throw new Error('No sports found in DB');
    const sportId = sportsRes.sports[0].id;

    console.log('Testing Owner Court Creation...');
    const courtRes = await request(`/owner/venues/${venueId}/courts`, {
      method: 'POST',
      headers: { 'Authorization': `Bearer ${ownerToken}` },
      body: JSON.stringify({
        name: 'Lapangan 1',
        sport_id: sportId,
        location_type: 'INDOOR',
        surface_type: 'Vinyl',
        price_per_hour: 50000
      })
    });
    const courtId = courtRes.court.id;
    console.log('✅ Create Court OK');

    // 7.5 Set Operating Hours
    console.log('Testing Set Operating Hours...');
    const days = [];
    for (let i = 0; i < 7; i++) {
      days.push({ day_of_week: i, open_time: '08:00', close_time: '22:00', is_closed: false });
    }
    await request(`/owner/courts/${courtId}/operating-hours`, {
      method: 'PUT',
      headers: { 'Authorization': `Bearer ${ownerToken}` },
      body: JSON.stringify({ days })
    });
    console.log('✅ Set Operating Hours OK');

    // 8. Owner Metrics
    console.log('Testing Owner Metrics...');
    const dashboard = await request('/owner/metrics', {
      headers: { 'Authorization': `Bearer ${ownerToken}` }
    });
    if (dashboard.metrics.total_venues !== 1) throw new Error('Dashboard metric mismatch');
    console.log('✅ Owner Dashboard OK');

    const testDate = new Date();
    testDate.setDate(testDate.getDate() + 1);
    const dateStr = testDate.toISOString().split('T')[0];
    const blockStart = `${dateStr}T10:00:00Z`;
    const blockEnd = `${dateStr}T12:00:00Z`;

    // 9. Create Blocked Slot (Owner)
    console.log('Testing Create Blocked Slot...');
    const blockedRes = await request(`/owner/courts/${courtId}/blocked-slots`, {
      method: 'POST',
      headers: { 'Authorization': `Bearer ${ownerToken}` },
      body: JSON.stringify({ start_at: blockStart, end_at: blockEnd, reason: 'Maintenance' })
    });
    const blockedId = blockedRes.blocked_slot.id;
    console.log('✅ Create Blocked Slot OK');

    // 10. List Blocked Slots
    console.log('Testing List Blocked Slots...');
    const listBlockedRes = await request(`/owner/courts/${courtId}/blocked-slots?date=${dateStr}`, {
      headers: { 'Authorization': `Bearer ${ownerToken}` }
    });
    if (!listBlockedRes.blocked_slots || listBlockedRes.blocked_slots.length === 0) throw new Error('Blocked slot not found');
    console.log('✅ List Blocked Slots OK');

    // 11. Delete Blocked Slot
    console.log('Testing Delete Blocked Slot...');
    await request(`/owner/blocked-slots/${blockedId}`, {
      method: 'DELETE',
      headers: { 'Authorization': `Bearer ${ownerToken}` }
    });
    console.log('✅ Delete Blocked Slot OK');

    // 12. Court Availability
    console.log('Testing Court Availability...');
    const availRes = await request(`/courts/${courtId}/availability?date=${dateStr}`);
    if (!availRes.slots) throw new Error('Invalid availability response');
    console.log('✅ Court Availability OK');

    // 13. Create Booking (Customer)
    console.log('Testing Create Booking...');
    const bookingRes = await request('/bookings', {
      method: 'POST',
      headers: { 'Authorization': `Bearer ${customerToken}` },
      body: JSON.stringify({
        court_id: courtId,
        booking_date: dateStr,
        start_time: '14:00',
        end_time: '15:00'
      })
    });
    const bookingId = bookingRes.booking.id;
    console.log('✅ Create Booking OK');

    // 14. List Customer Bookings
    console.log('Testing List Customer Bookings...');
    const listBookingsRes = await request('/bookings', {
      headers: { 'Authorization': `Bearer ${customerToken}` }
    });
    if (!listBookingsRes.data || listBookingsRes.data.length === 0) throw new Error('Customer booking not found in list');
    console.log('✅ List Customer Bookings OK');

    // 15. Detail Booking
    console.log('Testing Detail Booking...');
    const detailBooking = await request(`/bookings/${bookingId}`, {
      headers: { 'Authorization': `Bearer ${customerToken}` }
    });
    if (detailBooking.booking.id !== bookingId) throw new Error('Invalid booking detail');
    console.log('✅ Detail Booking OK');

    // 16. Submit Payment Proof (Customer)
    console.log('Testing Submit Payment Proof...');
    await request(`/bookings/${bookingId}/payment-proof`, {
      method: 'POST',
      headers: { 'Authorization': `Bearer ${customerToken}` },
      body: JSON.stringify({ payment_reference: 'REF-12345' })
    });
    console.log('✅ Submit Payment Proof OK');

    // 17. List Owner Venue Bookings
    console.log('Testing List Owner Venue Bookings...');
    const ownerBookingsRes = await request(`/owner/venues/${venueId}/bookings?date=${dateStr}`, {
      headers: { 'Authorization': `Bearer ${ownerToken}` }
    });
    if (!ownerBookingsRes.data || ownerBookingsRes.data.length === 0) throw new Error('Owner booking not found in list');
    console.log('✅ List Owner Venue Bookings OK');

    // 18. Verify Payment (Owner)
    console.log('Testing Verify Payment...');
    await request(`/owner/bookings/${bookingId}/verify-payment`, {
      method: 'PATCH',
      headers: { 'Authorization': `Bearer ${ownerToken}` },
      body: JSON.stringify({ is_approved: true })
    });
    console.log('✅ Verify Payment OK');

    console.log('\n🎉 All Smoke Tests Passed Successfully!');
  } catch (error) {
    console.error('\n❌ Smoke Test Failed:');
    console.error(error);
    process.exit(1);
  }
}

runSmokeTests();
