import { PORTFOLIO_DEMO_SESSION_KEY, isPortfolioDemo } from './config';
import {
  loadPortfolioDemoState,
  nextDemoID,
  portfolioDemoFacilities,
  portfolioDemoSports,
  portfolioDemoUserForRole,
  savePortfolioDemoState,
  type PortfolioDemoState,
} from './state';

const JSON_HEADERS = { 'Content-Type': 'application/json; charset=utf-8' };

const jsonResponse = (body: unknown, status = 200): Response => new Response(JSON.stringify(body), {
  status,
  headers: JSON_HEADERS,
});

const noContent = (): Response => new Response(null, { status: 204 });

const requestBody = (init?: RequestInit): Record<string, any> => {
  if (typeof init?.body !== 'string' || init.body.length === 0) return {};
  try {
    return JSON.parse(init.body) as Record<string, any>;
  } catch {
    return {};
  }
};

const currentSessionRole = (): string | null => {
  try {
    const value = localStorage.getItem(PORTFOLIO_DEMO_SESSION_KEY);
    if (!value) return null;
    return (JSON.parse(value) as { role?: string }).role ?? null;
  } catch {
    return null;
  }
};

const setSession = (userID: string, role: string): void => {
  localStorage.setItem(PORTFOLIO_DEMO_SESSION_KEY, JSON.stringify({ userID, role }));
};

const paginate = <T>(items: T[], url: URL) => {
  const page = Math.max(1, Number(url.searchParams.get('page') ?? 1));
  const limit = Math.max(1, Number(url.searchParams.get('limit') ?? 10));
  const start = (page - 1) * limit;
  return {
    data: items.slice(start, start + limit),
    page,
    limit,
    total: items.length,
    total_pages: Math.max(1, Math.ceil(items.length / limit)),
  };
};

const adminPaginate = <T>(items: T[], url: URL) => {
  const page = Math.max(1, Number(url.searchParams.get('page') ?? 1));
  const limit = Math.max(1, Number(url.searchParams.get('limit') ?? 10));
  const start = (page - 1) * limit;
  return {
    data: items.slice(start, start + limit),
    page,
    limit,
    total_items: items.length,
    total_pages: Math.max(1, Math.ceil(items.length / limit)),
  };
};

const ownerBooking = (state: PortfolioDemoState, booking: Record<string, any>) => {
  const customer = state.users.find((user) => user.id === booking.customer_id);
  return {
    ...booking,
    customer: {
      id: customer?.id ?? booking.customer_id,
      name: customer?.name ?? 'Demo Customer',
      email: customer?.email ?? 'customer@demo.lapanggo.test',
      phone: customer?.phone,
    },
  };
};

const mutateBooking = (state: PortfolioDemoState, id: string, status: string) => {
  const booking = state.bookings.find((item) => item.id === id);
  if (!booking) return null;
  booking.status = status as typeof booking.status;
  booking.updated_at = '2026-08-01T10:00:00.000Z';
  savePortfolioDemoState(state);
  return booking;
};

const financeSummary = (state: PortfolioDemoState) => {
  const income = state.ownerTransactions.filter((item) => item.type === 'INCOME').reduce((sum, item) => sum + Number(item.amount), 0);
  const expense = state.ownerTransactions.filter((item) => item.type === 'EXPENSE').reduce((sum, item) => sum + Number(item.amount), 0);
  return {
    total_income: income,
    total_expense: expense,
    net_profit: income - expense,
    realized_booking_revenue: income,
    manual_income: 0,
    manual_expense: expense,
    refund_expense: 100000,
    transaction_count: state.ownerTransactions.length,
    venue_breakdown: [{ venue_id: 'venue-1', venue_name: 'Arena Senayan Demo', realized_revenue: income, booking_count: 8 }],
    status_breakdown: [{ status: 'COMPLETED', amount: income, booking_count: 8 }],
    daily_cashflow: [{ date: '2030-06-15', income, expense, net: income - expense }],
    expense_by_category: [{ category: 'MAINTENANCE', amount: expense }],
  };
};

const platformFinanceSummary = () => ({
  period: { start_date: '2026-08-01', end_date: '2026-08-31' },
  mode: 'SIMULATION', currency: 'IDR', timezone: 'Asia/Jakarta', generated_at: '2026-08-01T10:00:00.000Z',
  as_of: '2026-08-01T10:00:00.000Z', granularity: 'day',
  metrics: {
    online_gmv_gross: '12500000', refund_principal: '500000', online_gmv_net: '12000000', projected_commission: '1200000',
    projected_owner_net_after_hypothetical_commission: '10800000', realized_online_booking_count: 48, refunded_booking_count: 2,
    legacy_manual_realized_gmv: '0', gateway_captured_gmv: null, actual_commission_revenue: null,
    payment_processing_expense: null, platform_operating_expense: '450000', projected_operating_result_before_transaction_costs: '750000',
    platform_revenue: null, transaction_contribution: null, operating_result: null,
  },
  data_availability: { platform_operating_expense: 'AVAILABLE', actual_platform_revenue: 'PROJECTION_ONLY', payment_processing_expense: 'UNAVAILABLE', owner_payable: 'UNAVAILABLE' },
  trend: [{ period_start: '2026-08-01', period_end: '2026-08-01', online_gmv_gross: '12500000', refund_principal: '500000', online_gmv_net: '12000000', projected_commission: '1200000', platform_operating_expense: '450000' }],
  caveats: ['Portfolio demo menggunakan data sintetis dan mode simulasi.'],
});

async function routePortfolioDemoRequest(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
  const rawURL = typeof input === 'string' ? input : input instanceof URL ? input.toString() : input.url;
  const url = new URL(rawURL, window.location.origin);
  const path = url.pathname.replace(/\/+$/, '') || '/';
  const method = (init?.method ?? (input instanceof Request ? input.method : 'GET')).toUpperCase();
  const body = requestBody(init);
  const state = loadPortfolioDemoState();

  if (path === '/auth/me' && method === 'GET') {
    const role = currentSessionRole();
    const user = role ? portfolioDemoUserForRole(state, role) : undefined;
    return user ? jsonResponse({ user }) : jsonResponse({ message: 'Demo session tidak tersedia' }, 401);
  }

  if (path === '/auth/login' && method === 'POST') {
    const user = state.users.find((item) => item.email.toLowerCase() === String(body.email ?? '').toLowerCase())
      ?? state.users.find((item) => item.role === 'CUSTOMER');
    if (!user) return jsonResponse({ message: 'Demo user tidak tersedia' }, 401);
    setSession(user.id, user.role);
    return jsonResponse({ message: 'Login demo berhasil', token: `portfolio-demo:${user.role}`, user });
  }

  if (path === '/auth/register' && method === 'POST') {
    const user = portfolioDemoUserForRole(state, 'CUSTOMER')!;
    setSession(user.id, user.role);
    return jsonResponse({ message: 'Registrasi disimulasikan', token: 'portfolio-demo:CUSTOMER', user }, 201);
  }

  if (path === '/staff/setup-password' && method === 'POST') {
    return jsonResponse({ message: 'Password staff berhasil disimulasikan' });
  }

  if (path === '/sports' && method === 'GET') return jsonResponse({ sports: portfolioDemoSports });
  if (path === '/facilities' && method === 'GET') return jsonResponse({ facilities: portfolioDemoFacilities });

  if (path === '/venues' && method === 'GET') {
    let items = state.venues.filter((venue) => venue.status === 'ACTIVE');
    const query = url.searchParams.get('q')?.toLowerCase();
    const city = url.searchParams.get('city');
    if (query) items = items.filter((venue) => `${venue.name} ${venue.city}`.toLowerCase().includes(query));
    if (city) items = items.filter((venue) => venue.city === city);
    return jsonResponse(paginate(items, url));
  }

  const publicVenueMatch = path.match(/^\/venues\/([^/]+)$/);
  if (publicVenueMatch && method === 'GET') {
    const venue = state.venues.find((item) => item.id === publicVenueMatch[1]);
    if (!venue) return jsonResponse({ message: 'Venue demo tidak ditemukan' }, 404);
    return jsonResponse({ ...venue, courts: state.courts.filter((court) => court.venue_id === venue.id) });
  }

  const availabilityMatch = path.match(/^\/courts\/([^/]+)\/availability$/);
  if (availabilityMatch && method === 'GET') {
    const date = url.searchParams.get('date') ?? '2030-06-15';
    const slots = Array.from({ length: 15 }, (_, index) => {
      const hour = 8 + index;
      return {
        start_at: `${date}T${String(hour).padStart(2, '0')}:00:00+07:00`,
        end_at: `${date}T${String(hour + 1).padStart(2, '0')}:00:00+07:00`,
        status: hour === 12 ? 'BLOCKED' : hour === 18 ? 'BOOKED' : 'AVAILABLE',
      };
    });
    return jsonResponse({ court_id: availabilityMatch[1], date, status: 'OPEN', slots });
  }

  if (path === '/promos/validate' && method === 'POST') {
    const promo = state.promos.find((item) => item.code === String(body.promo_code ?? '').toUpperCase());
    if (!promo) return jsonResponse({ message: 'Kode promo demo tidak valid' }, 404);
    return jsonResponse({ promo_id: promo.id, promo_code: promo.code, promo_name: promo.name, original_price: 100000, discount_amount: 20000, final_price: 80000 });
  }

  if (path === '/bookings' && method === 'GET') return jsonResponse(paginate(state.bookings, url));
  if (path === '/bookings' && method === 'POST') {
    const court = state.courts.find((item) => item.id === body.court_id) ?? state.courts[0];
    const venue = state.venues.find((item) => item.id === court.venue_id)!;
    const booking = {
      id: nextDemoID(state, 'booking'), customer_id: 'user-customer-001', court_id: court.id,
      venue: { id: venue.id, name: venue.name, address: venue.address, city: venue.city },
      court: { id: court.id, name: court.name, sport_name: court.sport.name },
      booking_date: body.booking_date, start_time: body.start_time, end_time: body.end_time,
      total_price: court.price_per_hour, status: 'PENDING_PAYMENT' as const,
      expires_at: '2030-06-15T10:30:00.000Z', created_at: '2026-08-01T10:00:00.000Z', updated_at: '2026-08-01T10:00:00.000Z',
    };
    state.bookings.unshift(booking);
    savePortfolioDemoState(state);
    return jsonResponse({ booking }, 201);
  }

  const bookingDetailMatch = path.match(/^\/bookings\/([^/]+)$/);
  if (bookingDetailMatch && method === 'GET') {
    const booking = state.bookings.find((item) => item.id === bookingDetailMatch[1]);
    return booking ? jsonResponse({ booking }) : jsonResponse({ message: 'Booking demo tidak ditemukan' }, 404);
  }

  const bookingCancelMatch = path.match(/^\/bookings\/([^/]+)\/cancel$/);
  if (bookingCancelMatch && method === 'PATCH') {
    const booking = mutateBooking(state, bookingCancelMatch[1], 'CANCELLED');
    return booking ? jsonResponse({ booking }) : jsonResponse({ message: 'Booking tidak ditemukan' }, 404);
  }

  const paymentProofMatch = path.match(/^\/bookings\/([^/]+)\/payment-proof$/);
  if (paymentProofMatch && method === 'POST') {
    const booking = mutateBooking(state, paymentProofMatch[1], 'WAITING_VERIFICATION');
    if (booking) booking.payment_reference = String(body.payment_reference ?? 'DEMO-PAYMENT');
    savePortfolioDemoState(state);
    return booking ? jsonResponse({ booking }) : jsonResponse({ message: 'Booking tidak ditemukan' }, 404);
  }

  const refundByBookingMatch = path.match(/^\/bookings\/([^/]+)\/refund-request$/);
  if (refundByBookingMatch && method === 'GET') {
    return jsonResponse({ data: state.refunds.find((item) => item.booking_id === refundByBookingMatch[1]) ?? null });
  }
  if (refundByBookingMatch && method === 'POST') {
    const refund = { id: nextDemoID(state, 'refund'), booking_id: refundByBookingMatch[1], customer_id: 'user-customer-001', owner_id: 'owner-profile-001', reason: body.reason, status: 'PENDING', requested_at: '2026-08-01T10:00:00.000Z', created_at: '2026-08-01T10:00:00.000Z', updated_at: '2026-08-01T10:00:00.000Z' };
    state.refunds.unshift(refund);
    savePortfolioDemoState(state);
    return jsonResponse({ refund_request: refund }, 201);
  }

  const paymentResolveMatch = path.match(/^\/payment-attempts\/resolve\/([^/]+)$/);
  if (paymentResolveMatch && method === 'GET') {
    return jsonResponse({ payment_attempt: { id: 'payment-attempt-demo', booking_id: 'booking-1', state: 'PENDING', expires_at: '2030-06-15T10:30:00.000Z' }, status_url: `/payments/return/${paymentResolveMatch[1]}/pending` });
  }

  if (path === '/open-matches' && method === 'GET') return jsonResponse(paginate(state.matches, url));
  const openMatchDetail = path.match(/^\/open-matches\/([^/]+)$/);
  if (openMatchDetail && method === 'GET') {
    const match = state.matches.find((item) => item.id === openMatchDetail[1]);
    return match ? jsonResponse({ open_match: match, participants: state.participants[match.id] ?? [] }) : jsonResponse({ message: 'Mabar demo tidak ditemukan' }, 404);
  }
  const openMatchJoin = path.match(/^\/open-matches\/([^/]+)\/join$/);
  if (openMatchJoin && ['POST', 'DELETE'].includes(method)) {
    const match = state.matches.find((item) => item.id === openMatchJoin[1]);
    if (!match) return jsonResponse({ message: 'Mabar demo tidak ditemukan' }, 404);
    match.joined_count = Math.max(0, match.joined_count + (method === 'POST' ? 1 : -1));
    match.remaining_slots = Math.max(0, match.max_players - match.joined_count);
    savePortfolioDemoState(state);
    return noContent();
  }
  const openMatchCancel = path.match(/^\/open-matches\/([^/]+)\/cancel$/);
  if (openMatchCancel && method === 'PATCH') {
    const match = state.matches.find((item) => item.id === openMatchCancel[1]);
    if (match) match.status = 'CANCELLED';
    savePortfolioDemoState(state);
    return noContent();
  }
  const createMatch = path.match(/^\/bookings\/([^/]+)\/open-matches$/);
  if (createMatch && method === 'POST') {
    const match = { ...state.matches[0], ...body, id: nextDemoID(state, 'match'), booking_id: createMatch[1], joined_count: 1, remaining_slots: Math.max(0, Number(body.max_players ?? 10) - 1), created_at: '2026-08-01T10:00:00.000Z', updated_at: '2026-08-01T10:00:00.000Z' };
    state.matches.unshift(match);
    savePortfolioDemoState(state);
    return jsonResponse({ open_match: match }, 201);
  }

  if (path === '/owner/profile' && method === 'GET') return jsonResponse({ profile: state.owners[0] });
  if (path === '/owner/metrics' && method === 'GET') return jsonResponse({ metrics: { total_venues: state.venues.filter((venue) => venue.owner_profile_id === 'owner-profile-001').length, upcoming_bookings: 8, pending_verifications: state.bookings.filter((booking) => booking.status === 'WAITING_VERIFICATION').length, revenue_current: 850000, booking_revenue_current: 850000, refund_current: 100000, net_revenue_current: 750000, revenue_all_time: 12500000, occupancy_rate: 64 } });
  if (path === '/owner/analytics/bookings' && method === 'GET') return jsonResponse({ trend: [{ date: '2026-08-01', booking_count: 5 }, { date: '2026-08-02', booking_count: 8 }, { date: '2026-08-03', booking_count: 6 }] });
  if (path === '/owner/analytics/revenue' && method === 'GET') return jsonResponse({ trend: [{ date: '2026-08-01', revenue: 450000 }, { date: '2026-08-02', revenue: 600000 }], venue_breakdown: [{ venue_id: 'venue-1', venue_name: 'Arena Senayan Demo', revenue: 850000 }, { venue_id: 'venue-4', venue_name: 'Futsal Center Tebet', revenue: 350000 }] });
  if (path === '/owner/analytics/status' && method === 'GET') return jsonResponse({ breakdown: [{ status: 'COMPLETED', booking_count: 6, amount: 850000 }, { status: 'CANCELLED', booking_count: 2, amount: 100000 }] });
  if (path === '/owner/analytics/expenses' && method === 'GET') return jsonResponse({ breakdown: [{ category: 'MAINTENANCE', amount: 125000 }] });
  if (path === '/owner/finance/summary' && method === 'GET') return jsonResponse(financeSummary(state));
  if (path === '/owner/finance/transactions' && method === 'GET') return jsonResponse({ transactions: state.ownerTransactions, total: state.ownerTransactions.length, page: 1, limit: 10 });
  if (path === '/owner/finance/transactions' && method === 'POST') {
    const transaction = { id: nextDemoID(state, 'transaction'), owner_id: 'owner-profile-001', source: 'MANUAL', ...body, created_at: '2026-08-01T10:00:00.000Z', updated_at: '2026-08-01T10:00:00.000Z' };
    state.ownerTransactions.unshift(transaction);
    savePortfolioDemoState(state);
    return jsonResponse(transaction, 201);
  }
  const transactionDelete = path.match(/^\/owner\/finance\/transactions\/([^/]+)$/);
  if (transactionDelete && method === 'DELETE') {
    state.ownerTransactions = state.ownerTransactions.filter((item) => item.id !== transactionDelete[1]);
    savePortfolioDemoState(state);
    return noContent();
  }

  if (path === '/owner/venues' && method === 'GET') return jsonResponse({ venues: state.venues.filter((venue) => venue.owner_profile_id === 'owner-profile-001') });
  if (path === '/owner/venues' && method === 'POST') {
    const venue = { ...state.venues[0], ...body, id: nextDemoID(state, 'venue'), owner_profile_id: 'owner-profile-001', created_at: '2026-08-01T10:00:00.000Z', updated_at: '2026-08-01T10:00:00.000Z' };
    state.venues.unshift(venue);
    savePortfolioDemoState(state);
    return jsonResponse({ venue }, 201);
  }
  const ownerVenue = path.match(/^\/owner\/venues\/([^/]+)$/);
  if (ownerVenue && method === 'GET') {
    const venue = state.venues.find((item) => item.id === ownerVenue[1]);
    return venue ? jsonResponse({ venue }) : jsonResponse({ message: 'Venue tidak ditemukan' }, 404);
  }
  if (ownerVenue && method === 'PUT') {
    const venue = state.venues.find((item) => item.id === ownerVenue[1]);
    if (!venue) return jsonResponse({ message: 'Venue tidak ditemukan' }, 404);
    Object.assign(venue, body, { updated_at: '2026-08-01T10:00:00.000Z' });
    savePortfolioDemoState(state);
    return jsonResponse({ venue });
  }
  const ownerVenuePhotos = path.match(/^\/owner\/venues\/([^/]+)\/photos$/);
  if (ownerVenuePhotos && method === 'POST') {
    const venue = state.venues.find((item) => item.id === ownerVenuePhotos[1]);
    if (!venue) return jsonResponse({ message: 'Venue tidak ditemukan' }, 404);
    const photo = {
      id: nextDemoID(state, 'venue-photo'),
      venue_id: venue.id,
      image_url: String(body.image_url ?? '/hero-basketball.webp'),
      alt_text: String(body.alt_text ?? `${venue.name} demo`),
      sort_order: Number(body.sort_order ?? venue.public_photos?.length ?? 0),
      is_primary: Boolean(body.is_primary),
      created_at: '2026-08-01T10:00:00.000Z',
    };
    venue.public_photos = [...(venue.public_photos ?? []), photo.image_url];
    if (photo.is_primary) venue.primary_photo = photo.image_url;
    savePortfolioDemoState(state);
    return jsonResponse({ photo }, 201);
  }
  const ownerVenuePhoto = path.match(/^\/owner\/venues\/([^/]+)\/photos\/([^/]+)$/);
  if (ownerVenuePhoto && method === 'PUT') {
    const venue = state.venues.find((item) => item.id === ownerVenuePhoto[1]);
    if (!venue) return jsonResponse({ message: 'Venue tidak ditemukan' }, 404);
    const photo = { id: ownerVenuePhoto[2], venue_id: venue.id, ...body };
    if (body.is_primary && body.image_url) venue.primary_photo = String(body.image_url);
    savePortfolioDemoState(state);
    return jsonResponse({ photo });
  }
  if (ownerVenuePhoto && method === 'DELETE') {
    const venue = state.venues.find((item) => item.id === ownerVenuePhoto[1]);
    if (!venue) return jsonResponse({ message: 'Venue tidak ditemukan' }, 404);
    venue.public_photos = body.image_url
      ? venue.public_photos?.filter((url) => url !== body.image_url)
      : (venue.public_photos ?? []).slice(0, -1);
    savePortfolioDemoState(state);
    return jsonResponse({ message: 'Foto demo dihapus' });
  }
  const ownerVenueCourts = path.match(/^\/owner\/venues\/([^/]+)\/courts$/);
  if (ownerVenueCourts && method === 'GET') return jsonResponse({ courts: state.courts.filter((court) => court.venue_id === ownerVenueCourts[1]) });
  if (ownerVenueCourts && method === 'POST') {
    const court = { ...state.courts[0], ...body, id: nextDemoID(state, 'court'), venue_id: ownerVenueCourts[1], sport: portfolioDemoSports.find((sport) => sport.id === body.sport_id) ?? portfolioDemoSports[0], created_at: '2026-08-01T10:00:00.000Z', updated_at: '2026-08-01T10:00:00.000Z' };
    state.courts.unshift(court);
    savePortfolioDemoState(state);
    return jsonResponse({ court }, 201);
  }
  const ownerCourt = path.match(/^\/owner\/courts\/([^/]+)$/);
  if (ownerCourt && method === 'PUT') {
    const court = state.courts.find((item) => item.id === ownerCourt[1]);
    if (!court) return jsonResponse({ message: 'Court tidak ditemukan' }, 404);
    Object.assign(court, body);
    savePortfolioDemoState(state);
    return jsonResponse({ court });
  }
  const hoursMatch = path.match(/^\/owner\/courts\/([^/]+)\/operating-hours$/);
  if (hoursMatch && method === 'GET') return jsonResponse({ operating_hours: state.operatingHours.filter((item) => item.court_id === hoursMatch[1]) });
  if (hoursMatch && method === 'PUT') {
    state.operatingHours = state.operatingHours.filter((item) => item.court_id !== hoursMatch[1]);
    state.operatingHours.push(...(body.days ?? []).map((item: any, index: number) => ({ ...item, id: `hours-${hoursMatch[1]}-${index}`, court_id: hoursMatch[1] })));
    savePortfolioDemoState(state);
    return jsonResponse({ operating_hours: state.operatingHours.filter((item) => item.court_id === hoursMatch[1]) });
  }
  const blockedMatch = path.match(/^\/owner\/courts\/([^/]+)\/blocked-slots$/);
  if (blockedMatch && method === 'GET') return jsonResponse({ blocked_slots: state.blockedSlots.filter((item) => item.court_id === blockedMatch[1]) });
  if (blockedMatch && method === 'POST') {
    const blocked = {
      id: nextDemoID(state, 'blocked-slot'),
      court_id: blockedMatch[1],
      start_at: String(body.start_at ?? ''),
      end_at: String(body.end_at ?? ''),
      reason: body.reason ? String(body.reason) : undefined,
      created_at: '2026-08-01T10:00:00.000Z',
      updated_at: '2026-08-01T10:00:00.000Z',
    };
    state.blockedSlots.push(blocked);
    savePortfolioDemoState(state);
    return jsonResponse({ blocked_slot: blocked }, 201);
  }
  const blockedDelete = path.match(/^\/owner\/blocked-slots\/([^/]+)$/);
  if (blockedDelete && method === 'DELETE') {
    state.blockedSlots = state.blockedSlots.filter((item) => item.id !== blockedDelete[1]);
    savePortfolioDemoState(state);
    return noContent();
  }

  if (path === '/owner/bookings' && method === 'GET') return jsonResponse(paginate(state.bookings.map((booking) => ownerBooking(state, booking)), url));
  if (path === '/owner/bookings/offline' && method === 'POST') {
    const booking = { ...state.bookings[0], ...body, id: nextDemoID(state, 'booking-offline'), customer_id: 'offline-demo-customer', status: body.status ?? 'PAID', created_at: '2026-08-01T10:00:00.000Z', updated_at: '2026-08-01T10:00:00.000Z' };
    state.bookings.unshift(booking);
    savePortfolioDemoState(state);
    return jsonResponse({ booking }, 201);
  }
  const ownerVenueBookings = path.match(/^\/owner\/venues\/([^/]+)\/bookings$/);
  if (ownerVenueBookings && method === 'GET') {
    const items = state.bookings.filter((booking) => booking.venue?.id === ownerVenueBookings[1]).map((booking) => ownerBooking(state, booking));
    return jsonResponse(paginate(items, url));
  }
  const ownerBookingAction = path.match(/^\/owner\/bookings\/([^/]+)\/(verify-payment|mark-paid|complete|cancel-refund)$/);
  if (ownerBookingAction && ['PATCH', 'POST'].includes(method)) {
    const statusByAction: Record<string, string> = { 'verify-payment': body.is_approved === false ? 'PENDING_PAYMENT' : 'PAID', 'mark-paid': 'PAID', complete: 'COMPLETED', 'cancel-refund': 'CANCELLED' };
    const booking = mutateBooking(state, ownerBookingAction[1], statusByAction[ownerBookingAction[2]]);
    return booking ? jsonResponse({ booking }) : jsonResponse({ message: 'Booking tidak ditemukan' }, 404);
  }

  if (path === '/owner/promos' && method === 'GET') return jsonResponse(state.promos);
  if (path === '/owner/promos' && method === 'POST') {
    const promo = { ...state.promos[0], ...body, id: nextDemoID(state, 'promo'), owner_id: 'owner-profile-001', usage_count: 0, total_discount_amount: 0, total_final_revenue: 0, can_delete: true, created_at: '2026-08-01T10:00:00.000Z', updated_at: '2026-08-01T10:00:00.000Z' };
    state.promos.unshift(promo);
    savePortfolioDemoState(state);
    return jsonResponse(promo, 201);
  }
  const promoMatch = path.match(/^\/owner\/promos\/([^/]+)(?:\/(toggle))?$/);
  if (promoMatch && method === 'GET') return jsonResponse(state.promos.find((item) => item.id === promoMatch[1]) ?? {}, 200);
  if (promoMatch && method === 'PUT') {
    const promo = state.promos.find((item) => item.id === promoMatch[1]);
    if (promo) Object.assign(promo, body);
    savePortfolioDemoState(state);
    return jsonResponse(promo ?? {});
  }
  if (promoMatch && promoMatch[2] === 'toggle' && method === 'PATCH') {
    const promo = state.promos.find((item) => item.id === promoMatch[1]);
    if (promo) promo.status = promo.status === 'ACTIVE' ? 'INACTIVE' : 'ACTIVE';
    savePortfolioDemoState(state);
    return jsonResponse(promo ?? {});
  }
  if (promoMatch && method === 'DELETE') {
    state.promos = state.promos.filter((item) => item.id !== promoMatch[1]);
    savePortfolioDemoState(state);
    return noContent();
  }

  if (path === '/owner/staff' && method === 'GET') return jsonResponse({ staff: state.staff });
  if (path === '/owner/staff' && method === 'POST') {
    const staff = { ...state.staff[0], ...body, id: nextDemoID(state, 'staff'), user_id: nextDemoID(state, 'user-staff'), invitation_status: 'PENDING', status: 'ACTIVE', invite_url: '/staff/setup-password?token=portfolio-demo', created_at: '2026-08-01T10:00:00.000Z', updated_at: '2026-08-01T10:00:00.000Z' };
    state.staff.unshift(staff);
    savePortfolioDemoState(state);
    return jsonResponse({ staff, invite_url: staff.invite_url }, 201);
  }
  const staffMatch = path.match(/^\/owner\/staff\/([^/]+)(?:\/(regenerate-invite|reset-password|status))?$/);
  if (staffMatch && method === 'PUT') {
    const staff = state.staff.find((item) => item.id === staffMatch[1]);
    if (staff) Object.assign(staff, body);
    savePortfolioDemoState(state);
    return jsonResponse({ staff });
  }
  if (staffMatch && staffMatch[2] === 'status' && method === 'PATCH') {
    const staff = state.staff.find((item) => item.id === staffMatch[1]);
    if (staff) staff.status = body.status ?? (staff.status === 'ACTIVE' ? 'INACTIVE' : 'ACTIVE');
    savePortfolioDemoState(state);
    return jsonResponse({ staff });
  }
  if (staffMatch && ['regenerate-invite', 'reset-password'].includes(staffMatch[2] ?? '') && method === 'POST') {
    return jsonResponse({ invite_url: '/staff/setup-password?token=portfolio-demo', reset_url: '/staff/setup-password?token=portfolio-demo', expires_at: '2030-06-16T00:00:00.000Z', email_delivery: { attempted: false, sent: false, message: 'Portfolio demo tidak mengirim email.' } });
  }
  if (staffMatch && method === 'DELETE') {
    state.staff = state.staff.filter((item) => item.id !== staffMatch[1]);
    savePortfolioDemoState(state);
    return noContent();
  }

  if (path === '/owner/refund-requests' && method === 'GET') return jsonResponse(paginate(state.refunds, url));
  const refundAction = path.match(/^\/owner\/refund-requests\/([^/]+)\/(approve|reject)$/);
  if (refundAction && method === 'PATCH') {
    const refund = state.refunds.find((item) => item.id === refundAction[1]);
    if (refund) Object.assign(refund, { status: refundAction[2] === 'approve' ? 'APPROVED' : 'REJECTED', owner_note: body.owner_note, reviewed_at: '2026-08-01T10:00:00.000Z' });
    savePortfolioDemoState(state);
    return jsonResponse({ message: 'Refund demo diperbarui' });
  }
  if (path === '/owner/audit-logs' && method === 'GET') return jsonResponse(paginate(state.auditLogs, url));

  if (path === '/notifications' && method === 'GET') return jsonResponse(paginate(state.notifications, url));
  if (path === '/notifications/unread-count' && method === 'GET') return jsonResponse({ count: state.notifications.filter((item) => !item.read_at).length });
  if (path === '/notifications/read-all' && method === 'PATCH') {
    state.notifications.forEach((item) => { item.read_at = '2026-08-01T10:00:00.000Z'; });
    savePortfolioDemoState(state);
    return noContent();
  }
  const notificationRead = path.match(/^\/notifications\/([^/]+)\/read$/);
  if (notificationRead && method === 'PATCH') {
    const notification = state.notifications.find((item) => item.id === notificationRead[1]);
    if (notification) notification.read_at = '2026-08-01T10:00:00.000Z';
    savePortfolioDemoState(state);
    return noContent();
  }

  if (path === '/admin/dashboard' && method === 'GET') return jsonResponse({ total_users: state.users.length, total_owners: state.owners.length, total_venues: state.venues.length, total_bookings: state.bookings.length });
  if (path === '/admin/users' && method === 'GET') return jsonResponse(adminPaginate(state.users, url));
  if (path === '/admin/owners' && method === 'GET') return jsonResponse(adminPaginate(state.owners, url));
  if (path === '/admin/venues' && method === 'GET') return jsonResponse(adminPaginate(state.venues.map((venue) => ({ id: venue.id, owner_profile_id: venue.owner_profile_id, name: venue.name, city: venue.city, status: venue.status, created_at: venue.created_at })), url));
  const adminOwnerStatus = path.match(/^\/admin\/owners\/([^/]+)\/status$/);
  if (adminOwnerStatus && method === 'PATCH') {
    const owner = state.owners.find((item) => item.id === adminOwnerStatus[1]);
    if (owner) owner.status = body.status;
    savePortfolioDemoState(state);
    return noContent();
  }
  const adminVenueStatus = path.match(/^\/admin\/venues\/([^/]+)\/status$/);
  if (adminVenueStatus && method === 'PATCH') {
    const venue = state.venues.find((item) => item.id === adminVenueStatus[1]);
    if (venue) venue.status = body.status;
    savePortfolioDemoState(state);
    return noContent();
  }
  if (path === '/admin/audit-logs' && method === 'GET') return jsonResponse(adminPaginate(state.auditLogs, url));
  if (path === '/admin/commercial-terms' && method === 'GET') return jsonResponse(adminPaginate([{ id: 'term-1', owner_profile_id: null, scope_key: 'GLOBAL', label: 'Portfolio Demo Terms', phase: 'TRIAL', finance_mode: 'SIMULATION', collection_method: 'NONE', commission_bps: 1000, valid_from: '2026-01-01', valid_until: null, supersedes_id: null, created_by_user_id: 'user-super-admin-001', created_at: '2026-08-01T08:00:00.000Z', status: 'CURRENT' }], url));
  if (path === '/admin/finance/summary' && method === 'GET') return jsonResponse(platformFinanceSummary());
  if (path === '/admin/finance/breakdown' && method === 'GET') return jsonResponse({ mode: 'SIMULATION', data: [{ owner_profile_id: 'owner-profile-001', business_name: 'LapangGo Demo Sports 01', realized_online_booking_count: 8, online_gmv_net: '1200000', projected_commission: '120000', projection_basis: 'DEMO', legacy_scenario_count: 0, snapshot_projection_count: 8, non_billable_projection_amount: '0', snapshot_projection_amount: '120000' }], total_items: 1, total_pages: 1, page: 1, limit: 10, as_of: '2026-08-01T10:00:00.000Z', generated_at: '2026-08-01T10:00:00.000Z', metric_source_version: 'portfolio-demo-v1', projection_basis: 'DEMO', legacy_scenario_count: 0, snapshot_projection_count: 8, non_billable_projection_amount: '0', snapshot_projection_amount: '120000', platform_operating_expense: '450000', data_availability: platformFinanceSummary().data_availability });
  if (path === '/admin/finance/expenses' && method === 'GET') return jsonResponse(adminPaginate(state.platformExpenses, url));
  if (path === '/admin/finance/journals' && method === 'GET') return jsonResponse(adminPaginate(state.platformJournals, url));
  if (path === '/admin/finance/expenses' && method === 'POST') {
    const expense = { ...state.platformExpenses[0], ...body, id: nextDemoID(state, 'expense'), status: 'DRAFT', created_at: '2026-08-01T10:00:00.000Z' };
    state.platformExpenses.unshift(expense);
    savePortfolioDemoState(state);
    return jsonResponse(expense, 201);
  }
  const expenseAction = path.match(/^\/admin\/finance\/expenses\/([^/]+)\/(cancel|approve|post|void)$/);
  if (expenseAction && method === 'POST') {
    const expense = state.platformExpenses.find((item) => item.id === expenseAction[1]);
    if (!expense) return jsonResponse({ message: 'Expense demo tidak ditemukan' }, 404);
    const statusByAction: Record<string, string> = { cancel: 'CANCELLED', approve: 'APPROVED', post: 'POSTED', void: 'VOID' };
    expense.status = statusByAction[expenseAction[2]];
    if (expenseAction[2] === 'cancel') expense.cancel_reason = body.reason;
    if (expenseAction[2] === 'void') expense.void_reason = body.reason;
    savePortfolioDemoState(state);
    return jsonResponse(expense);
  }

  return jsonResponse({
    code: 'PORTFOLIO_DEMO_UNAVAILABLE',
    message: `Fitur ${method} ${path} belum tersedia pada Portfolio Demo Mode. Tidak ada request yang dikirim ke backend.`,
  }, 501);
}

let installed = false;

export function installPortfolioDemoFetch(): void {
  if (!isPortfolioDemo || installed) return;
  installed = true;
  loadPortfolioDemoState();
  globalThis.fetch = routePortfolioDemoRequest as typeof globalThis.fetch;
}

export const portfolioDemoFetchForTest = routePortfolioDemoRequest;
