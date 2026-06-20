package courts

import "testing"

func TestBuildCreateCourtParamsAllowsZeroPrice(t *testing.T) {
	price := 0.0

	params, err := buildCreateCourtParams("venue-1", CreateCourtRequest{
		SportID:      "sport-1",
		Name:         " Lapangan A ",
		Description:  " Indoor utama ",
		LocationType: "INDOOR",
		SurfaceType:  " Rumput Sintetis ",
		PricePerHour: &price,
	})
	if err != nil {
		t.Fatalf("expected params to be valid, got %v", err)
	}

	if params.VenueID != "venue-1" {
		t.Fatalf("expected venue id venue-1, got %q", params.VenueID)
	}
	if params.Name != "Lapangan A" {
		t.Fatalf("expected trimmed name, got %q", params.Name)
	}
	if params.PricePerHour != 0 {
		t.Fatalf("expected zero price, got %v", params.PricePerHour)
	}
	if params.SurfaceType == nil || *params.SurfaceType != "Rumput Sintetis" {
		t.Fatalf("expected trimmed surface type, got %v", params.SurfaceType)
	}
}

func TestBuildCreateCourtParamsRejectsMissingPrice(t *testing.T) {
	_, err := buildCreateCourtParams("venue-1", CreateCourtRequest{
		SportID:      "sport-1",
		Name:         "Lapangan A",
		LocationType: "INDOOR",
	})
	if err != ErrInvalidCourtPayload {
		t.Fatalf("expected ErrInvalidCourtPayload, got %v", err)
	}
}

func TestBuildCreateCourtParamsRejectsInvalidLocationType(t *testing.T) {
	price := 120000.0

	_, err := buildCreateCourtParams("venue-1", CreateCourtRequest{
		SportID:      "sport-1",
		Name:         "Lapangan A",
		LocationType: "SEMI_INDOOR",
		PricePerHour: &price,
	})
	if err != ErrInvalidCourtPayload {
		t.Fatalf("expected ErrInvalidCourtPayload, got %v", err)
	}
}

func TestIsWritableCourtStatus(t *testing.T) {
	allowed := []string{"ACTIVE", "INACTIVE", "MAINTENANCE"}
	for _, status := range allowed {
		if !isWritableCourtStatus(status) {
			t.Fatalf("expected status %q to be writable", status)
		}
	}

	if isWritableCourtStatus("SUSPENDED") {
		t.Fatal("expected SUSPENDED to be rejected for court status")
	}
}
