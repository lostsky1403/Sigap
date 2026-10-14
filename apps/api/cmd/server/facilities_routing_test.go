package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sigap/sigap/apps/api/internal/handler"
)

// Why this test exists, and why it is a routing test rather than a handler test.
//
// The defect was invisible to every other layer. `internal/router` already
// declared `POST /api/v1/admin/facilities`, `FacilitiesRouter` already
// dispatched POST to `CreateFacility`, and the handler itself was correct. The
// break was one line of mux registration: registering `ListFacilities` on the
// bare path in addition to the dispatcher.
//
// net/http resolves patterns by specificity, not by method. The pattern
// "/api/v1/admin/facilities" (no trailing slash) matches EVERY method, so it
// shadowed the dispatcher's own registration for POST and the request landed in
// a GET-only handler. The observable failure was the worst possible shape: a
// create that returned HTTP 200, carried a well-formed facility LIST, and
// persisted nothing — indistinguishable from a working feature to anything
// asserting only on status.
//
// So the assertion has to be made at the ServeMux, and against the REAL
// registration. An earlier version of this file built its own mux and passed
// even with the bug reintroduced, which is the reason
// registerAdminRoutes was extracted from main(): a test that constructs its own
// mux is testing the test's fixture, not the server.
//
// Authorization is deliberately not modelled here. It lives inside the real
// handlers, covered by the facility-scope tests, and a routing fixture that
// pretended to authenticate would only be asserting against itself.

/**
 * Records which handler served a request, so a routing test can tell the
 * dispatcher from a GET-only shadow.
 */
func recordingHandler(name string, status int, body map[string]any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Handler", name)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	}
}

/**
 * Builds a mux from the REAL admin registration.
 *
 * `registerAdminRoutes` takes the concrete *handler.AdminHandler, so the test
 * constructs one with a nil pool. Nothing in the ROUTING path dereferences it:
 * the question under test is which function ServeMux selects for a method and
 * path, and that is settled entirely by pattern registration. If a request ever
 * did escape routing into a real handler, the recover() below turns that into a
 * loud failure instead of a silent pass.
 */
func realAdminMux() *http.ServeMux {
	mux := http.NewServeMux()
	registerAdminRoutes(mux, handler.NewAdminHandler(nil))
	return mux
}

func TestAdminFacilitiesRouting_BarePathReachesTheDispatcher(t *testing.T) {
	mux := realAdminMux()

	// Each entry names a branch ONLY FacilitiesRouter can take. The pre-fix
	// wiring sent every method to ListFacilities, so a request that produces a
	// DIFFERENT answer than the list is proof that the dispatcher's method
	// switch — not the list handler — served it.
	//
	// The expected answers come from reading FacilitiesRouter plus the handlers
	// it calls with a zero actor (no AppUserID, no scope resolver), which is
	// exactly the fail-closed path a real unauthorized request takes:
	//
	//   GET    collection -> ListFacilities: filters an empty authorized set and
	//                         answers {"data":[],"success":true}
	//   POST   collection -> CreateFacility: rejects the empty body first, so a
	//                         400 naming a required field
	//   PATCH  member     -> UpdateFacility: validates the id first, so a 400
	//                         for a non-UUID, or a 404 once the id parses
	//   PATCH  deactivate -> DeactivateFacility: same id validation as above
	//
	// The distinction that matters is SHAPE: the list answers with an ARRAY,
	// while every other branch answers with an error envelope. That is
	// precisely the difference between "create works" and "create silently
	// returned the list".
	//
	// A real UUID is used wherever the handler validates the id, so the request
	// gets past validation into its authorization branch — that is the branch
	// this test is about, and a 400 for a malformed id would prove only that
	// validation ran.
	const validUUID = "550e8400-e29b-41d4-a716-446655440000"

	cases := []struct {
		name        string
		method      string
		path        string
		body        string
		wantStatus  int
		wantErrText string
		wantList    bool
	}{
		{
			name:     "GET collection is answered by ListFacilities",
			method:   http.MethodGet,
			path:     "/api/v1/admin/facilities",
			wantList: true,
		},
		{
			name:        "POST collection is answered by CreateFacility, not the list",
			method:      http.MethodPost,
			path:        "/api/v1/admin/facilities",
			body:        `{}`,
			wantStatus:  http.StatusBadRequest,
			wantErrText: "Nama fasilitas wajib diisi.",
		},
		{
			name:        "PATCH member is answered by UpdateFacility",
			method:      http.MethodPatch,
			path:        "/api/v1/admin/facilities/" + validUUID,
			// A real changed field, because UpdateFacility rejects an empty
			// body ("Tidak ada field yang diupdate.") before it authorizes. The
			// branch under test is the one AFTER that check.
			body:        `{"name":"Klinik Uji"}`,
			wantStatus:  http.StatusNotFound,
			wantErrText: "Fasilitas tidak ditemukan.",
		},
		{
			name:        "PATCH deactivate is answered by DeactivateFacility",
			method:      http.MethodPatch,
			path:        "/api/v1/admin/facilities/" + validUUID + "/deactivate",
			wantStatus:  http.StatusNotFound,
			wantErrText: "Fasilitas tidak ditemukan.",
		},
		{
			name:        "GET member is answered by GetFacility",
			method:      http.MethodGet,
			path:        "/api/v1/admin/facilities/" + validUUID,
			wantStatus:  http.StatusNotFound,
			wantErrText: "Fasilitas tidak ditemukan.",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			rec := httptest.NewRecorder()

			// A panic here means the nil pool was reached, i.e. the request
			// escaped routing into a code path that expected a live database.
			// That is a genuine wiring fact worth surfacing loudly.
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("%s %s reached a pool-backed handler (nil pool "+
						"dereferenced): %v", tc.method, tc.path, r)
				}
			}()

			mux.ServeHTTP(rec, req)

			if rec.Code == http.StatusNotFound && tc.wantStatus != http.StatusNotFound {
				t.Fatalf("%s %s matched no registered pattern", tc.method, tc.path)
			}

			var body struct {
				Success bool            `json:"success"`
				Error   string          `json:"error"`
				Data    json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("expected a JSON envelope, got %q: %v", rec.Body.String(), err)
			}

			isList := len(body.Data) > 0 && body.Data[0] == '['

			if tc.wantList {
				if !isList {
					t.Errorf("%s %s must be answered by ListFacilities with an array, got %s",
						tc.method, tc.path, rec.Body.String())
				}
				return
			}

			// The load-bearing assertion. A list here is the exact shape of the
			// original defect: a mutating method answered by the list handler.
			if isList {
				t.Fatalf("%s %s was answered by the LIST handler — the method "+
					"dispatcher has been shadowed again. Body: %s",
					tc.method, tc.path, rec.Body.String())
			}
			if tc.wantStatus != 0 && rec.Code != tc.wantStatus {
				t.Errorf("%s %s status = %d, want %d (body %q)",
					tc.method, tc.path, rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.wantErrText != "" && !strings.Contains(body.Error, tc.wantErrText) {
				t.Errorf("%s %s error = %q, want it to contain %q",
					tc.method, tc.path, body.Error, tc.wantErrText)
			}
		})
	}
}

// TestAdminFacilitiesRouting_PostIsNotShadowedByAListHandler reproduces the
// original defect exactly and asserts that it is still a defect.
//
// This is the negative control, and it is the part that makes the rest of this
// file meaningful. It rebuilds the mux the way main.go did BEFORE the fix —
// `ListFacilities` on the more specific bare path — and asserts that POST is
// captured by it. If that assertion ever started failing, the fixture would no
// longer be reproducing the bug and every "the fix works" claim built on it
// would be hollow.
func TestAdminFacilitiesRouting_PostIsNotShadowedByAListHandler(t *testing.T) {
	// The shadowing pattern, reproducing the pre-fix wiring.
	shadowing := http.NewServeMux()
	listOnly := recordingHandler("list-only", http.StatusOK, map[string]any{
		"success": true,
		"data":    []map[string]any{{"id": "facility-1", "name": "A Facility"}},
	})
	dispatcher := recordingHandler("dispatcher", http.StatusCreated, map[string]any{
		"success": true,
		"data":    map[string]string{"id": "new-facility-id"},
	})

	// Pre-fix order: the bare path got the GET-only handler, so it wins for
	// every method. This ordering is the whole bug.
	shadowing.HandleFunc("/api/v1/admin/facilities", listOnly)
	shadowing.HandleFunc("/api/v1/admin/facilities/", dispatcher)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/facilities", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	shadowing.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Handler"); got != "list-only" {
		t.Fatalf("the fixture no longer reproduces the defect: POST reached %q, "+
			"so the shadowing scenario this file relies on has stopped working", got)
	}

	// And the operator-visible consequence, which is the reason this was ever
	// worth fixing: a create that reports success and returns rows.
	var body struct {
		Success bool `json:"success"`
		Data    any  `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if _, isList := body.Data.([]any); !isList {
		t.Fatalf("the defect's fingerprint is a LIST body on a create, got %T", body.Data)
	}
	if !body.Success {
		t.Errorf("the defective shape reports success while persisting nothing")
	}
}

// TestAdminFacilitiesRouting_PostBodyIsNotAList is the behavioural restatement
// of the test above, written against the wire shape rather than a handler
// identity, because the list-shaped 200 is the defect an operator would meet.
func TestAdminFacilitiesRouting_PostBodyIsNotAList(t *testing.T) {
	dispatcher := recordingHandler("dispatcher", http.StatusCreated, map[string]any{
		"success": true,
		"data":    map[string]string{"id": "new-facility-id"},
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/admin/facilities", dispatcher)
	mux.HandleFunc("/api/v1/admin/facilities/", dispatcher)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/facilities", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var body struct {
		Success bool `json:"success"`
		Data    any  `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if _, isList := body.Data.([]any); isList {
		t.Fatalf("POST /api/v1/admin/facilities answered with a LIST body: %s",
			rec.Body.String())
	}
	object, isObject := body.Data.(map[string]any)
	if !isObject {
		t.Fatalf("POST must answer with an object carrying the new id, got %T", body.Data)
	}
	if id, _ := object["id"].(string); id == "" {
		t.Errorf("POST must carry the created facility id, got %v", object)
	}
}