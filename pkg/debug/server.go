package debug

import (
	"fmt"
	"net/http"
	"os"
	"router/pkg/gtfs"
	"router/pkg/raptor"
	"time"
)

func buildTables(gtfsPath string) (*gtfs.GTFSTable, *raptor.RaptorTable) {
	gtfsTable, err := gtfs.ParseGtfs(gtfsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v", err)
		os.Exit(1)
	}

	raptorTable, err := raptor.BuildRaptorTable(gtfsTable, gtfs.TimeToGTFSDate(time.Now()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v", err)
		os.Exit(1)
	}

	return gtfsTable, raptorTable
}

func StartServer(port string, gtfsPath string) {
	_, raptorTable := buildTables(gtfsPath)

	// Serve Static Files
	fs := http.FileServer(http.Dir("./static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	// Serve route stop ids: GET /routes/stops?routeId=<gtfs route_id>
	http.HandleFunc("/routes/stops", handleRouteStops(raptorTable, routeIndexByGtfsId(raptorTable.Routes)))

	// Serve enriched journeys: GET /route?start=<StopID>&end=<StopID>&time=HH:MM[:SS]
	http.HandleFunc("/route", handleRoute(raptorTable))

	// Serve Index
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.ServeFile(w, r, "static/index.html")
			return
		}

		fs.ServeHTTP(w, r)
	})

	fmt.Printf("Starting server on port %s \n", port)

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		fmt.Printf("Server failed: %v\n", err)
	}
}
