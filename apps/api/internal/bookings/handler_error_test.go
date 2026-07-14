package bookings

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRespondBookingError_InvalidRupiahAmountReturnsGenericBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, testCase := range []struct {
		name string
		err  error
	}{
		{name: "NaN", err: errNaNDetected},
		{name: "infinity", err: errInfinityDetected},
		{name: "negative", err: errNegativeValueDetected},
		{name: "exceeds legacy maximum", err: errValueExceedsMax},
		{name: "fractional rupiah", err: errFractionalRupiahDetected},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)

			respondBookingError(ctx, testCase.err, "Failed to create offline booking")

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
			}
			if got, want := recorder.Body.String(), `{"message":"Invalid price"}`; got != want {
				t.Fatalf("body = %s, want %s", got, want)
			}
		})
	}
}
