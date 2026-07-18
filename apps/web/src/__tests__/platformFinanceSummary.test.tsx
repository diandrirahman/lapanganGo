import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { MemoryRouter, useNavigate } from 'react-router-dom';
import { AdminPlatformFinancePage } from '../pages/admin/AdminPlatformFinancePage';
import { adminApi } from '../lib/api/admin';
import { chartIntegerPercent } from '../lib/platformFinance';
import type { PlatformFinanceSummaryResponse } from '../types/platformFinance';

vi.mock('../lib/api/admin', () => ({
  adminApi: {
    getOwners: vi.fn().mockResolvedValue({ data: [] }),
    getVenues: vi.fn().mockResolvedValue({ data: [] }),
    getPlatformFinanceSummary: vi.fn(),
  },
}));

const summary = (opex: string, projectedResult: string | null): PlatformFinanceSummaryResponse => ({
  period: { start_date: '2026-06-01', end_date: '2026-06-02' },
  mode: 'SIMULATION', currency: 'IDR', timezone: 'Asia/Jakarta',
  generated_at: '2026-06-02T10:00:00+07:00', as_of: '2026-06-02T10:00:00+07:00', granularity: 'day',
  metrics: {
    online_gmv_gross: '100000', refund_principal: '0', online_gmv_net: '100000', projected_commission: '7000',
    projected_owner_net_after_hypothetical_commission: '93000', realized_online_booking_count: 1, refunded_booking_count: 0,
    legacy_manual_realized_gmv: '100000', gateway_captured_gmv: null, actual_commission_revenue: null,
    payment_processing_expense: null, platform_operating_expense: opex,
    projected_operating_result_before_transaction_costs: projectedResult, platform_revenue: null,
    transaction_contribution: null, operating_result: null,
  },
  data_availability: {
    platform_operating_expense: 'AVAILABLE', actual_platform_revenue: 'UNAVAILABLE_UNTIL_LIVE',
    payment_processing_expense: 'UNAVAILABLE_UNTIL_GATEWAY', owner_payable: 'UNAVAILABLE_UNTIL_PLATFORM_COLLECTED',
  },
  trend: [
    { period_start: '2026-06-01', period_end: '2026-06-01', online_gmv_gross: '100000', refund_principal: '0', online_gmv_net: '100000', projected_commission: '7000', platform_operating_expense: opex },
    { period_start: '2026-06-02', period_end: '2026-06-02', online_gmv_gross: '0', refund_principal: '0', online_gmv_net: '0', projected_commission: '0', platform_operating_expense: '0' },
  ],
  caveats: ['Proyeksi komisi bukan pendapatan aktual.'],
});

describe('AdminPlatformFinancePage OPEX summary contract', () => {
  beforeEach(() => {
    localStorage.setItem('auth_token', 'test-token');
    vi.mocked(adminApi.getPlatformFinanceSummary).mockResolvedValue(summary('0', '7000'));
  });

  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
    localStorage.clear();
  });

  it('keeps large integer OPEX visible in the normalized trend scale', () => {
    const large = '900719925474099200000';
    expect(chartIntegerPercent(large, BigInt(large))).toBe(100);
    expect(chartIntegerPercent('1', BigInt(large))).toBeGreaterThanOrEqual(1);
  });

  it('renders available zero as Rp 0 and keeps actual metrics unavailable', async () => {
    render(<MemoryRouter><AdminPlatformFinancePage /></MemoryRouter>);
    expect((await screen.findByTestId('platform-opex-value')).textContent).toContain('Rp 0');
    expect(screen.getByText('Pendapatan Aktual').parentElement?.textContent).toContain('Belum tersedia');
    expect(screen.getByText('Transaction Contribution').parentElement?.textContent).toContain('Belum tersedia');
    expect(screen.getByText('Operating Result Aktual').parentElement?.textContent).toContain('Belum tersedia');
    expect(screen.getByTestId('platform-finance-trend').textContent).toContain('Rp 0');
  });

  it('passes period and granularity filters to the summary endpoint', async () => {
    vi.mocked(adminApi.getPlatformFinanceSummary)
      .mockResolvedValueOnce(summary('10000', '-3000'))
      .mockResolvedValue(summary('20000', '-13000'));
    render(<MemoryRouter><AdminPlatformFinancePage /></MemoryRouter>);
    await screen.findByTestId('platform-opex-value');
    fireEvent.change(screen.getByLabelText('Granularity'), { target: { value: 'month' } });
    fireEvent.change(screen.getByLabelText('Mulai'), { target: { value: '2026-05-01' } });
    await waitFor(() => expect(vi.mocked(adminApi.getPlatformFinanceSummary).mock.calls.some(([params]) => params?.granularity === 'month' && params.start_date === '2026-05-01')).toBe(true));
    expect(screen.getByTestId('platform-opex-value').textContent).toContain('Rp 20.000');
    expect(screen.getByText('Projected Operating Result').parentElement?.textContent).toContain('Rp -13.000');
  });

  it('marks the previous summary as stale while a new filter request is pending', async () => {
    let resolveNext: (value: PlatformFinanceSummaryResponse) => void = () => undefined;
    const pendingSummary = new Promise<PlatformFinanceSummaryResponse>((resolve) => { resolveNext = resolve; });
    vi.mocked(adminApi.getPlatformFinanceSummary)
      .mockResolvedValueOnce(summary('10000', '-3000'))
      .mockReturnValueOnce(pendingSummary);

    render(<MemoryRouter><AdminPlatformFinancePage /></MemoryRouter>);
    await screen.findByTestId('platform-opex-value');
    fireEvent.change(screen.getByLabelText('Granularity'), { target: { value: 'month' } });

    expect((await screen.findByTestId('platform-finance-stale')).textContent).toContain('ringkasan yang tampil masih berasal dari request sebelumnya');
    resolveNext(summary('20000', '-13000'));
    await waitFor(() => expect(screen.queryByTestId('platform-finance-stale')).toBeNull());
  });

  it('keeps the newest filter result when an older request resolves late', async () => {
    let resolveOlder: (value: PlatformFinanceSummaryResponse) => void = () => undefined;
    let resolveNewest: (value: PlatformFinanceSummaryResponse) => void = () => undefined;
    const olderRequest = new Promise<PlatformFinanceSummaryResponse>((resolve) => { resolveOlder = resolve; });
    const newestRequest = new Promise<PlatformFinanceSummaryResponse>((resolve) => { resolveNewest = resolve; });
    vi.mocked(adminApi.getPlatformFinanceSummary)
      .mockResolvedValueOnce(summary('10000', '-3000'))
      .mockReturnValueOnce(olderRequest)
      .mockReturnValueOnce(newestRequest);

    render(<MemoryRouter><AdminPlatformFinancePage /></MemoryRouter>);
    await screen.findByTestId('platform-opex-value');
    fireEvent.change(screen.getByLabelText('Granularity'), { target: { value: 'month' } });
    fireEvent.change(screen.getByLabelText('Mulai'), { target: { value: '2026-05-01' } });
    await waitFor(() => expect(vi.mocked(adminApi.getPlatformFinanceSummary).mock.calls.length).toBeGreaterThanOrEqual(3));

    resolveNewest(summary('30000', '-27000'));
    await waitFor(() => expect(screen.getByTestId('platform-opex-value').textContent).toContain('Rp 30.000'));
    resolveOlder(summary('12000', '-18000'));
    await waitFor(() => expect(screen.getByTestId('platform-opex-value').textContent).toContain('Rp 30.000'));
    expect(screen.getByText('Projected Operating Result').parentElement?.textContent).toContain('Rp -27.000');
  });

  it('does not display a mixed-scope projected result when an owner filter is active', async () => {
    vi.mocked(adminApi.getPlatformFinanceSummary).mockResolvedValue(summary('10000', null));
    render(<MemoryRouter initialEntries={['/admin/finance?owner_profile_id=00000000-0000-0000-0000-000000000001']}><AdminPlatformFinancePage /></MemoryRouter>);
    await screen.findByTestId('platform-opex-value');
    expect(screen.getByText('Projected Operating Result').parentElement?.textContent).toContain('Belum tersedia');
    expect(screen.getByRole('status').textContent).toContain('OPEX belum memiliki alokasi per scope');
  });

  it('keeps mobile and desktop responsive layout contracts explicit', async () => {
    render(<MemoryRouter><AdminPlatformFinancePage /></MemoryRouter>);
    await screen.findByTestId('platform-opex-value');

    const filterSection = screen.getByText('Filter laporan').closest('section');
    const filterGrid = filterSection?.querySelector('div.grid');
    expect(filterGrid?.className).toContain('sm:grid-cols-2');
    expect(filterGrid?.className).toContain('lg:grid-cols-5');
    expect(screen.getByTestId('platform-finance-trend').querySelector('div.overflow-x-auto')).not.toBeNull();
  });

  it('uses URL state for browser history navigation', async () => {
    const NavigationHarness = () => {
      const navigate = useNavigate();
      return <><button type="button" onClick={() => navigate('/admin/finance?granularity=month')}>Go month</button><button type="button" onClick={() => navigate(-1)}>Go back</button></>;
    };
    render(<MemoryRouter initialEntries={['/admin/finance?granularity=day']}><NavigationHarness /><AdminPlatformFinancePage /></MemoryRouter>);
    await screen.findByTestId('platform-opex-value');
    expect((screen.getByLabelText('Granularity') as HTMLSelectElement).value).toBe('day');
    fireEvent.click(screen.getByRole('button', { name: 'Go month' }));
    await waitFor(() => expect((screen.getByLabelText('Granularity') as HTMLSelectElement).value).toBe('month'));
    fireEvent.click(screen.getByRole('button', { name: 'Go back' }));
    await waitFor(() => expect((screen.getByLabelText('Granularity') as HTMLSelectElement).value).toBe('day'));
  });

  it('queries owners with server-side pagination beyond the first 100 records', async () => {
    vi.mocked(adminApi.getOwners).mockResolvedValue({ data: [], total_pages: 4, total_items: 101, page: 1, limit: 25 });
    render(<MemoryRouter><AdminPlatformFinancePage /></MemoryRouter>);
    await waitFor(() => expect(vi.mocked(adminApi.getOwners).mock.calls.some(([params]) => params?.page === 1 && params.limit === 25)).toBe(true));
    fireEvent.click(screen.getByRole('button', { name: 'Next owner page' }));
    await waitFor(() => expect(vi.mocked(adminApi.getOwners).mock.calls.some(([params]) => params?.page === 2 && params.limit === 25)).toBe(true));
  });

  it('surfaces owner option failures and retries the request', async () => {
    vi.mocked(adminApi.getOwners)
      .mockRejectedValueOnce(new Error('owner service unavailable'))
      .mockResolvedValue({ data: [], total_pages: 1, total_items: 0, page: 1, limit: 25 });
    render(<MemoryRouter><AdminPlatformFinancePage /></MemoryRouter>);

    await waitFor(() => expect(screen.getByTestId('platform-finance-options-error').textContent).toContain('Daftar owner tidak dapat dimuat'));
    fireEvent.click(screen.getByRole('button', { name: 'Retry finance filter options' }));
    await waitFor(() => expect(vi.mocked(adminApi.getOwners).mock.calls.length).toBeGreaterThanOrEqual(2));
    await waitFor(() => expect(screen.queryByTestId('platform-finance-options-error')).toBeNull());
  });

  it('surfaces venue option failures and retries the request', async () => {
    vi.mocked(adminApi.getVenues)
      .mockRejectedValueOnce(new Error('venue service unavailable'))
      .mockResolvedValue({ data: [], total_pages: 1, total_items: 0, page: 1, limit: 100 });
    render(<MemoryRouter><AdminPlatformFinancePage /></MemoryRouter>);

    await waitFor(() => expect(screen.getByTestId('platform-finance-options-error').textContent).toContain('Daftar venue tidak dapat dimuat'));
    fireEvent.click(screen.getByRole('button', { name: 'Retry finance filter options' }));
    await waitFor(() => expect(vi.mocked(adminApi.getVenues).mock.calls.length).toBeGreaterThanOrEqual(2));
    await waitFor(() => expect(screen.queryByTestId('platform-finance-options-error')).toBeNull());
  });
});
