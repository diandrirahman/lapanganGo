import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { MemoryRouter } from 'react-router-dom';
import { AdminPlatformFinancePage } from '../pages/admin/AdminPlatformFinancePage';
import contractPayload from '../test-fixtures/platformFinanceApiUiContract.json';
import type { PlatformFinanceSummaryResponse } from '../types/platformFinance';

const platformFinanceApiUiSummary = contractPayload as PlatformFinanceSummaryResponse;

const jsonResponse = (body: unknown): Response => new Response(JSON.stringify(body), {
  status: 200,
  headers: { 'Content-Type': 'application/json' },
});

describe('platform finance API to UI contract', () => {
  beforeEach(() => {
    localStorage.setItem('auth_token', 'contract-test-token');
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes('/admin/finance/summary')) return jsonResponse(platformFinanceApiUiSummary);
      if (url.includes('/owners')) return jsonResponse({ data: [], total_pages: 0, total_items: 0, page: 1, limit: 25 });
      if (url.includes('/venues')) return jsonResponse({ data: [], total_pages: 0, total_items: 0, page: 1, limit: 100 });
      throw new Error(`Unexpected contract request: ${url}`);
    }));
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  it('renders exact serialized API values and preserves unavailable actual metrics', async () => {
    render(<MemoryRouter initialEntries={['/admin/finance?start_date=2026-06-01&end_date=2026-06-02']}><AdminPlatformFinancePage /></MemoryRouter>);

    expect((await screen.findByTestId('platform-opex-value')).textContent).toContain('Rp 125.000');
    expect(screen.getByText('Projected Operating Result').parentElement?.textContent).toContain('Rp -118.000');
    expect(screen.getByText('Pendapatan Aktual').parentElement?.textContent).toContain('Belum tersedia');
    expect(screen.getByText('Transaction Contribution').parentElement?.textContent).toContain('Belum tersedia');
    expect(screen.getByText('Operating Result Aktual').parentElement?.textContent).toContain('Belum tersedia');

    const fetchMock = vi.mocked(fetch);
    const summaryCall = fetchMock.mock.calls.find(([input]) => String(input).includes('/admin/finance/summary?start_date=2026-06-01&end_date=2026-06-02'));
    expect(summaryCall).toBeDefined();
    const summaryInit = summaryCall ? summaryCall[1] : undefined;
    expect(summaryInit?.headers).toEqual({ Authorization: 'Bearer contract-test-token' });
  });
});
