export interface JournalFocusState {
  tab: 'journals';
  journalID: string;
  page: 1;
  clearDateFilters: true;
}

export const createJournalFocusState = (journalID: string): JournalFocusState => ({
  tab: 'journals',
  journalID,
  page: 1,
  clearDateFilters: true,
});

export interface ExpenseMutationRefreshState {
  tab: 'expenses';
  page: 1;
  clearJournalFocus: true;
  incrementRefreshToken: true;
}

export const createExpenseMutationRefreshState = (): ExpenseMutationRefreshState => ({
  tab: 'expenses',
  page: 1,
  clearJournalFocus: true,
  incrementRefreshToken: true,
});
