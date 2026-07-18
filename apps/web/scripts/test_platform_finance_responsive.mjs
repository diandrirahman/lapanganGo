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
      .map((element) => element.getBoundingClientRect())
      .filter((rect) => rect.width > 0 && rect.right > viewport + 1)
      .slice(0, 5)
      .map((rect) => ({ right: Math.round(rect.right), width: Math.round(rect.width) }));
    return { documentWidth, viewport, overflowing };
  });
  if (measurements.documentWidth > viewportWidth || measurements.viewport > viewportWidth || measurements.overflowing.length > 0) {
    throw new Error(`horizontal overflow at ${viewportWidth}px: ${JSON.stringify(measurements)}`);
  }
  return measurements;
};

const run = async () => {
  const fixture = JSON.parse(await readFile(fixturePath, 'utf8'));
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
      const pathname = new URL(route.request().url()).pathname;
      if (pathname === '/auth/me') {
        await route.fulfill(json({ user: { id: 'qa-super-admin', name: 'QA SuperAdmin', email: 'qa@lapanggo.id', role: 'SUPER_ADMIN', status: 'ACTIVE', created_at: '2026-06-01T00:00:00Z' } }));
        return;
      }
      if (pathname.endsWith('/admin/finance/summary')) {
        await route.fulfill(json(fixture));
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
    console.log(`platform finance responsive QA: PASS (SuperAdmin fixture; 360px ${JSON.stringify(mobile)}; 1440px ${JSON.stringify(desktop)})`);
  } finally {
    await browser?.close();
    server.kill();
  }
};

run().catch((error) => {
  console.error(`platform finance responsive QA: FAIL\n${error.stack ?? error}`);
  process.exitCode = 1;
});
