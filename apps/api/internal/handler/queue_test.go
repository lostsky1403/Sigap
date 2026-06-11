package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sigap/sigap/apps/api/internal/limiter"
	"github.com/sigap/sigap/apps/api/internal/service"
)

// TestGenerateQueueHandler_SuccessShape verifies the happy path returns the expected
// {success:true, data:{...}} shape with Indonesian-friendly data.
func TestGenerateQueueHandler_SuccessShape(t *testing.T) {
	rl := limiter.NewRateLimiter(100, time.Minute) // high limit so rate limit never triggers in this test
	svc := service.NewFakeQueueService()
	h := NewHandler(svc, rl)

	body := `{
		"facilityId": "00000000-0000-0000-0000-000000000001",
		"patient": {"fullName": "Budi Santoso", "phone": "081234567890"}
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/queues/generate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:12345"

	rr := httptest.NewRecorder()
	h.Generate(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d. body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if success, _ := resp["success"].(bool); !success {
		t.Errorf("expected success:true, got %v", resp)
	}
	if _, ok := resp["data"]; !ok {
		t.Error("expected data field in success response")
	}
}

// TestGenerateQueueHandler_MissingFields_ReturnsIndonesian400 ensures validation
// errors are returned with clear Indonesian messages (user-facing).
func TestGenerateQueueHandler_MissingFields_ReturnsIndonesian400(t *testing.T) {
	rl := limiter.NewRateLimiter(100, time.Minute)
	svc := service.NewFakeQueueService()
	h := NewHandler(svc, rl)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/queues/generate", strings.NewReader(`{"facilityId":""}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:12345"

	rr := httptest.NewRecorder()
	h.Generate(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}

	var resp map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	errMsg, _ := resp["error"].(string)
	if !strings.Contains(errMsg, "tidak lengkap") && !strings.Contains(errMsg, "wajib") {
		t.Errorf("expected Indonesian validation message, got: %s", errMsg)
	}
}

// TestGenerateQueueHandler_RateLimit_Returns429WithIndonesianMessage is the novelty requirement.
// It proves the anti-spam rate limiting is now based on (nomor HP + faskes) per hari,
// not just IP (so it works on public WiFi). Returns 429 + friendly Indonesian message.
func TestGenerateQueueHandler_RateLimit_Returns429WithIndonesianMessage(t *testing.T) {
	// Tight limit=1 for this test only, to trigger 429 on the 2nd request with same phone+facility.
	// In production main we use NewDailyLimiter(2).
	rl := limiter.NewDailyLimiter(1)
	svc := service.NewFakeQueueService()
	h := NewHandler(svc, rl)

	body := `{"facilityId":"f1","patient":{"fullName":"Test Pasien","phone":"081234567890"}}`
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/queues/generate", strings.NewReader(body))
	req1.Header.Set("Content-Type", "application/json")

	rr1 := httptest.NewRecorder()
	h.Generate(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("first request should succeed, got %d: %s", rr1.Code, rr1.Body.String())
	}

	// Second request with *same phone + same facilityId* must be rate limited (daily limit hit)
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/queues/generate", strings.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")

	rr2 := httptest.NewRecorder()
	h.Generate(rr2, req2)

	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 Too Many Requests, got %d. body: %s", rr2.Code, rr2.Body.String())
	}

	var resp map[string]any
	_ = json.Unmarshal(rr2.Body.Bytes(), &resp)
	errMsg, _ := resp["error"].(string)
	if !strings.Contains(errMsg, "2 antrean per hari") || !strings.Contains(errMsg, "fasilitas tersebut") {
		t.Errorf("expected Indonesian message mentioning '2 antrean per hari' and 'fasilitas tersebut', got: %s", errMsg)
	}
}
