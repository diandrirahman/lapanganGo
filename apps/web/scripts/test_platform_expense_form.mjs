import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import * as ts from 'typescript';

const source = await readFile(new URL('../src/lib/platformExpenseForm.ts', import.meta.url), 'utf8');
const workflowSource = await readFile(new URL('../src/lib/platformExpenseWorkflow.ts', import.meta.url), 'utf8');
const adminApiSource = await readFile(new URL('../src/lib/api/admin.ts', import.meta.url), 'utf8');
const expensePageSource = await readFile(new URL('../src/pages/admin/AdminPlatformExpensesPage.tsx', import.meta.url), 'utf8');
const { outputText } = ts.transpileModule(source, {
  compilerOptions: {
    module: ts.ModuleKind.ESNext,
    target: ts.ScriptTarget.ES2022,
  },
});
const { outputText: workflowOutputText } = ts.transpileModule(workflowSource, {
  compilerOptions: {
    module: ts.ModuleKind.ESNext,
    target: ts.ScriptTarget.ES2022,
  },
});
const form = await import(`data:text/javascript,${encodeURIComponent(outputText)}`);
const workflow = await import(`data:text/javascript,${encodeURIComponent(workflowOutputText)}`);

assert.equal(form.isRetryableExpenseSubmissionError(new Error('Request timeout')), true);
assert.equal(form.isRetryableExpenseSubmissionError(new TypeError('Failed to fetch')), true);
assert.equal(form.isRetryableExpenseSubmissionError(new SyntaxError('Unexpected end of JSON input')), true);
assert.equal(form.isRetryableExpenseSubmissionError({ status: 500 }), true);
assert.equal(form.isRetryableExpenseSubmissionError({ status: 409 }), false);
assert.equal(form.validateExpenseCancelReason('  duplicate invoice  '), null);
assert.match(form.validateExpenseCancelReason(''), /required/);
assert.match(form.validateExpenseCancelReason('x'.repeat(501)), /500 bytes/);
assert.equal(form.validateExpenseVoidReason('  supplier correction  '), null);
assert.match(form.validateExpenseVoidReason(''), /required/);

const journalFocus = workflow.createJournalFocusState('journal-123');
assert.deepEqual(journalFocus, {
  tab: 'journals',
  journalID: 'journal-123',
  page: 1,
  clearDateFilters: true,
}, 'a reversal link must switch to journals and focus the exact journal query');
assert.deepEqual(workflow.createExpenseMutationRefreshState(), {
  tab: 'expenses',
  page: 1,
  clearJournalFocus: true,
  incrementRefreshToken: true,
}, 'a successful or conflict mutation must return to a refreshed expenses view');

const timeoutState = form.createExpenseAttemptState();
const timeoutPayload = '{"amount_rupiah":"400000"}';
const timeoutKey = form.getExpenseAttemptKey(timeoutState, timeoutPayload, () => 'timeout-key');
assert.equal(form.isRetryableExpenseSubmissionError(new Error('Request timeout')), true);
assert.equal(
  form.getExpenseAttemptKey(timeoutState, timeoutPayload, () => 'wrong-new-key'),
  timeoutKey,
  'a client timeout must retain the same key for close/reopen retry',
);

const state = form.createExpenseAttemptState();
let keyCalls = 0;
const createKey = () => {
  keyCalls += 1;
  return keyCalls === 1 ? 'stable-expense-key' : 'new-expense-key';
};
const payload = '{"amount_rupiah":"250000"}';
const firstKey = form.getExpenseAttemptKey(state, payload, createKey);
const reopenedKey = form.getExpenseAttemptKey(state, payload, createKey);
assert.equal(reopenedKey, firstKey, 'same action must reuse its idempotency key after close/reopen');
assert.equal(keyCalls, 1, 'reopening the same action must not generate a second key');

form.clearExpenseAttempt(state);
assert.equal(
  form.getExpenseAttemptKey(state, '{"amount_rupiah":"300000"}', createKey),
  'new-expense-key',
  'a terminal action must start with a fresh idempotency key',
);

const instant = new Date('2026-07-17T13:00:00.000Z');
assert.equal(form.formatJakartaDateTimeInput(instant), '2026-07-17T20:00');
assert.equal(form.toJakartaExpenseTimestamp('2026-07-17T20:00'), '2026-07-17T20:00:00+07:00');
assert.equal(
  form.parseJakartaExpenseTimestamp('2026-07-17T20:00')?.toISOString(),
  '2026-07-17T13:00:00.000Z',
  'Jakarta input must round-trip to the same instant from any browser timezone',
);

assert.match(adminApiSource, /postPlatformExpense/);
assert.match(adminApiSource, /\/post/);
assert.match(adminApiSource, /voidPlatformExpense/);
assert.match(adminApiSource, /\/void/);
assert.match(expensePageSource, /type: 'post'/);
assert.match(expensePageSource, /type: 'void'/);
assert.match(expensePageSource, /Void reversal/);
assert.match(expensePageSource, /onJournalLink/);
assert.match(expensePageSource, /refreshExpenses/);

console.log('platform expense form regression tests: PASS');
