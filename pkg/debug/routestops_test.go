package debug

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRouteIndexByGtfsId(t *testing.T) {
	rt := miniTable()
	idx := routeIndexByGtfsId(rt.Routes)

	if routeId, ok := idx["R"]; !ok || routeId != 0 {
		t.Fatalf(`routeIndexByGtfsId["R"] = %v, %v; want 0, true`, routeId, ok)
	}

	if routeId, ok := idx["EM:Shell/Pow"]; !ok || routeId != 1 {
		t.Fatalf(`routeIndexByGtfsId["EM:Shell/Pow"] = %v, %v; want 1, true`, routeId, ok)
	}
}

func TestHandleRouteStopsOK(t *testing.T) {
	rt := miniTable()
	h := handleRouteStops(rt, routeIndexByGtfsId(rt.Routes))

	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/routes/stops?routeId=R", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	if got, want := strings.TrimSpace(rec.Body.String()), "[0,1]"; got != want {
		t.Errorf("body = %s, want %s", got, want)
	}
}

func TestHandleRouteStopsSlashInId(t *testing.T) {
	rt := miniTable()
	h := handleRouteStops(rt, routeIndexByGtfsId(rt.Routes))

	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/routes/stops?routeId=EM:Shell/Pow", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	if got, want := strings.TrimSpace(rec.Body.String()), "[2]"; got != want {
		t.Errorf("body = %s, want %s", got, want)
	}
}

func TestHandleRouteStopsUnknown(t *testing.T) {
	rt := miniTable()
	h := handleRouteStops(rt, routeIndexByGtfsId(rt.Routes))

	cases := []string{
		"/routes/stops",              // missing routeId
		"/routes/stops?routeId=",     // empty routeId
		"/routes/stops?routeId=nope", // unknown routeId
	}

	for _, target := range cases {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(http.MethodGet, target, nil))

		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", target, rec.Code)
		}
	}
}
