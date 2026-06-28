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
	}, blockedSlots, nil, time.Hour)
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

func TestBuildSlotsMarksBookedOverlap(t *testing.T) {
	location := jakartaLocation()
	date, err := parseAvailabilityDate("2026-06-25", location)
	if err != nil {
		t.Fatalf("expected valid date, got %v", err)
	}

	openTime := "08:00"
	closeTime := "12:00"

	// Create dummy time representing 09:30 and 11:30 for booking
	// Time fields from DB for time type are usually Jan 1 year 0000 or year 1970
	t0930 := time.Date(0, 1, 1, 9, 30, 0, 0, time.UTC)
	t1130 := time.Date(0, 1, 1, 11, 30, 0, 0, time.UTC)

	bookings := []ActiveBooking{
		{
			Date:      date,
			StartTime: t0930,
			EndTime:   t1130,
		},
	}

	slots, err := buildSlots(date, OperatingHour{
		OpenTime:  &openTime,
		CloseTime: &closeTime,
	}, nil, bookings, time.Hour)
	if err != nil {
		t.Fatalf("expected slots to build, got %v", err)
	}

	if len(slots) != 4 {
		t.Fatalf("expected 4 slots, got %d", len(slots))
	}

	// 08:00-09:00 -> AVAILABLE
	// 09:00-10:00 -> BOOKED (overlap 09:30)
	// 10:00-11:00 -> BOOKED (inside booking)
	// 11:00-12:00 -> BOOKED (overlap 11:30)
	expectedStatuses := []string{slotStatusAvailable, slotStatusBooked, slotStatusBooked, slotStatusBooked}
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
	}, nil, nil, time.Hour)
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

func TestBuildSlotsIgnoresExpiredBookings(t *testing.T) {
	location := jakartaLocation()
	date, _ := parseAvailabilityDate("2026-06-25", location)
	openTime := "08:00"
	closeTime := "12:00"

	t0800 := time.Date(0, 1, 1, 8, 0, 0, 0, time.UTC)
	t0900 := time.Date(0, 1, 1, 9, 0, 0, 0, time.UTC)
	t1000 := time.Date(0, 1, 1, 10, 0, 0, 0, time.UTC)
	t1100 := time.Date(0, 1, 1, 11, 0, 0, 0, time.UTC)
	t1200 := time.Date(0, 1, 1, 12, 0, 0, 0, time.UTC)

	now := time.Now()
	expiredTime := now.Add(-time.Hour)
	activeTime := now.Add(time.Hour)

	bookings := []ActiveBooking{
		// 1. Confirmed booking -> should block (08:00 - 09:00)
		{Date: date, StartTime: t0800, EndTime: t0900, Status: "CONFIRMED"},
		// 2. Pending expired -> should NOT block (09:00 - 10:00)
		{Date: date, StartTime: t0900, EndTime: t1000, Status: "PENDING_PAYMENT", ExpiresAt: &expiredTime},
		// 3. Pending active -> should block (10:00 - 11:00)
		{Date: date, StartTime: t1000, EndTime: t1100, Status: "PENDING_PAYMENT", ExpiresAt: &activeTime},
		// 4. Pending null expiry -> should NOT block (11:00 - 12:00)
		{Date: date, StartTime: t1100, EndTime: t1200, Status: "PENDING_PAYMENT", ExpiresAt: nil},
	}

	slots, err := buildSlots(date, OperatingHour{OpenTime: &openTime, CloseTime: &closeTime}, nil, bookings, time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(slots) != 4 {
		t.Fatalf("expected 4 slots, got %d", len(slots))
	}

	expectedStatuses := []string{
		slotStatusBooked,    // 08:00-09:00 (CONFIRMED)
		slotStatusAvailable, // 09:00-10:00 (Expired PENDING_PAYMENT)
		slotStatusBooked,    // 10:00-11:00 (Active PENDING_PAYMENT)
		slotStatusAvailable, // 11:00-12:00 (Null expiry PENDING_PAYMENT)
	}

	for i, expected := range expectedStatuses {
		if slots[i].Status != expected {
			t.Errorf("slot %d expected status %q, got %q", i, expected, slots[i].Status)
		}
	}
}
