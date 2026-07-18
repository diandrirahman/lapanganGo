import { spawn } from 'node:child_process';
import { readFile } from 'node:fs/promises';
import { existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { chromium } from 'playwright-core';

const webRoot = path.resolve(fileURLToPath(new URL('..', import.meta.url)));
const viteEntry = path.join(webRoot, 'node_modules', 'vite', 'bin', 'vite.js');
const port = 4174;
const appUrl = `http://127.0.0.1:${port}`;
const financeUrl = `${appUrl}/admin/finance?start_date=2026-06-01&end_date=2026-06-02`;
const browserExecutableCandidates = [
  process.env.LAPANGGO_CHROME_PATH,
  'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe',
  'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe',
  '/usr/bin/google-chrome',
  '/usr/bin/chromium',
  '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
].filter((candidate) => candidate && existsSync(candidate));
const fixturePath = path.join(webRoot, 'src', 'test-fixtures', 'platformFinanceApiUiContract.json');
const expenseFixture = {
  data: [{
    id: '11111111-1111-4111-8111-111111111111',
    category: 'OFFICE_ADMIN',
    vendor: 'Responsive QA Posted Vendor',
    amount_rupiah: '125000',
    currency: 'IDR',
    occurred_at: '2026-06-01T03:00:00Z',
    payment_account: 'FUNDING_CLEARING',
    external_reference: 'RESPONSIVE-POSTED-001',
    description: 'Responsive posted expense workflow fixture',
    status: 'POSTED',
    posted_journal_id: '33333333-3333-4333-8333-333333333333',
    void_journal_id: null,
    created_by_user_id: 'qa-super-admin',
    approved_by_user_id: 'qa-super-admin',
    posted_by_user_id: 'qa-super-admin',
    voided_by_user_id: null,
    cancelled_by_user_id: null,
    cancel_reason: null,
    void_reason: null,
    created_at: '2026-06-01T03:00:00Z',
    approved_at: '2026-06-01T03:01:00Z',
    posted_at: '2026-06-01T03:02:00Z',
    voided_at: null,
    cancelled_at: null,
  }, {
    id: '22222222-2222-4222-8222-222222222222',
    category: 'OFFICE_ADMIN',
    vendor: 'Responsive QA Voided Vendor',
    amount_rupiah: '125000',
    currency: 'IDR',
    occurred_at: '2026-06-01T03:00:00Z',
    payment_account: 'FUNDING_CLEARING',
    external_reference: 'RESPONSIVE-VOID-001',
    description: 'Responsive voided expense workflow fixture',
    status: 'VOID',
    posted_journal_id: '44444444-4444-4444-8444-444444444444',
    void_journal_id: '55555555-5555-4555-8555-555555555555',
    created_by_user_id: 'qa-super-admin',
    approved_by_user_id: 'qa-super-admin',
    posted_by_user_id: 'qa-super-admin',
    voided_by_user_id: 'qa-super-admin',
    cancelled_by_user_id: null,
    cancel_reason: null,
    void_reason: 'responsive QA reversal',
    created_at: '2026-06-01T03:00:00Z',
    approved_at: '2026-06-01T03:01:00Z',
    posted_at: '2026-06-01T03:02:00Z',
    voided_at: '2026-06-01T03:03:00Z',
    cancelled_at: null,
  }],
  page: 1,
  limit: 20,
  total_items: 2,
  total_pages: 1,
};
const journalFixture = {
  data: [{
    id: '55555555-5555-4555-8555-555555555555',
    event_key: 'journal.reversed:44444444-4444-4444-8444-444444444444',
    event_type: 'PLATFORM_FINANCE_JOURNAL_REVERSED',
    booking_id: null,
    owner_profile_id: null,
    venue_id: null,
    currency: 'IDR',
    effective_at: '2026-06-01T03:03:00Z',
    posted_at: '2026-06-01T03:03:00Z',
    reverses_journal_id: '44444444-4444-4444-8444-444444444444',
    reversal_reason: 'responsive QA',
    reversed_by_journal_id: null,
    entry_count: 2,
    debit_total_rupiah: '125000',
    credit_total_rupiah: '125000',
  }, {
    id: '44444444-4444-4444-8444-444444444444',
    event_key: 'expense.posted:22222222-2222-4222-8222-222222222222',
    event_type: 'PLATFORM_FINANCE_EXPENSE_POSTED',
    booking_id: null,
    owner_profile_id: null,
    venue_id: null,
    currency: 'IDR',
    effective_at: '2026-06-01T03:02:00Z',
    posted_at: '2026-06-01T03:02:00Z',
    reverses_journal_id: null,
    reversal_reason: null,
    reversed_by_journal_id: '55555555-5555-4555-8555-555555555555',
    entry_count: 2,
    debit_total_rupiah: '125000',
    credit_total_rupiah: '125000',
  }, {
    id: '33333333-3333-4333-8333-333333333333',
    event_key: 'expense.posted:11111111-1111-4111-8111-111111111111',
    event_type: 'PLATFORM_FINANCE_EXPENSE_POSTED',
    booking_id: null,
    owner_profile_id: null,
    venue_id: null,
    currency: 'IDR',
    effective_at: '2026-06-01T03:02:00Z',
    posted_at: '2026-06-01T03:02:00Z',
    reverses_journal_id: null,
    reversal_reason: null,
    reversed_by_journal_id: null,
    entry_count: 2,
    debit_total_rupiah: '125000',
    credit_total_rupiah: '125000',
  }],
  page: 1,
  limit: 20,
  total_items: 3,
  total_pages: 1,
};

const assertValidExpenseFixture = () => {
  for (const expense of expenseFixture.data) {
    if (!expense.posted_journal_id) {
      throw new Error(`invalid responsive fixture: ${expense.status} expense is missing posted_journal_id`);
    }
    if (expense.status === 'POSTED' && (expense.void_journal_id || expense.voided_at || expense.voided_by_user_id || expense.void_reason)) {
      throw new Error('invalid responsive fixture: POSTED expense contains VOID-only fields');
    }
    if (expense.status === 'VOID' && (!expense.void_journal_id || !expense.voided_at || !expense.voided_by_user_id || !expense.void_reason)) {
      throw new Error('invalid responsive fixture: VOID expense is missing reversal audit fields');
    }
  }
};

const json = (body, status = 200) => ({
  status,
  contentType: 'application/json',
  body: JSON.stringify(body),
});

const waitForServer = async () => {
  for (let attempt = 0; attempt < 50; attempt += 1) {
    try {
      const response = await fetch(appUrl);
      if (response.ok) return;
    } catch {
      // Vite is still starting.
    }
    await new Promise((resolve) => setTimeout(resolve, 100));
  }
  throw new Error(`Vite did not start at ${appUrl}`);
};

const assertNoHorizontalOverflow = async (page, viewportWidth) => {
  const measurements = await page.evaluate(() => {
    const documentWidth = document.documentElement.scrollWidth;
    const viewport = document.documentElement.clientWidth;
    const overflowing = Array.from(document.querySelectorAll('body *'))
      .map((element) => ({ element, rect: element.getBoundingClientRect() }))
      .filter(({ element, rect }) => rect.width > 0 && rect.right > viewport + 1 && element !== document.body)
      .slice(0, 5)
      .map(({ element, rect }) => ({ right: Math.round(rect.right), width: Math.round(rect.width), tag: element.tagName, className: element.className, text: (element.textContent ?? '').trim().slice(0, 80) }));
    return { documentWidth, viewport, overflowing };
  });
  if (measurements.documentWidth > viewportWidth || measurements.viewport > viewportWidth || measurements.overflowing.length > 0) {
    throw new Error(`horizontal overflow at ${viewportWidth}px: ${JSON.stringify(measurements)}`);
  }
  return measurements;
};

const run = async () => {
  const fixture = JSON.parse(await readFile(fixturePath, 'utf8'));
  assertValidExpenseFixture();
  const server = spawn(process.execPath, [viteEntry, '--host', '127.0.0.1', '--port', String(port)], {
    cwd: webRoot,
    stdio: 'ignore',
  });
  let browser;
  try {
    await waitForServer();
    if (browserExecutableCandidates.length === 0) {
      throw new Error('No Chrome/Chromium executable found; set LAPANGGO_CHROME_PATH for responsive QA.');
    }
    browser = await chromium.launch({ headless: true, executablePath: browserExecutableCandidates[0] });
    const page = await browser.newPage({ viewport: { width: 360, height: 800 } });
    await page.addInitScript(() => localStorage.setItem('auth_token', 'responsive-qa-token'));
    await page.route('http://localhost:8080/**', async (route) => {
      const method = route.request().method();
      const pathname = new URL(route.request().url()).pathname;
      if (pathname === '/auth/me') {
        await route.fulfill(json({ user: { id: 'qa-super-admin', name: 'QA SuperAdmin', email: 'qa@lapanggo.id', role: 'SUPER_ADMIN', status: 'ACTIVE', created_at: '2026-06-01T00:00:00Z' } }));
        return;
      }
      if (pathname.endsWith('/admin/finance/summary')) {
        await route.fulfill(json(fixture));
        return;
      }
      if (method === 'GET' && pathname === '/admin/finance/expenses') {
        await route.fulfill(json(expenseFixture));
        return;
      }
      if (method === 'GET' && pathname === '/admin/finance/journals') {
        await route.fulfill(json(journalFixture));
        return;
      }
      if (pathname.endsWith('/owners') || pathname.endsWith('/venues')) {
        await route.fulfill(json({ data: [], total_pages: 0, total_items: 0, page: 1, limit: 100 }));
        return;
      }
      await route.fulfill(json({ message: 'Unexpected responsive QA request' }, 404));
    });

    await page.goto(financeUrl, { waitUntil: 'networkidle' });
    await page.getByTestId('platform-opex-value').waitFor({ state: 'visible' });
    await page.getByRole('heading', { name: 'Keuangan Platform' }).waitFor({ state: 'visible' });
    const mobile = await assertNoHorizontalOverflow(page, 360);

    await page.setViewportSize({ width: 1440, height: 900 });
    const desktop = await assertNoHorizontalOverflow(page, 1440);

    const clickJournalLinkAndWait = async (name) => {
      await Promise.all([
        page.waitForResponse((response) => response.url().includes('/admin/finance/journals') && response.request().method() === 'GET' && response.ok()),
        page.getByRole('link', { name }).first().click(),
      ]);
      await page.getByText(/Showing linked journal/).waitFor({ state: 'visible' });
    };
    const assertExpenseWorkflowAtViewport = async (width, height) => {
      await page.setViewportSize({ width, height });
      await page.goto(`${appUrl}/admin/finance/expenses`, { waitUntil: 'networkidle' });
      await page.getByRole('heading', { name: 'Platform Finance' }).waitFor({ state: 'visible' });
      await page.locator('span:visible').filter({ hasText: 'POSTED' }).first().waitFor({ state: 'visible' });
      const visibleVoidButtons = page.locator('button:visible').filter({ hasText: /^Void/ });
      if (await visibleVoidButtons.count() !== 1) {
        throw new Error(`expected exactly one Void action for the POSTED fixture, found ${await visibleVoidButtons.count()}`);
      }
      const visibleReversalLinks = page.locator('a:visible').filter({ hasText: /Void reversal/ });
      if (await visibleReversalLinks.count() !== 1) {
        throw new Error(`expected exactly one Void reversal link for the VOID fixture, found ${await visibleReversalLinks.count()}`);
      }
      const list = await assertNoHorizontalOverflow(page, width);

      await Promise.all([
        page.waitForResponse((response) => response.url().includes('/admin/finance/journals') && response.request().method() === 'GET' && response.ok()),
        page.getByRole('link', { name: /Void reversal/ }).click(),
      ]);
      await page.getByText(/Showing linked journal/).waitFor({ state: 'visible' });
      await clickJournalLinkAndWait(/Reversal of/);
      await clickJournalLinkAndWait(/Reversed by/);
      const journals = await assertNoHorizontalOverflow(page, width);

      await page.goto(`${appUrl}/admin/finance/expenses`, { waitUntil: 'networkidle' });
      await page.getByRole('button', { name: 'Add expense' }).click();
      await page.getByRole('dialog', { name: 'Add platform expense' }).waitFor({ state: 'visible' });
      const createModal = await assertNoHorizontalOverflow(page, width);
      await page.getByRole('button', { name: 'Close' }).click();
      await page.getByRole('button', { name: /^Void/ }).click();
      await page.getByRole('dialog', { name: 'Void posted expense' }).waitFor({ state: 'visible' });
      const actionModal = await assertNoHorizontalOverflow(page, width);
      await page.getByRole('button', { name: 'Keep current status' }).click();
      return { list, journals, createModal, actionModal };
    };

    const expenseMobile = await assertExpenseWorkflowAtViewport(360, 800);
    const expenseDesktop = await assertExpenseWorkflowAtViewport(1440, 900);
    console.log(`platform finance responsive QA: PASS (summary 360px ${JSON.stringify(mobile)}; summary 1440px ${JSON.stringify(desktop)}; expenses 360px ${JSON.stringify(expenseMobile)}; expenses 1440px ${JSON.stringify(expenseDesktop)})`);
  } finally {
    await browser?.close();
    server.kill();
  }
};

run().catch((error) => {
  console.error(`platform finance responsive QA: FAIL\n${error.stack ?? error}`);
  process.exitCode = 1;
});
