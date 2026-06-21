package availability

import (
	"errors"
	"testing"
	"time"
)

func TestParseAvailabilityDate(t *testing.T) {
	location := jakartaLocation()
	date, err := parseAvailabilityDate("2026-06-25", location)
	if err != nil {
		t.Fatalf("expected valid date, got %v", err)
	}

	if date.Year() != 2026 || date.Month() != time.June || date.Day() != 25 {
		t.Fatalf("unexpected date %v", date)
	}
	if date.Location() != location {
		t.Fatalf("expected location %v, got %v", location, date.Location())
	}
}

func TestParseAvailabilityDateRejectsInvalidDate(t *testing.T) {
	_, err := parseAvailabilityDate("25-06-2026", jakartaLocation())
	if err == nil {
		t.Fatal("expected invalid date error")
	}
}

func TestBuildSlotsMarksBlockedOverlap(t *testing.T) {
	location := jakartaLocation()
	date, err := parseAvailabilityDate("2026-06-25", location)
	if err != nil {
		t.Fatalf("expected valid date, got %v", err)
	}

	openTime := "08:00"
	closeTime := "11:00"
	blockedSlots := []BlockedSlot{
		{
			StartAt: time.Date(2026, time.June, 25, 9, 30, 0, 0, location),
			EndAt:   time.Date(2026, time.June, 25, 10, 30, 0, 0, location),
		},
	}

	slots, err := buildSlots(date, OperatingHour{
		OpenTime:  &openTime,
		CloseTime: &closeTime,
	}, blockedSlots, time.Hour)
	if err != nil {
		t.Fatalf("expected slots to build, got %v", err)
	}

	if len(slots) != 3 {
		t.Fatalf("expected 3 slots, got %d", len(slots))
	}

	expectedStatuses := []string{slotStatusAvailable, slotStatusBlocked, slotStatusBlocked}
	for i, expectedStatus := range expectedStatuses {
		if slots[i].Status != expectedStatus {
			t.Fatalf("expected slot %d status %q, got %q", i, expectedStatus, slots[i].Status)
		}
	}
}

func TestBuildSlotsRejectsInvalidOperatingHours(t *testing.T) {
	location := jakartaLocation()
	date, err := parseAvailabilityDate("2026-06-25", location)
	if err != nil {
		t.Fatalf("expected valid date, got %v", err)
	}

	openTime := "22:00"
	closeTime := "08:00"
	_, err = buildSlots(date, OperatingHour{
		OpenTime:  &openTime,
		CloseTime: &closeTime,
	}, nil, time.Hour)
	if !errors.Is(err, ErrInvalidOperatingHours) {
		t.Fatalf("expected ErrInvalidOperatingHours, got %v", err)
	}
}

func TestIsClosedOperatingHour(t *testing.T) {
	openTime := "08:00"
	closeTime := "22:00"
	if !isClosedOperatingHour(OperatingHour{IsClosed: true}) {
		t.Fatal("expected explicitly closed operating hour to be closed")
	}
	if !isClosedOperatingHour(OperatingHour{OpenTime: nil, CloseTime: &closeTime}) {
		t.Fatal("expected missing open time to be closed")
	}
	if isClosedOperatingHour(OperatingHour{OpenTime: &openTime, CloseTime: &closeTime}) {
		t.Fatal("expected valid operating hour to be open")
	}
}
