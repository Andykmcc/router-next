package debug

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"router/pkg/raptor"
	"router/pkg/types"
)

type stopView struct {
	ID   types.StopID `json:"id"`
	Name string       `json:"name"`
	Lat  float64      `json:"lat"`
	Lon  float64      `json:"lon"`
}

type routeView struct {
	ShortName string `json:"shortName"`
	LongName  string `json:"longName"`
}

type legView struct {
	Kind            string     `json:"kind"`
	Route           *routeView `json:"route,omitempty"`
	From            stopView   `json:"from"`
	To              stopView   `json:"to"`
	TransferSeconds uint32     `json:"transferSeconds"`
}

type journeyView struct {
	ArrivalTime    types.Timestamp `json:"arrivalTime"`
	NumTransitLegs int             `json:"numTransitLegs"`
	Legs           []legView       `json:"legs"`
}

type routeResponse struct {
	Journeys []journeyView `json:"journeys"`
}

// nowClock returns the server host's current local time as seconds since
// midnight.
func nowClock() types.Timestamp {
	now := time.Now()

	return types.Timestamp(now.Hour()*3600 + now.Minute()*60 + now.Second())
}

// parseClock converts "HH:MM" or "HH:MM:SS" to seconds since midnight.
func parseClock(s string) (types.Timestamp, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 && len(parts) != 3 {
		return 0, fmt.Errorf("invalid time %q", s)
	}

	var secs int

	for i, p := range parts {
		v, err := strconv.Atoi(p)
		if err != nil || v < 0 {
			return 0, fmt.Errorf("invalid time %q", s)
		}

		switch i {
		case 0:
			secs += v * 3600
		case 1:
			secs += v * 60
		case 2:
			secs += v
		}
	}

	return types.Timestamp(secs), nil
}

func stopViewOf(rt *raptor.RaptorTable, id types.StopID) stopView {
	s := rt.Stops[id]

	return stopView{ID: id, Name: s.Name, Lat: s.Lat, Lon: s.Lon}
}

func buildRouteResponse(rt *raptor.RaptorTable, journeys []raptor.Journey) routeResponse {
	out := routeResponse{Journeys: make([]journeyView, 0, len(journeys))}

	for _, j := range journeys {
		jv := journeyView{ArrivalTime: j.ArrivalTime, NumTransitLegs: 0, Legs: make([]legView, 0, len(j.Legs))}

		for _, leg := range j.Legs {
			if leg.IsTransfer() {
				jv.Legs = append(jv.Legs, legView{
					Kind:            leg.TransferMode.String(),
					Route:           nil,
					From:            stopViewOf(rt, leg.BoardStopId),
					To:              stopViewOf(rt, leg.TransferToStopId),
					TransferSeconds: leg.TransferWeight.RealTime,
				})

				continue
			}

			jv.NumTransitLegs++
			gtfsRoute := rt.Routes[leg.RouteId]
			alightStopID := rt.StopsForRoute(leg.RouteId)[leg.AlightStopIdx]
			jv.Legs = append(jv.Legs, legView{
				Kind:            "transit",
				Route:           &routeView{ShortName: gtfsRoute.ShortName, LongName: gtfsRoute.LongName},
				From:            stopViewOf(rt, leg.BoardStopId),
				To:              stopViewOf(rt, alightStopID),
				TransferSeconds: 0,
			})
		}

		out.Journeys = append(out.Journeys, jv)
	}

	return out
}

func handleRoute(rt *raptor.RaptorTable) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start, err1 := strconv.ParseUint(r.URL.Query().Get("start"), 10, 32)
		end, err2 := strconv.ParseUint(r.URL.Query().Get("end"), 10, 32)

		// an absent/empty time defaults to the server host's local
		// time.Now() clock-time; only an unparseable non-empty time is a 400.
		timeParam := r.URL.Query().Get("time")
		clock := nowClock()

		var err3 error
		if timeParam != "" {
			clock, err3 = parseClock(timeParam)
		}

		if err1 != nil || err2 != nil || err3 != nil {
			http.Error(w, "start, end (uint) are required and time (HH:MM[:SS]) must be valid if given",
				http.StatusBadRequest)

			return
		}

		if int(start) >= rt.NumStops() || int(end) >= rt.NumStops() {
			http.Error(w, "start/end out of range", http.StatusBadRequest)

			return
		}

		journeys := rt.Route(types.StopID(start), types.StopID(end), clock)
		resp := buildRouteResponse(rt, journeys)

		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(resp); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}
