import type { User } from '../types/auth';
import type { Booking } from '../types/booking';
import type { OpenMatch, ParticipantResponse } from '../types/mabar';
import type { Promo } from '../types/promo';
import type { StaffMember } from '../types/staff';
import type { Venue, Court, OperatingHour, BlockedSlot } from '../types/venue';
import { PORTFOLIO_DEMO_SESSION_KEY, PORTFOLIO_DEMO_STATE_KEY } from './config';

export interface PortfolioDemoState {
  version: 1;
  users: User[];
  owners: Array<Record<string, any>>;
  venues: Venue[];
  courts: Court[];
  operatingHours: OperatingHour[];
  blockedSlots: BlockedSlot[];
  bookings: Booking[];
  matches: OpenMatch[];
  participants: Record<string, ParticipantResponse[]>;
  promos: Promo[];
  refunds: Array<Record<string, any>>;
  staff: StaffMember[];
  notifications: Array<Record<string, any>>;
  ownerTransactions: Array<Record<string, any>>;
  platformExpenses: Array<Record<string, any>>;
  platformJournals: Array<Record<string, any>>;
  auditLogs: Array<Record<string, any>>;
  sequence: number;
}

const CREATED_AT = '2026-08-01T08:00:00.000Z';
const UPDATED_AT = '2026-08-01T09:00:00.000Z';
const DEMO_DATE = '2030-06-15';
const allOwnerPermissions = [
  'BOOKINGS_READ', 'BOOKINGS_WRITE', 'PAYMENT_VERIFY', 'OFFLINE_BOOKINGS_CREATE',
  'VENUES_READ', 'VENUES_WRITE', 'COURTS_READ', 'COURTS_WRITE', 'SCHEDULE_READ',
  'SCHEDULE_WRITE', 'BLOCKED_SLOTS_READ', 'BLOCKED_SLOTS_WRITE', 'FINANCE_READ',
  'FINANCE_WRITE', 'REFUNDS_READ', 'REFUNDS_WRITE', 'PROMOS_READ', 'PROMOS_WRITE',
  'ANALYTICS_READ',
];

const sports = [
  { id: 'sport-badminton', name: 'Badminton' },
  { id: 'sport-futsal', name: 'Futsal' },
  { id: 'sport-basket', name: 'Basket' },
];

const facilities = [
  { id: 'facility-parking', name: 'Parkir' },
  { id: 'facility-shower', name: 'Shower' },
  { id: 'facility-canteen', name: 'Kantin' },
];

const makeUser = (index: number, role: string): User => ({
  id: `user-${role.toLowerCase().replace('_', '-')}-${String(index).padStart(3, '0')}`,
  name: role === 'SUPER_ADMIN' ? 'Demo Superadmin' : `Demo ${role === 'OWNER' ? 'Owner' : 'Customer'} ${String(index).padStart(2, '0')}`,
  email: role === 'SUPER_ADMIN' ? 'superadmin@demo.lapanggo.test' : `${role.toLowerCase()}.${String(index).padStart(3, '0')}@demo.lapanggo.test`,
  phone: `08120000${String(index).padStart(4, '0')}`,
  role,
  status: 'ACTIVE',
  created_at: CREATED_AT,
});

export function createInitialPortfolioDemoState(): PortfolioDemoState {
  const superadmin = makeUser(1, 'SUPER_ADMIN');
  const owners = Array.from({ length: 20 }, (_, index) => {
    const number = index + 1;
    return {
      id: `owner-profile-${String(number).padStart(3, '0')}`,
      user_id: `user-owner-${String(number).padStart(3, '0')}`,
      business_name: `LapangGo Demo Sports ${String(number).padStart(2, '0')}`,
      status: number % 7 === 0 ? 'SUSPENDED' : 'ACTIVE',
      phone_number: `021555${String(number).padStart(4, '0')}`,
      bank_name: 'BANK DEMO',
      bank_account_number: `000${String(number).padStart(7, '0')}`,
      bank_account_name: `Demo Owner ${String(number).padStart(2, '0')}`,
      created_at: CREATED_AT,
      updated_at: UPDATED_AT,
    };
  });
  const ownerUsers = Array.from({ length: 20 }, (_, index) => {
    const user = makeUser(index + 1, 'OWNER');
    const owner = owners[index];
    return {
      ...user,
      owner_profile: {
        id: String(owner.id),
        name: String(owner.business_name),
      },
    };
  });
  const customerUsers = Array.from({ length: 100 }, (_, index) => makeUser(index + 1, 'CUSTOMER'));

  const venueNames = [
    ['Arena Senayan Demo', 'Jakarta Pusat'],
    ['Smash Hub Bintaro', 'Tangerang Selatan'],
    ['Court Kemang Portfolio', 'Jakarta Selatan'],
    ['Futsal Center Tebet', 'Jakarta Selatan'],
    ['Sports Hall Depok', 'Depok'],
    ['Mini Arena Bekasi', 'Bekasi'],
  ];
  const venues: Venue[] = venueNames.map(([name, city], index) => ({
    id: `venue-${index + 1}`,
    owner_profile_id: `owner-profile-${String((index % 3) + 1).padStart(3, '0')}`,
    name,
    description: 'Venue sintetis untuk demonstrasi portofolio LapangGo.',
    address: `Jalan Demo Olahraga No. ${index + 1}`,
    district: 'Kecamatan Demo',
    city,
    province: 'Indonesia',
    status: index === 5 ? 'SUSPENDED' : 'ACTIVE',
    primary_photo: '/hero-basketball.webp',
    public_photos: ['/hero-basketball.webp'],
    facilities,
    has_promo: index < 2,
    promos: index < 2 ? [{
      id: `promo-${index + 1}`,
      code: index === 0 ? 'PORTFOLIO20' : 'MABAR10',
      name: 'Promo Demo',
      discount_type: 'PERCENTAGE',
      discount_value: index === 0 ? 20 : 10,
      starts_at: '2026-01-01T00:00:00.000Z',
      ends_at: '2035-12-31T23:59:59.000Z',
    }] : [],
    created_at: CREATED_AT,
    updated_at: UPDATED_AT,
  }));

  const courts: Court[] = venues.flatMap((venue, venueIndex) => [0, 1].map((courtIndex) => ({
    id: `court-${venueIndex + 1}-${courtIndex + 1}`,
    venue_id: venue.id,
    sport: sports[(venueIndex + courtIndex) % sports.length],
    name: courtIndex === 0 ? 'Lapangan Utama' : 'Lapangan Premium',
    description: 'Lapangan demo dengan data sintetis.',
    location_type: courtIndex === 0 ? 'INDOOR' : 'OUTDOOR',
    surface_type: courtIndex === 0 ? 'VINYL' : 'SYNTHETIC_GRASS',
    price_per_hour: 100000 + (venueIndex * 25000) + (courtIndex * 50000),
    status: 'ACTIVE',
    created_at: CREATED_AT,
    updated_at: UPDATED_AT,
  })));

  const operatingHours: OperatingHour[] = courts.flatMap((court) => Array.from({ length: 7 }, (_, day) => ({
    id: `hours-${court.id}-${day}`,
    court_id: court.id,
    day_of_week: day,
    open_time: day === 0 ? '07:00' : '08:00',
    close_time: day === 0 ? '23:00' : '22:00',
    is_closed: false,
  })));

  const blockedSlots: BlockedSlot[] = [{
    id: 'blocked-slot-1',
    court_id: 'court-1-1',
    start_at: `${DEMO_DATE}T12:00:00+07:00`,
    end_at: `${DEMO_DATE}T14:00:00+07:00`,
    reason: 'Perawatan rutin demo',
    created_at: CREATED_AT,
    updated_at: UPDATED_AT,
  }];

  const bookings: Booking[] = Array.from({ length: 12 }, (_, index) => {
    const court = courts[index % courts.length];
    const venue = venues.find((item) => item.id === court.venue_id)!;
    const statuses: Booking['status'][] = ['PENDING_PAYMENT', 'WAITING_VERIFICATION', 'PAID', 'CONFIRMED', 'COMPLETED', 'CANCELLED'];
    return {
      id: `booking-${index + 1}`,
      customer_id: `user-customer-${String((index % 8) + 1).padStart(3, '0')}`,
      venue: { id: venue.id, name: venue.name, address: venue.address, city: venue.city },
      court: { id: court.id, name: court.name, sport_name: court.sport.name },
      court_id: court.id,
      booking_date: `2030-06-${String(15 + (index % 10)).padStart(2, '0')}`,
      start_time: `${String(9 + (index % 8)).padStart(2, '0')}:00`,
      end_time: `${String(10 + (index % 8)).padStart(2, '0')}:00`,
      original_price: court.price_per_hour,
      discount_amount: index === 0 ? 20000 : 0,
      final_price: court.price_per_hour - (index === 0 ? 20000 : 0),
      total_price: court.price_per_hour - (index === 0 ? 20000 : 0),
      status: statuses[index % statuses.length],
      payment_reference: index < 3 ? `DEMO-PAY-${String(index + 1).padStart(4, '0')}` : undefined,
      expires_at: '2030-06-15T10:30:00.000Z',
      created_at: CREATED_AT,
      updated_at: UPDATED_AT,
    };
  });

  const matches: OpenMatch[] = Array.from({ length: 4 }, (_, index) => ({
    id: `match-${index + 1}`,
    booking_id: `booking-${index + 3}`,
    host_user_id: `user-customer-${String(index + 2).padStart(3, '0')}`,
    host_name: `Demo Customer ${String(index + 2).padStart(2, '0')}`,
    title: ['Badminton Santai', 'Futsal After Office', 'Basket Weekend', 'Mabar Semua Level'][index],
    description: 'Pertandingan sintetis untuk demo portofolio.',
    sport_name: sports[index % sports.length].name,
    venue_name: venues[index].name,
    court_name: courts[index * 2].name,
    match_date: `2030-06-${18 + index}`,
    start_time: '19:00',
    end_time: '21:00',
    level: index === 0 ? 'Beginner' : 'All Levels',
    max_players: 10,
    joined_count: 3 + index,
    remaining_slots: 7 - index,
    price_per_player: 35000 + (index * 5000),
    status: index === 3 ? 'FULL' : 'OPEN',
    created_at: CREATED_AT,
    updated_at: UPDATED_AT,
  }));

  const participants = Object.fromEntries(matches.map((match, matchIndex) => [
    match.id,
    Array.from({ length: match.joined_count }, (_, index) => ({
      id: `participant-${matchIndex + 1}-${index + 1}`,
      user_id: `user-customer-${String(index + 10).padStart(3, '0')}`,
      name: `Demo Participant ${index + 1}`,
      status: 'JOINED',
      joined_at: CREATED_AT,
    })),
  ]));

  const promos: Promo[] = [{
    id: 'promo-1', owner_id: 'owner-profile-001', venue_id: 'venue-1', code: 'PORTFOLIO20',
    name: 'Portfolio Launch', description: 'Promo sintetis untuk demo.', discount_type: 'PERCENTAGE',
    discount_value: 20, starts_at: '2026-01-01T00:00:00.000Z', ends_at: '2035-12-31T23:59:59.000Z',
    status: 'ACTIVE', created_at: CREATED_AT, updated_at: UPDATED_AT, usage_count: 18,
    total_discount_amount: 360000, total_final_revenue: 1440000, can_delete: false,
  }];

  const staff: StaffMember[] = [{
    id: 'staff-1', owner_profile_id: 'owner-profile-001', user_id: 'user-staff-001',
    name: 'Demo Venue Manager', email: 'staff.manager@demo.lapanggo.test', phone: '081299990001',
    role: 'MANAGER', permissions: allOwnerPermissions, status: 'ACTIVE', invitation_status: 'ACTIVE',
    invited_at: CREATED_AT, activated_at: UPDATED_AT, venue_ids: ['venue-1', 'venue-4'],
    created_at: CREATED_AT, updated_at: UPDATED_AT,
  }];

  const refunds = [{
    id: 'refund-1', booking_id: 'booking-6', customer_id: 'user-customer-006', owner_id: 'owner-profile-001',
    customer_name: 'Demo Customer 06', customer_email: 'customer.006@demo.lapanggo.test', venue_id: 'venue-1',
    venue_name: venues[0].name, court_name: courts[0].name, booking_date: DEMO_DATE,
    start_time: '14:00', end_time: '15:00', amount: 100000, reason: 'Jadwal berubah', status: 'PENDING',
    requested_at: CREATED_AT, created_at: CREATED_AT, updated_at: UPDATED_AT,
  }];

  const ownerTransactions = [
    { id: 'transaction-1', owner_id: 'owner-profile-001', venue_id: 'venue-1', type: 'INCOME', source: 'BOOKING', category: 'BOOKING', amount: 850000, transaction_date: DEMO_DATE, description: 'Pendapatan booking demo', created_at: CREATED_AT, updated_at: UPDATED_AT },
    { id: 'transaction-2', owner_id: 'owner-profile-001', venue_id: 'venue-1', type: 'EXPENSE', source: 'MAINTENANCE', category: 'MAINTENANCE', amount: 125000, transaction_date: DEMO_DATE, description: 'Perawatan demo', created_at: CREATED_AT, updated_at: UPDATED_AT },
  ];

  const platformExpenses = [{
    id: 'expense-1', category: 'INFRASTRUCTURE', vendor: 'Demo Cloud Vendor', amount_rupiah: '450000', currency: 'IDR',
    occurred_at: CREATED_AT, payment_account: 'ACCOUNTS_PAYABLE', external_reference: 'DEMO-EXP-001',
    description: 'Biaya infrastruktur sintetis', status: 'DRAFT', posted_journal_id: null, void_journal_id: null,
    created_by_user_id: superadmin.id, approved_by_user_id: null, posted_by_user_id: null, voided_by_user_id: null,
    cancelled_by_user_id: null, cancel_reason: null, void_reason: null, created_at: CREATED_AT, approved_at: null,
    posted_at: null, voided_at: null, cancelled_at: null,
  }];

  const platformJournals = [{
    id: 'journal-1', event_key: 'DEMO|BOOKING|001', event_type: 'BOOKING_CAPTURED', booking_id: 'booking-3',
    owner_profile_id: 'owner-profile-001', venue_id: 'venue-1', currency: 'IDR', effective_at: CREATED_AT,
    posted_at: UPDATED_AT, reverses_journal_id: null, reversal_reason: null, reversed_by_journal_id: null,
    entry_count: 2, debit_total_rupiah: '100000', credit_total_rupiah: '100000',
  }];

  const auditLogs = Array.from({ length: 12 }, (_, index) => ({
    id: `audit-${index + 1}`, scope: index % 2 === 0 ? 'OWNER' : 'PLATFORM', owner_profile_id: index % 2 === 0 ? 'owner-profile-001' : undefined,
    actor_user_id: index % 2 === 0 ? ownerUsers[0].id : superadmin.id, actor_role: index % 2 === 0 ? 'OWNER' : 'SUPER_ADMIN',
    actor: { id: index % 2 === 0 ? ownerUsers[0].id : superadmin.id, name: index % 2 === 0 ? ownerUsers[0].name : superadmin.name, role: index % 2 === 0 ? 'OWNER' : 'SUPER_ADMIN' },
    action: ['BOOKING_UPDATED', 'VENUE_STATUS_UPDATED', 'PROMO_CREATED'][index % 3], entity_type: ['BOOKING', 'VENUE', 'PROMO'][index % 3],
    entity_id: `demo-entity-${index + 1}`, venue_id: index % 2 === 0 ? 'venue-1' : undefined,
    metadata: { demo: true, summary: 'Aktivitas sintetis portofolio' }, created_at: CREATED_AT,
  }));

  return {
    version: 1,
    users: [superadmin, ...ownerUsers, ...customerUsers],
    owners,
    venues,
    courts,
    operatingHours,
    blockedSlots,
    bookings,
    matches,
    participants,
    promos,
    refunds,
    staff,
    notifications: [{ id: 'notification-1', type: 'BOOKING', title: 'Booking demo baru', message: 'Satu booking sintetis menunggu ditinjau.', entity_type: 'BOOKING', entity_id: 'booking-2', read_at: null, created_at: CREATED_AT }],
    ownerTransactions,
    platformExpenses,
    platformJournals,
    auditLogs,
    sequence: 1000,
  };
}

export function loadPortfolioDemoState(): PortfolioDemoState {
  try {
    const stored = localStorage.getItem(PORTFOLIO_DEMO_STATE_KEY);
    if (stored) {
      const parsed = JSON.parse(stored) as PortfolioDemoState;
      if (parsed.version === 1) return parsed;
    }
  } catch {
    // Corrupt demo state is replaced with the deterministic baseline.
  }
  const state = createInitialPortfolioDemoState();
  savePortfolioDemoState(state);
  return state;
}

export function savePortfolioDemoState(state: PortfolioDemoState): void {
  localStorage.setItem(PORTFOLIO_DEMO_STATE_KEY, JSON.stringify(state));
}

export function resetPortfolioDemoState(): PortfolioDemoState {
  const state = createInitialPortfolioDemoState();
  savePortfolioDemoState(state);
  localStorage.removeItem(PORTFOLIO_DEMO_SESSION_KEY);
  localStorage.removeItem('auth_token');
  return state;
}

export function nextDemoID(state: PortfolioDemoState, prefix: string): string {
  state.sequence += 1;
  return `${prefix}-${state.sequence}`;
}

export function portfolioDemoUserForRole(state: PortfolioDemoState, role: string): User | undefined {
  return state.users.find((user) => user.role === role);
}

export function startPortfolioDemoSession(role: string): { token: string; user: User } {
  const state = loadPortfolioDemoState();
  const user = portfolioDemoUserForRole(state, role);
  if (!user) throw new Error(`Demo role ${role} tidak tersedia`);

  const token = `portfolio-demo:${role}`;
  localStorage.setItem(PORTFOLIO_DEMO_SESSION_KEY, JSON.stringify({ user_id: user.id, role }));
  return { token, user };
}

export const portfolioDemoSports = sports;
export const portfolioDemoFacilities = facilities;
export const portfolioDemoOwnerPermissions = allOwnerPermissions;
