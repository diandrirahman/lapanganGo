import { spawn, spawnSync } from 'child_process';
import { existsSync } from 'fs';
import path from 'path';
import process from 'process';
import { createServer } from 'net';
import { chromium } from 'playwright-core';
import { fileURLToPath } from 'url';

const configuredPort = Number(process.env.PLATFORM_FINANCE_QA_PORT || 4173);
if (!Number.isInteger(configuredPort) || configuredPort < 1 || configuredPort > 65535) {
  throw new Error(`Invalid PLATFORM_FINANCE_QA_PORT: ${process.env.PLATFORM_FINANCE_QA_PORT || configuredPort}`);
}

const port = configuredPort;
const host = '127.0.0.1';
const baseURL = `http://${host}:${port}`;
const apiURL = 'http://localhost:8080';
const qaEmail = 'qa@example.com';
const qaPassword = 'responsive-qa-password';
const qaToken = 'responsive-qa-token';
const qaUser = {
  id: 'qa-super-admin',
  name: 'QA SuperAdmin',
  email: qaEmail,
  role: 'SUPER_ADMIN',
  status: 'ACTIVE',
  created_at: '2026-01-01T00:00:00Z',
};

function delay(milliseconds) {
  return new Promise((resolve) => setTimeout(resolve, milliseconds));
}

function checkPortFree(candidatePort) {
  return new Promise((resolve, reject) => {
    const probe = createServer();
    let settled = false;
    const finish = (result, error = null) => {
      if (settled) return;
      settled = true;
      if (probe.listening) {
        probe.close((closeError) => {
          if (error || closeError) reject(error || closeError);
          else resolve(result);
        });
        return;
      }
      if (error) reject(error);
      else resolve(result);
    };
    probe.once('error', (error) => {
      if (error.code === 'EADDRINUSE') finish(false);
      else finish(false, error);
    });
    probe.once('listening', () => finish(true));
    probe.listen(candidatePort, host);
  });
}

async function waitForPortFree(candidatePort, timeoutMs = 10_000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (await checkPortFree(candidatePort)) return true;
    await delay(100);
  }
  return false;
}

function waitForChildExit(child, timeoutMs = 5_000) {
  if (child.exitCode !== null || child.signalCode !== null) return Promise.resolve(true);
  return new Promise((resolve) => {
    let settled = false;
    const finish = (result) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      resolve(result);
    };
    const timer = setTimeout(() => finish(false), timeoutMs);
    child.once('exit', () => finish(true));
    child.once('error', () => finish(true));
  });
}

async function stopServer(child) {
  if (!child || child.pid == null) return;

  if (process.platform === 'win32') {
    // The exact Vite node PID is owned by this test, so taskkill /T cannot
    // accidentally target a pre-existing npm/cmd process.
    spawnSync('taskkill', ['/PID', String(child.pid), '/T', '/F'], { stdio: 'ignore', windowsHide: true });
  } else {
    try {
      child.kill('SIGTERM');
    } catch {
      // The child may already have exited; the postcondition below is authoritative.
    }
    if (!(await waitForChildExit(child, 5_000))) {
      try {
        process.kill(-child.pid, 'SIGKILL');
      } catch {
        // The process group may have exited between the two checks.
      }
    }
  }

  if (!(await waitForChildExit(child, 5_000))) {
    throw new Error(`Vite process ${child.pid} did not exit`);
  }
  if (!(await waitForPortFree(port))) {
    throw new Error(`Port ${port} remained occupied after Vite cleanup`);
  }
}

function findBrowser() {
  const candidates = [
    process.env.CHROME_PATH,
    process.env.CHROMIUM_PATH,
    'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe',
    'C:\\Program Files (x86)\\Google\\Chrome\\Application\\chrome.exe',
    '/usr/bin/google-chrome',
    '/usr/bin/chromium',
    '/usr/bin/chromium-browser',
  ].filter(Boolean);
  return candidates.find((candidate) => existsSync(candidate));
}

async function waitForServer(child) {
  const deadline = Date.now() + 30_000;
  while (Date.now() < deadline) {
    if (child.exitCode !== null) throw new Error(`Vite exited before readiness with code ${child.exitCode}`);
    try {
      const response = await fetch(`${baseURL}/`);
      if (response.ok) return;
    } catch {
      // Vite is still starting.
    }
    await delay(250);
  }
  throw new Error('Timed out waiting for Vite dev server');
}

function jsonResponse(body, status = 200) {
  return { status, contentType: 'application/json', body: JSON.stringify(body) };
}

function paginated(data, totalItems = data.length) {
  return { data, total_items: totalItems, total_pages: totalItems > 0 ? 1 : 0, page: 1, limit: 25 };
}

const summary = {
  period: { start_date: '2026-01-01', end_date: '2026-01-01' },
  mode: 'SIMULATION', currency: 'IDR', timezone: 'Asia/Jakarta',
  generated_at: '2026-01-01T00:00:00Z', as_of: '2026-01-01T00:00:00Z', granularity: 'day',
  metrics: {
    online_gmv_gross: '100000', refund_principal: '0', online_gmv_net: '100000',
    projected_commission: '7000', projected_owner_net_after_hypothetical_commission: '93000',
    realized_online_booking_count: 1, refunded_booking_count: 0, legacy_manual_realized_gmv: '0',
    gateway_captured_gmv: null, actual_commission_revenue: null, payment_processing_expense: null,
    platform_operating_expense: '1000', projected_operating_result_before_transaction_costs: '6000',
    platform_revenue: null, transaction_contribution: null, operating_result: null,
  },
  data_availability: {
    platform_operating_expense: 'AVAILABLE', actual_platform_revenue: 'UNAVAILABLE',
    payment_processing_expense: 'UNAVAILABLE', owner_payable: 'UNAVAILABLE',
  },
  trend: [{
    period_start: '2026-01-01', period_end: '2026-01-01', online_gmv_gross: '100000',
    refund_principal: '0', online_gmv_net: '100000', projected_commission: '7000',
    platform_operating_expense: '1000',
  }],
  caveats: [],
};

const expenseFixture = {
  id: 'exp-1',
  category: 'INFRASTRUCTURE',
  vendor: 'QA Vendor',
  amount_rupiah: '100000',
  currency: 'IDR',
  occurred_at: '2026-01-01T00:00:00Z',
  payment_account: 'FUNDING_CLEARING',
  external_reference: 'QA-EXP-1',
  description: 'Responsive QA expense',
  status: 'VOID',
  posted_journal_id: 'jnl-1',
  void_journal_id: 'jnl-2',
  created_by_user_id: qaUser.id,
  approved_by_user_id: qaUser.id,
  posted_by_user_id: qaUser.id,
  voided_by_user_id: qaUser.id,
  cancelled_by_user_id: null,
  cancel_reason: null,
  void_reason: 'Responsive QA reversal',
  created_at: '2026-01-01T00:00:00Z',
  approved_at: '2026-01-01T00:01:00Z',
  posted_at: '2026-01-01T00:02:00Z',
  voided_at: '2026-01-01T00:03:00Z',
  cancelled_at: null,
};

const journals = [
  {
    id: 'jnl-1', event_key: 'exp-1', event_type: 'PLATFORM_EXPENSE_POSTED', booking_id: null,
    owner_profile_id: null, venue_id: null, currency: 'IDR', effective_at: '2026-01-01T00:02:00Z',
    posted_at: '2026-01-01T00:02:00Z', reverses_journal_id: null, reversal_reason: null,
    reversed_by_journal_id: 'jnl-2', entry_count: 2, debit_total_rupiah: '100000', credit_total_rupiah: '100000',
  },
  {
    id: 'jnl-2', event_key: 'exp-1', event_type: 'PLATFORM_EXPENSE_REVERSED', booking_id: null,
    owner_profile_id: null, venue_id: null, currency: 'IDR', effective_at: '2026-01-01T00:03:00Z',
    posted_at: '2026-01-01T00:03:00Z', reverses_journal_id: 'jnl-1', reversal_reason: 'Responsive QA reversal',
    reversed_by_journal_id: null, entry_count: 2, debit_total_rupiah: '100000', credit_total_rupiah: '100000',
  },
];

function createServerProcess() {
  const viteEntry = path.join(process.cwd(), 'node_modules', 'vite', 'bin', 'vite.js');
  if (!existsSync(viteEntry)) throw new Error(`Vite entry not found: ${viteEntry}`);
  return spawn(process.execPath, [viteEntry, '--host', host, '--port', String(port), '--strictPort'], {
    cwd: process.cwd(),
    env: {
      ...process.env,
      VITE_API_BASE_URL: apiURL,
      VITE_PLATFORM_FINANCE_ADMIN_ENABLED: 'true',
      VITE_USE_MOCK_AUTH: 'false',
    },
    stdio: 'ignore',
    windowsHide: true,
    detached: process.platform !== 'win32',
  });
}

function assertRequest(failures, request, expectedMethod, expectedPath) {
  if (request.method() !== expectedMethod) {
    failures.push(`${expectedPath}: expected ${expectedMethod}, got ${request.method()}`);
  }
  const requestURL = new URL(request.url());
  if (requestURL.pathname !== expectedPath) failures.push(`${expectedPath}: unexpected path ${requestURL.pathname}`);
}

async function runViewport(browser, viewport) {
  const context = await browser.newContext({ viewport: { width: viewport.width, height: viewport.height } });
  const page = await context.newPage();
  const failures = [];
  const journalRequests = [];

  const fail = (message) => failures.push(`${viewport.name}: ${message}`);
  const assertNoOverflow = async (label) => {
    if (await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth + 1)) {
      fail(`${label} has horizontal overflow`);
    }
  };
  const waitForJournal = (journalID) => page.waitForResponse((response) => {
    const requestURL = new URL(response.url());
    return requestURL.pathname === '/admin/finance/journals'
      && requestURL.searchParams.get('journal_id') === journalID
      && response.status() === 200;
  });

  page.on('pageerror', (error) => fail(`page error: ${error.message}`));
  page.on('console', (message) => {
    if (message.type() === 'error') fail(`console error: ${message.text()}`);
  });

  // Register the API catch-all first; specific routes registered below take precedence.
  await page.route(`${apiURL}/**`, async (route) => {
    failures.push(`${viewport.name}: unexpected API request ${route.request().method()} ${route.request().url()}`);
    await route.abort();
  });

  await page.route(`${apiURL}/auth/login`, async (route) => {
    const request = route.request();
    assertRequest(failures, request, 'POST', '/auth/login');
    const contentType = request.headers()['content-type'] || '';
    if (!contentType.includes('application/json')) failures.push(`${viewport.name}: login content type was not JSON`);
    let body;
    try { body = request.postDataJSON(); } catch { body = null; }
    if (!body || body.email !== qaEmail || body.password !== qaPassword) {
      failures.push(`${viewport.name}: login body did not match the typed credentials`);
      await route.fulfill(jsonResponse({ message: 'invalid QA request' }, 400));
      return;
    }
    await route.fulfill(jsonResponse({ message: 'OK', token: qaToken, user: qaUser }));
  });

  await page.route(`${apiURL}/auth/me`, async (route) => {
    const request = route.request();
    assertRequest(failures, request, 'GET', '/auth/me');
    if (request.headers().authorization !== `Bearer ${qaToken}`) {
      failures.push(`${viewport.name}: /auth/me bearer token was invalid`);
      await route.fulfill(jsonResponse({ message: 'invalid QA token' }, 401));
      return;
    }
    await route.fulfill(jsonResponse({ user: qaUser }));
  });

  await page.route(`${apiURL}/admin/dashboard`, (route) => {
    assertRequest(failures, route.request(), 'GET', '/admin/dashboard');
    return route.fulfill(jsonResponse({ total_users: 1, total_owners: 0, total_venues: 0, total_bookings: 0 }));
  });
  await page.route(`${apiURL}/admin/finance/summary*`, (route) => {
    assertRequest(failures, route.request(), 'GET', '/admin/finance/summary');
    return route.fulfill(jsonResponse(summary));
  });
  await page.route(`${apiURL}/admin/owners*`, (route) => {
    assertRequest(failures, route.request(), 'GET', '/admin/owners');
    return route.fulfill(jsonResponse(paginated([])));
  });
  await page.route(`${apiURL}/admin/venues*`, (route) => {
    assertRequest(failures, route.request(), 'GET', '/admin/venues');
    return route.fulfill(jsonResponse(paginated([])));
  });
  await page.route(`${apiURL}/notifications/unread-count`, (route) => {
    assertRequest(failures, route.request(), 'GET', '/notifications/unread-count');
    return route.fulfill(jsonResponse({ count: 0 }));
  });
  await page.route(`${apiURL}/admin/finance/expenses*`, (route) => {
    assertRequest(failures, route.request(), 'GET', '/admin/finance/expenses');
    return route.fulfill(jsonResponse(paginated([expenseFixture])));
  });
  await page.route(`${apiURL}/admin/finance/journals*`, (route) => {
    assertRequest(failures, route.request(), 'GET', '/admin/finance/journals');
    const requestURL = new URL(route.request().url());
    const journalID = requestURL.searchParams.get('journal_id');
    if (journalID) {
      journalRequests.push(journalID);
      const match = journals.find((journal) => journal.id === journalID);
      if (!match) {
        failures.push(`${viewport.name}: unknown journal_id ${journalID}`);
        return route.fulfill(jsonResponse(paginated([]), 400));
      }
      return route.fulfill(jsonResponse(paginated([match])));
    }
    return route.fulfill(jsonResponse(paginated(journals, journals.length)));
  });

  try {
    await page.goto(`${baseURL}/login`, { waitUntil: 'networkidle' });
    await page.getByLabel('Alamat Email').fill(qaEmail);
    await page.getByLabel('Kata Sandi').fill(qaPassword);
    await page.getByRole('button', { name: 'Masuk Sekarang' }).click();
    await page.waitForURL('**/admin/dashboard', { waitUntil: 'networkidle' });

    if (viewport.name === 'mobile') {
      await page.getByRole('button', { name: 'Open admin navigation' }).click();
    }
    await page.getByRole('button', { name: 'Platform Finance' }).click();
    await page.waitForURL('**/admin/finance');
    if (!(await page.getByRole('heading', { name: 'Keuangan Platform' }).isVisible())) fail('summary heading did not render');
    await assertNoOverflow('summary');

    await page.getByRole('link', { name: /Pengeluaran/ }).click();
    await page.waitForURL('**/admin/finance/expenses');
    if (!(await page.getByRole('heading', { name: 'Platform Finance' }).isVisible())) fail('expense heading did not render');
    const visibleVoidBadge = viewport.name === 'desktop'
      ? page.getByRole('cell', { name: 'VOID', exact: true })
      : page.locator('article:visible').getByText('VOID', { exact: true });
    await visibleVoidBadge.first().waitFor({ state: 'visible' });
    await assertNoOverflow('expenses');

    const originalResponse = waitForJournal('jnl-1');
    await page.locator('a[href="#journal-jnl-1"]:visible').first().click();
    await originalResponse;
    const journalsTab = page.getByRole('button', { name: 'Journals' });
    if (!(await journalsTab.isVisible())) fail('Journals tab did not render');
    if (!((await journalsTab.getAttribute('class')) || '').includes('border-emerald-600')) fail('Journals tab was not active');
    if (!(await page.getByText(/Showing linked journal/).isVisible())) fail('original journal focus notice did not render');
    if (!(await page.locator('[data-focused-journal="jnl-1"]:visible').isVisible())) fail('jnl-1 was not focused');

    const reversalResponse = waitForJournal('jnl-2');
    await page.locator('#journal-jnl-1:visible').getByRole('link', { name: 'Reversed by jnl-2' }).click();
    await reversalResponse;
    if (!(await page.locator('[data-focused-journal="jnl-2"]:visible').isVisible())) fail('jnl-2 was not focused');

    const originalAgainResponse = waitForJournal('jnl-1');
    await page.locator('#journal-jnl-2:visible').getByRole('link', { name: 'Reversal of jnl-1' }).click();
    await originalAgainResponse;
    if (!(await page.locator('[data-focused-journal="jnl-1"]:visible').isVisible())) fail('focus did not return to jnl-1');
    if (journalRequests.join('→') !== 'jnl-1→jnl-2→jnl-1') fail(`unexpected journal request sequence: ${journalRequests.join('→')}`);
    await assertNoOverflow('journals');
  } finally {
    await context.close();
  }

  if (failures.length > 0) throw new Error(failures.join('\n'));
}

export async function cleanupResources(browser, server, stopServerFn = stopServer) {
  const cleanupErrors = [];
  if (browser) {
    try {
      await browser.close();
    } catch (error) {
      cleanupErrors.push(`browser cleanup failed: ${error instanceof Error ? error.message : String(error)}`);
    }
  }
  try {
    await stopServerFn(server);
  } catch (error) {
    cleanupErrors.push(`server cleanup failed: ${error instanceof Error ? error.message : String(error)}`);
  }
  if (cleanupErrors.length > 0) throw new Error(cleanupErrors.join('; '));
}

async function main() {
  const executablePath = findBrowser();
  if (!executablePath) throw new Error('No Chromium/Chrome executable found; set CHROME_PATH to run responsive browser QA.');
  if (!(await checkPortFree(port))) throw new Error(`Port ${port} is already in use; refusing to use a stale Vite server.`);

  const server = createServerProcess();
  let browser;
  let workflowError;
  let cleanupError;
  try {
    await waitForServer(server);
    browser = await chromium.launch({ headless: true, executablePath });
    for (const viewport of [
      { name: 'desktop', width: 1280, height: 800 },
      { name: 'mobile', width: 390, height: 844 },
    ]) {
      await runViewport(browser, viewport);
    }
    console.log('SUCCESS: Platform finance browser QA passed for desktop and mobile viewports.');
  } catch (error) {
    workflowError = error;
  } finally {
    try {
      await cleanupResources(browser, server);
    } catch (error) {
      cleanupError = error;
    }
  }
  if (workflowError && cleanupError) throw new Error(`${workflowError.message}; cleanup failed: ${cleanupError.message}`);
  if (workflowError) throw workflowError;
  if (cleanupError) throw cleanupError;
}

const isMainModule = process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url);
if (isMainModule) {
  main().catch((error) => {
    console.error(`FAILED: ${error instanceof Error ? error.message : String(error)}`);
    process.exitCode = 1;
  });
}
