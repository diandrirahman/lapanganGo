import type { AvailabilitySlot } from '../types/booking';

export function formatSlotTime(value: string): string {
  // value is expected to be an ISO string like "2026-07-05T18:00:00Z" or similar
  const d = new Date(value);
  const h = d.getHours().toString().padStart(2, '0');
  const m = d.getMinutes().toString().padStart(2, '0');
  return `${h}:${m}`;
}

export function sortSlotsByStart(slots: AvailabilitySlot[]): AvailabilitySlot[] {
  return [...slots].sort((a, b) => new Date(a.start_at).getTime() - new Date(b.start_at).getTime());
}

export function areSlotsContiguous(slots: AvailabilitySlot[]): boolean {
  if (slots.length <= 1) return true;
  const sorted = sortSlotsByStart(slots);
  for (let i = 1; i < sorted.length; i++) {
    // We consider them contiguous if end of previous == start of next
    const prevEnd = new Date(sorted[i - 1].end_at).getTime();
    const nextStart = new Date(sorted[i].start_at).getTime();
    if (prevEnd !== nextStart) {
      return false;
    }
  }
  return true;
}

export function getSelectedSlotRange(slots: AvailabilitySlot[]): {
  startTime: string;
  endTime: string;
  slotCount: number;
} | null {
  if (!slots || slots.length === 0) return null;
  const sorted = sortSlotsByStart(slots);
  return {
    startTime: formatSlotTime(sorted[0].start_at),
    endTime: formatSlotTime(sorted[sorted.length - 1].end_at),
    slotCount: slots.length,
  };
}

export function toggleContiguousSlotSelection(
  currentSelected: AvailabilitySlot[],
  clickedSlot: AvailabilitySlot
): { selection: AvailabilitySlot[], resetHappened: boolean } {
  if (clickedSlot.status !== 'AVAILABLE') return { selection: currentSelected, resetHappened: false };

  // If empty, just select this one
  if (currentSelected.length === 0) {
    return { selection: [clickedSlot], resetHappened: false };
  }

  const clickedTime = new Date(clickedSlot.start_at).getTime();
  const alreadySelectedIndex = currentSelected.findIndex(
    s => new Date(s.start_at).getTime() === clickedTime
  );

  // If clicked slot is already selected
  if (alreadySelectedIndex >= 0) {
    const sorted = sortSlotsByStart(currentSelected);
    const clickedSortedIndex = sorted.findIndex(
      s => new Date(s.start_at).getTime() === clickedTime
    );

    // If it's at the edges (first or last), remove it to shrink the selection
    if (clickedSortedIndex === 0) {
      return { selection: sorted.slice(1), resetHappened: false };
    } else if (clickedSortedIndex === sorted.length - 1) {
      return { selection: sorted.slice(0, sorted.length - 1), resetHappened: false };
    }

    // If clicked in the middle, reset selection to just this slot
    return { selection: [clickedSlot], resetHappened: true };
  }

  // Clicking an unselected slot
  const proposedSelection = sortSlotsByStart([...currentSelected, clickedSlot]);
  
  if (areSlotsContiguous(proposedSelection)) {
    return { selection: proposedSelection, resetHappened: false };
  }

  // Not contiguous, so reset to just this slot
  return { selection: [clickedSlot], resetHappened: true };
}

export function areSelectedSlotsStillAvailable(
  selectedSlots: AvailabilitySlot[],
  latestSlots: AvailabilitySlot[]
): boolean {
  for (const selected of selectedSlots) {
    const latest = latestSlots.find(
      s => new Date(s.start_at).getTime() === new Date(selected.start_at).getTime()
    );
    if (!latest || latest.status !== 'AVAILABLE') {
      return false;
    }
  }
  return true;
}
