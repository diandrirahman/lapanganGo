import assert from 'node:assert/strict';
import test from 'node:test';
import { cleanupResources } from './test_platform_finance_responsive.mjs';

test('server cleanup still runs when browser close fails', async () => {
  let stopCalls = 0;
  const browser = { close: async () => { throw new Error('injected browser close failure'); } };
  const stopServer = async () => { stopCalls += 1; };

  await assert.rejects(
    cleanupResources(browser, { pid: 12345 }, stopServer),
    /browser cleanup failed: injected browser close failure/,
  );
  assert.equal(stopCalls, 1, 'Vite cleanup must run after browser cleanup failure');
});

test('browser cleanup failure and server cleanup failure are both reported', async () => {
  const browser = { close: async () => { throw new Error('browser failure'); } };
  const stopServer = async () => { throw new Error('server failure'); };

  await assert.rejects(
    cleanupResources(browser, { pid: 12345 }, stopServer),
    /browser cleanup failed: browser failure; server cleanup failed: server failure/,
  );
});
