package debug

import (
	"encoding/json"
	"net/http"

	"router/pkg/gtfs"
	"router/pkg/raptor"
	"router/pkg/types"
)

// routeIndexByGtfsId maps each route's GTFS route_id (e.g. "EM:Shell/Pow",
// "SF:15") to its RAPTOR-internal RouteID, built once at server startup.
func routeIndexByGtfsId(routes []gtfs.GTFSRoute) map[gtfs.GTFSRouteID]types.RouteID {
	byGtfsId := make(map[gtfs.GTFSRouteID]types.RouteID, len(routes))

	for i, route := range routes {
		byGtfsId[route.GtfsId] = types.RouteID(i)
	}

	return byGtfsId
}

// handleRouteStops serves GET /routes/stops?routeId=<gtfs route_id>, returning
// that route's ordered stop ids. routeId is a query param because GTFS route
// ids can contain "/".
func handleRouteStops(rt *raptor.RaptorTable, byGtfsId map[gtfs.GTFSRouteID]types.RouteID) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gtfsId := r.URL.Query().Get("routeId")

		routeId, ok := byGtfsId[gtfs.GTFSRouteID(gtfsId)]
		if gtfsId == "" || !ok {
			http.Error(w, "unknown route id", http.StatusBadRequest)

			return
		}

		stopIds := rt.StopsForRoute(routeId)

		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(stopIds); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}
