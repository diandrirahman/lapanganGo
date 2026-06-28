package availability

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
)

const (
	availabilityStatusOpen   = "OPEN"
	availabilityStatusClosed = "CLOSED"

	slotStatusAvailable = "AVAILABLE"
	slotStatusBlocked   = "BLOCKED"
	slotStatusBooked    = "BOOKED"

	defaultSlotDuration = time.Hour
	dateLayout          = "2006-01-02"
)

var (
	ErrCourtNotFound           = errors.New("court not found")
	ErrInvalidAvailabilityDate = errors.New("invalid availability date")
	ErrInvalidOperatingHours   = errors.New("invalid operating hours")
)

type Service struct {
	repository *Repository
	location   *time.Location
}

func NewService(repository *Repository) *Service {
	return &Service{
		repository: repository,
		location:   jakartaLocation(),
	}
}

func (s *Service) GetAvailability(ctx context.Context, courtID, dateValue string) (AvailabilityResponse, error) {
	date, err := parseAvailabilityDate(dateValue, s.location)
	if err != nil {
		return AvailabilityResponse{}, ErrInvalidAvailabilityDate
	}

	court, err := s.repository.FindCourtByID(ctx, courtID)
	if IsNotFound(err) {
		return AvailabilityResponse{}, ErrCourtNotFound
	}
	if err != nil {
		return AvailabilityResponse{}, err
	}

	if court.Status != "ACTIVE" || court.VenueStatus != "ACTIVE" {
		return closedAvailability(courtID, date), nil
	}

	dayOfWeek := int(date.Weekday())
	operatingHour, err := s.repository.FindOperatingHour(ctx, courtID, dayOfWeek)
	if IsNotFound(err) {
		return closedAvailability(courtID, date), nil
	}
	if err != nil {
		return AvailabilityResponse{}, err
	}

	if isClosedOperatingHour(operatingHour) {
		return closedAvailability(courtID, date), nil
	}

	dayStart := date
	dayEnd := date.AddDate(0, 0, 1)
	blockedSlots, err := s.repository.ListBlockedSlots(ctx, courtID, dayStart, dayEnd)
	if err != nil {
		return AvailabilityResponse{}, err
	}

	bookings, err := s.repository.ListActiveBookings(ctx, courtID, date.Format(dateLayout))
	if err != nil {
		return AvailabilityResponse{}, err
	}

	slots, err := buildSlots(date, operatingHour, blockedSlots, bookings, defaultSlotDuration)
	if err != nil {
		return AvailabilityResponse{}, err
	}

	return AvailabilityResponse{
		CourtID: courtID,
		Date:    date.Format(dateLayout),
		Status:  availabilityStatusOpen,
		Slots:   slots,
	}, nil
}

func closedAvailability(courtID string, date time.Time) AvailabilityResponse {
	return AvailabilityResponse{
		CourtID: courtID,
		Date:    date.Format(dateLayout),
		Status:  availabilityStatusClosed,
		Slots:   []SlotResponse{},
	}
}

func parseAvailabilityDate(value string, location *time.Location) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, ErrInvalidAvailabilityDate
	}

	date, err := time.ParseInLocation(dateLayout, value, location)
	if err != nil {
		return time.Time{}, err
	}

	return date, nil
}

func buildSlots(date time.Time, operatingHour OperatingHour, blockedSlots []BlockedSlot, bookings []ActiveBooking, slotDuration time.Duration) ([]SlotResponse, error) {
	if operatingHour.OpenTime == nil || operatingHour.CloseTime == nil || slotDuration <= 0 {
		return nil, ErrInvalidOperatingHours
	}

	openMinutes, err := parseClockMinutes(*operatingHour.OpenTime)
	if err != nil {
		return nil, ErrInvalidOperatingHours
	}

	closeMinutes, err := parseClockMinutes(*operatingHour.CloseTime)
	if err != nil {
		return nil, ErrInvalidOperatingHours
	}

	if closeMinutes <= openMinutes {
		return nil, ErrInvalidOperatingHours
	}

	slotMinutes := int(slotDuration / time.Minute)
	slots := make([]SlotResponse, 0, (closeMinutes-openMinutes)/slotMinutes)
	for startMinutes := openMinutes; startMinutes+slotMinutes <= closeMinutes; startMinutes += slotMinutes {
		slotStart := timeAtMinutes(date, startMinutes)
		slotEnd := timeAtMinutes(date, startMinutes+slotMinutes)
		status := slotStatusAvailable
		if overlapsAnyBlockedSlot(slotStart, slotEnd, blockedSlots) {
			status = slotStatusBlocked
		} else if overlapsAnyBooking(slotStart, slotEnd, bookings) {
			status = slotStatusBooked
		}

		slots = append(slots, SlotResponse{
			StartAt: slotStart,
			EndAt:   slotEnd,
			Status:  status,
		})
	}

	return slots, nil
}

func isClosedOperatingHour(operatingHour OperatingHour) bool {
	return operatingHour.IsClosed || operatingHour.OpenTime == nil || operatingHour.CloseTime == nil
}

func overlapsAnyBlockedSlot(slotStart, slotEnd time.Time, blockedSlots []BlockedSlot) bool {
	for _, blockedSlot := range blockedSlots {
		if slotStart.Before(blockedSlot.EndAt) && slotEnd.After(blockedSlot.StartAt) {
			return true
		}
	}

	return false
}

func overlapsAnyBooking(slotStart, slotEnd time.Time, bookings []ActiveBooking) bool {
	now := time.Now()
	for _, b := range bookings {
		if b.Status == "PENDING_PAYMENT" && (b.ExpiresAt == nil || b.ExpiresAt.Before(now) || b.ExpiresAt.Equal(now)) {
			continue // ignore expired pending booking
		}

		bStart := time.Date(b.Date.Year(), b.Date.Month(), b.Date.Day(), b.StartTime.Hour(), b.StartTime.Minute(), 0, 0, slotStart.Location())
		bEnd := time.Date(b.Date.Year(), b.Date.Month(), b.Date.Day(), b.EndTime.Hour(), b.EndTime.Minute(), 0, 0, slotEnd.Location())

		if slotStart.Before(bEnd) && slotEnd.After(bStart) {
			return true
		}
	}
	return false
}

func timeAtMinutes(date time.Time, minutes int) time.Time {
	hour := minutes / 60
	minute := minutes % 60
	return time.Date(date.Year(), date.Month(), date.Day(), hour, minute, 0, 0, date.Location())
}

func parseClockMinutes(value string) (int, error) {
	if len(value) != 5 || value[2] != ':' {
		return 0, ErrInvalidOperatingHours
	}

	hour, err := strconv.Atoi(value[:2])
	if err != nil {
		return 0, err
	}

	minute, err := strconv.Atoi(value[3:])
	if err != nil {
		return 0, err
	}

	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, ErrInvalidOperatingHours
	}

	return hour*60 + minute, nil
}

func jakartaLocation() *time.Location {
	location, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return time.FixedZone("Asia/Jakarta", 7*60*60)
	}

	return location
}
