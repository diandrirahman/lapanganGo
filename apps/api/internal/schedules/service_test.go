package schedules

import (
	"errors"
	"testing"
)

func TestBuildOperatingHourParams(t *testing.T) {
	openTime := "08:00"
	closeTime := "22:00"
	days := make([]OperatingHourRequest, 0, 7)
	for day := 0; day < 7; day++ {
		dayOfWeek := day
		days = append(days, OperatingHourRequest{
			DayOfWeek: &dayOfWeek,
			OpenTime:  &openTime,
			CloseTime: &closeTime,
			IsClosed:  false,
		})
	}

	params, err := buildOperatingHourParams(ReplaceOperatingHoursRequest{Days: days})
	if err != nil {
		t.Fatalf("expected valid operating hours, got error %v", err)
	}
	if len(params) != 7 {
		t.Fatalf("expected 7 params, got %d", len(params))
	}
}

func TestBuildOperatingHourParamsRejectsIncompleteSet(t *testing.T) {
	_, err := buildOperatingHourParams(ReplaceOperatingHoursRequest{
		Days: []OperatingHourRequest{},
	})
	if !errors.Is(err, ErrIncompleteOperatingSet) {
		t.Fatalf("expected ErrIncompleteOperatingSet, got %v", err)
	}
}

func TestBuildOperatingHourParamsRejectsDuplicateDay(t *testing.T) {
	openTime := "08:00"
	closeTime := "22:00"
	days := make([]OperatingHourRequest, 0, 7)
	for day := 0; day < 7; day++ {
		dayOfWeek := day
		if day == 6 {
			dayOfWeek = 5
		}
		days = append(days, OperatingHourRequest{
			DayOfWeek: &dayOfWeek,
			OpenTime:  &openTime,
			CloseTime: &closeTime,
			IsClosed:  false,
		})
	}

	_, err := buildOperatingHourParams(ReplaceOperatingHoursRequest{Days: days})
	if !errors.Is(err, ErrDuplicateOperatingDay) {
		t.Fatalf("expected ErrDuplicateOperatingDay, got %v", err)
	}
}

func TestBuildOperatingHourParamsRejectsInvalidTimeRange(t *testing.T) {
	openTime := "22:00"
	closeTime := "08:00"
	days := make([]OperatingHourRequest, 0, 7)
	for day := 0; day < 7; day++ {
		dayOfWeek := day
		days = append(days, OperatingHourRequest{
			DayOfWeek: &dayOfWeek,
			OpenTime:  &openTime,
			CloseTime: &closeTime,
			IsClosed:  false,
		})
	}

	_, err := buildOperatingHourParams(ReplaceOperatingHoursRequest{Days: days})
	if !errors.Is(err, ErrInvalidOperatingHours) {
		t.Fatalf("expected ErrInvalidOperatingHours, got %v", err)
	}
}

func TestBuildOperatingHourParamsRejectsClosedDayWithTimes(t *testing.T) {
	openTime := "08:00"
	closedDay := 0
	days := []OperatingHourRequest{
		{
			DayOfWeek: &closedDay,
			OpenTime:  &openTime,
			IsClosed:  true,
		},
	}
	for day := 1; day < 7; day++ {
		dayOfWeek := day
		closeTime := "22:00"
		days = append(days, OperatingHourRequest{
			DayOfWeek: &dayOfWeek,
			OpenTime:  &openTime,
			CloseTime: &closeTime,
			IsClosed:  false,
		})
	}

	_, err := buildOperatingHourParams(ReplaceOperatingHoursRequest{Days: days})
	if !errors.Is(err, ErrInvalidOperatingHours) {
		t.Fatalf("expected ErrInvalidOperatingHours, got %v", err)
	}
}

func TestBuildOperatingHourParamsAcceptsClosedDay(t *testing.T) {
	closedDay := 0
	openTime := "08:00"
	closeTime := "22:00"
	days := []OperatingHourRequest{
		{
			DayOfWeek: &closedDay,
			IsClosed:  true,
		},
	}
	for day := 1; day < 7; day++ {
		dayOfWeek := day
		days = append(days, OperatingHourRequest{
			DayOfWeek: &dayOfWeek,
			OpenTime:  &openTime,
			CloseTime: &closeTime,
			IsClosed:  false,
		})
	}

	params, err := buildOperatingHourParams(ReplaceOperatingHoursRequest{Days: days})
	if err != nil {
		t.Fatalf("expected valid operating hours, got error %v", err)
	}
	if !params[0].IsClosed || params[0].OpenTime != nil || params[0].CloseTime != nil {
		t.Fatalf("expected first day to be closed without times, got %+v", params[0])
	}
}
