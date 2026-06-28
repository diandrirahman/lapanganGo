package venues

import "testing"

func TestNormalizeIDs(t *testing.T) {
	got := normalizeIDs([]string{
		" facility-1 ",
		"",
		"facility-2",
		"facility-1",
		"   ",
	})

	want := []string{"facility-1", "facility-2"}
	if len(got) != len(want) {
		t.Fatalf("expected %d ids, got %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected id %d to be %q, got %q", i, want[i], got[i])
		}
	}
}

func TestBuildCreateVenueParamsTrimsInput(t *testing.T) {
	latitude := -6.1912345
	longitude := 106.7812345

	params, err := buildCreateVenueParams("profile-1", CreateVenueRequest{
		Name:        " Arena Futsal ",
		Description: "  ",
		Address:     " Jl. Kemanggisan ",
		District:    " Palmerah ",
		City:        " Jakarta Barat ",
		Province:    " DKI Jakarta ",
		PostalCode:  " 11480 ",
		Latitude:    &latitude,
		Longitude:   &longitude,
	})
	if err != nil {
		t.Fatalf("expected params to be valid, got %v", err)
	}

	if params.Name != "Arena Futsal" {
		t.Fatalf("expected trimmed name, got %q", params.Name)
	}
	if params.Description != nil {
		t.Fatalf("expected blank description to become nil, got %v", params.Description)
	}
	if params.Address != "Jl. Kemanggisan" {
		t.Fatalf("expected trimmed address, got %q", params.Address)
	}
	if params.District == nil || *params.District != "Palmerah" {
		t.Fatalf("expected trimmed district, got %v", params.District)
	}
}

func TestBuildCreateVenueParamsRejectsRequiredBlanks(t *testing.T) {
	_, err := buildCreateVenueParams("profile-1", CreateVenueRequest{
		Name:    " ",
		Address: "Jl. Kemanggisan",
		City:    "Jakarta Barat",
	})
	if err != ErrInvalidVenuePayload {
		t.Fatalf("expected ErrInvalidVenuePayload, got %v", err)
	}
}

func TestIsOwnerWritableStatus(t *testing.T) {
	allowed := []string{"DRAFT", "ACTIVE", "INACTIVE"}
	for _, status := range allowed {
		if !isOwnerWritableStatus(status) {
			t.Fatalf("expected status %q to be writable", status)
		}
	}

	if isOwnerWritableStatus("SUSPENDED") {
		t.Fatal("expected SUSPENDED to be rejected for owner updates")
	}
}

func TestNormalizeListPublicVenuesQuery(t *testing.T) {
	req := ListPublicVenuesQuery{} // empty request
	req, offset := normalizeListPublicVenuesQuery(req)
	if req.Limit != 10 {
		t.Errorf("expected default limit 10, got %d", req.Limit)
	}
	if req.Page != 1 {
		t.Errorf("expected default page 1, got %d", req.Page)
	}
	if offset != 0 {
		t.Errorf("expected default offset 0, got %d", offset)
	}
}
