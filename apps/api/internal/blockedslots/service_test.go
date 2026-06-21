package blockedslots

import (
	"errors"
	"testing"
)

func TestBuildBlockedSlotParams(t *testing.T) {
	params, err := buildBlockedSlotParams("court-1", CreateBlockedSlotRequest{
		StartAt: "2026-06-25T18:00:00+07:00",
		EndAt:   "2026-06-25T20:00:00+07:00",
		Reason:  " Maintenance lampu ",
	})
	if err != nil {
		t.Fatalf("expected valid params, got %v", err)
	}

	if params.CourtID != "court-1" {
		t.Fatalf("expected court id court-1, got %q", params.CourtID)
	}
	if !params.EndAt.After(params.StartAt) {
		t.Fatal("expected end_at to be after start_at")
	}
	if params.Reason == nil || *params.Reason != "Maintenance lampu" {
		t.Fatalf("expected trimmed reason, got %v", params.Reason)
	}
}

func TestBuildBlockedSlotParamsRejectsInvalidDatetime(t *testing.T) {
	_, err := buildBlockedSlotParams("court-1", CreateBlockedSlotRequest{
		StartAt: "2026-06-25 18:00",
		EndAt:   "2026-06-25T20:00:00+07:00",
	})
	if !errors.Is(err, ErrInvalidBlockedSlot) {
		t.Fatalf("expected ErrInvalidBlockedSlot, got %v", err)
	}
}

func TestBuildBlockedSlotParamsRejectsInvalidRange(t *testing.T) {
	_, err := buildBlockedSlotParams("court-1", CreateBlockedSlotRequest{
		StartAt: "2026-06-25T20:00:00+07:00",
		EndAt:   "2026-06-25T18:00:00+07:00",
	})
	if !errors.Is(err, ErrInvalidBlockedSlotRange) {
		t.Fatalf("expected ErrInvalidBlockedSlotRange, got %v", err)
	}
}

func TestBuildBlockedSlotParamsConvertsBlankReasonToNil(t *testing.T) {
	params, err := buildBlockedSlotParams("court-1", CreateBlockedSlotRequest{
		StartAt: "2026-06-25T18:00:00+07:00",
		EndAt:   "2026-06-25T20:00:00+07:00",
		Reason:  " ",
	})
	if err != nil {
		t.Fatalf("expected valid params, got %v", err)
	}
	if params.Reason != nil {
		t.Fatalf("expected blank reason to become nil, got %v", params.Reason)
	}
}

func TestBuildListRange(t *testing.T) {
	from, to, err := buildListRange("2026-06-01T00:00:00+07:00", "2026-06-30T23:59:59+07:00")
	if err != nil {
		t.Fatalf("expected valid range, got %v", err)
	}
	if from == nil || to == nil {
		t.Fatal("expected both from and to values")
	}
	if !to.After(*from) {
		t.Fatal("expected to to be after from")
	}
}

func TestBuildListRangeAllowsOpenRange(t *testing.T) {
	from, to, err := buildListRange("", "")
	if err != nil {
		t.Fatalf("expected empty range to be valid, got %v", err)
	}
	if from != nil || to != nil {
		t.Fatalf("expected nil range values, got from=%v to=%v", from, to)
	}
}

func TestBuildListRangeRejectsInvalidRange(t *testing.T) {
	_, _, err := buildListRange("2026-06-30T23:59:59+07:00", "2026-06-01T00:00:00+07:00")
	if !errors.Is(err, ErrInvalidBlockedSlotRange) {
		t.Fatalf("expected ErrInvalidBlockedSlotRange, got %v", err)
	}
}
