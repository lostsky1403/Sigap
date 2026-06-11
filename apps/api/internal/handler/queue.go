package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/sigap/sigap/apps/api/internal/limiter"
	"github.com/sigap/sigap/apps/api/internal/service"
)

// Handler wires the rate limiter and queue service for the generate endpoint.
// Rate limiting is applied early as the first line of anti-spam protection
// for this public civic service (protects against bots and queue scalpers/calo).
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
	// 1. Rate limit check (anti-spam) — done before any business logic or DB work.
	// Key by remote IP (X-Forwarded-For respected for proxies in real deploys).
	clientIP := clientIP(r)
	if !h.limiter.Allow(clientIP) {
		writeError(w, http.StatusTooManyRequests, "Terlalu banyak permintaan dari alamat Anda. Silakan tunggu beberapa saat sebelum mencoba lagi.")
		return
	}

	// 2. Parse + basic validation
	var req GenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Format permintaan tidak valid.")
		return
	}

	if req.FacilityID == "" || req.Patient.FullName == "" || req.Patient.Phone == "" {
		writeError(w, http.StatusBadRequest, "Data pasien tidak lengkap. Nama lengkap, nomor telepon, dan ID fasilitas wajib diisi.")
		return
	}

	// 3. Call service (will later be the gRPC call to Rust engine)
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
		// Map known service errors (currently only validation in the fake impl)
		if err == service.ErrValidation {
			writeError(w, http.StatusBadRequest, "Data pasien tidak lengkap. Nama lengkap, nomor telepon, dan ID fasilitas wajib diisi.")
			return
		}
		writeError(w, http.StatusInternalServerError, "Terjadi kesalahan sistem. Silakan coba lagi atau hubungi petugas.")
		return
	}

	// 4. Success — consistent {success, data} shape
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data":    result,
	})
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
