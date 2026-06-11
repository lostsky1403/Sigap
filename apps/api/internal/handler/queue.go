package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sigap/sigap/apps/api/internal/events"
	"github.com/sigap/sigap/apps/api/internal/limiter"
	"github.com/sigap/sigap/apps/api/internal/service"
)

// Handler wires the rate limiter and queue service for the generate endpoint.
// Rate limiting is now based on (phone + facility) per calendar day to protect
// against abuse even on shared/public WiFi (e.g. 1 nomor HP max 2 antrean/hari/faskes).
// This replaces the previous pure-IP limiter for the registration action.
type Handler struct {
	svc     service.QueueService
	limiter *limiter.RateLimiter
}

// NewHandler creates a handler with the given dependencies.
func NewHandler(svc service.QueueService, rl *limiter.RateLimiter) *Handler {
	return &Handler{svc: svc, limiter: rl}
}

// Generate handles POST /api/v1/queues/generate
func (h *Handler) Generate(w http.ResponseWriter, r *http.Request) {
	// 1. Parse body first so we have the real identity (phone) + facility for rate limiting.
	var req GenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Format permintaan tidak valid.")
		return
	}

	// 2. Basic field presence check (before expensive rate limit or service call)
	if req.FacilityID == "" || req.Patient.Phone == "" {
		writeError(w, http.StatusBadRequest, "Data pasien tidak lengkap. Nomor telepon dan ID fasilitas wajib diisi.")
		return
	}

	// 3. Per-hari per-(HP + faskes) rate limit (anti-calo / anti-spam).
	// Key includes calendar date so it naturally resets every day.
	// Limit is enforced by the limiter instance (created with NewDailyLimiter(2) in main).
	today := time.Now().UTC().Format("2006-01-02")
	identityKey := today + ":" + req.Patient.Phone + ":" + req.FacilityID
	if !h.limiter.Allow(identityKey) {
		writeError(w, http.StatusTooManyRequests,
			"Nomor HP ini sudah mencapai batas maksimal 2 antrean per hari untuk fasilitas tersebut. Silakan coba lagi besok atau daftar di fasilitas lain.")
		return
	}

	// 4. Full validation (name is also required for a real registration)
	if req.Patient.FullName == "" {
		writeError(w, http.StatusBadRequest, "Data pasien tidak lengkap. Nama lengkap wajib diisi.")
		return
	}

	// 5. Call service (will later be the gRPC call to Rust engine)
	result, err := h.svc.Generate(r.Context(), service.GenerateInput{
		FacilityID: req.FacilityID,
		Patient: service.PatientInput{
			FullName:    req.Patient.FullName,
			Phone:       req.Patient.Phone,
			Gender:      req.Patient.Gender,
			DateOfBirth: req.Patient.DateOfBirth,
		},
	})
	if err != nil {
		if err == service.ErrValidation {
			writeError(w, http.StatusBadRequest, "Data pasien tidak lengkap. Nama lengkap, nomor telepon, dan ID fasilitas wajib diisi.")
			return
		}
		writeError(w, http.StatusInternalServerError, "Terjadi kesalahan sistem. Silakan coba lagi atau hubungi petugas.")
		return
	}

	// 6. Success — consistent {success, data} shape
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data":    result,
	})

	// 7. Real-time: notify SSE subscribers that a bed/queue state may have changed
	// (in real system this could carry actual new available count from engine).
	events.Bus.Publish(fmt.Sprintf(`{"facility_id":"%s","action":"queue_created"}`, req.FacilityID))
}

// GenerateRequest is the JSON body expected by the endpoint.
type GenerateRequest struct {
	FacilityID string `json:"facilityId"`
	Patient    struct {
		FullName    string `json:"fullName"`
		Phone       string `json:"phone"`
		Gender      string `json:"gender,omitempty"`
		DateOfBirth string `json:"dateOfBirth,omitempty"`
	} `json:"patient"`
}

// clientIP extracts a usable client identifier for rate limiting.
func clientIP(r *http.Request) string {
	// Respect common proxy headers (first value only)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := strings.Index(xff, ","); idx > 0 {
			return strings.TrimSpace(xff[:idx])
		}
		return xff
	}
	if xr := r.Header.Get("X-Real-IP"); xr != "" {
		return xr
	}
	return r.RemoteAddr
}

// writeError writes the standard error envelope with Indonesian user message.
func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": false,
		"error":   msg,
	})
}

// writeJSON is a small helper for success responses.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
