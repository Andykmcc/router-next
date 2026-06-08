package raptor

import (
	"slices"

	mapset "github.com/deckarep/golang-set/v2"

	"router/pkg/types"
)

type Label struct {
	RouteId       types.RouteID
	TripIdx       uint32
	BoardStopId   types.StopID
	BoardStopIdx  StopIndex
	AlightStopIdx StopIndex
}

type Journey struct {
	NumLegs     int
	ArrivalTime types.Timestamp
	Legs        []Label
}

const MAX_ROUNDS = 10

func (rt *RaptorTable) Route(start types.StopID, end types.StopID, startTime types.Timestamp) []Journey {
	numStops := rt.NumStops()

	// parents[k][s]: the label preceding rounds[k][s] for route reconstruction
	parents := make([][]Label, MAX_ROUNDS+1)

	// rounds[k][s]: best arrival at stop s with k or fewer transit legs
	rounds := make([][]types.Timestamp, MAX_ROUNDS+1)
	for k := range rounds {
		parents[k] = make([]Label, numStops)

		rounds[k] = make([]types.Timestamp, numStops)
		for s := range rounds[k] {
			rounds[k][s] = types.INFINITY
		}
	}

	rounds[0][start] = startTime

	// best[s]: the best arrival time for stop s over all rounds for cross-round pruning
	best := make([]types.Timestamp, numStops)
	for i := range best {
		best[i] = types.INFINITY
	}

	best[start] = startTime

	// we store which stops were updated in round k-1 so we can check if
	// they enable new connections in round k
	stopsUpdated := []types.StopID{start}

	for round := 1; round <= MAX_ROUNDS; round++ {
		// if no stops were updated last round, we are done
		if len(stopsUpdated) == 0 {
			break
		}

		// we start with the arrival times from the previous round
		copy(rounds[round], rounds[round-1])

		// for each route serving an updated stop, we remember the earliest
		// updated stop index in that route so we can board ASAP
		routeEarliestStop := make(map[types.RouteID]StopIndex)

		for _, stopId := range stopsUpdated {
			for _, segment := range rt.RoutesForStop(stopId) {
				existing, ok := routeEarliestStop[segment.RouteId]
				if !ok || segment.StopIndex < existing {
					routeEarliestStop[segment.RouteId] = segment.StopIndex
				}
			}
		}

		// the stops updated in this round to provide the updated set for the next round
		stopsUpdatedByRoute := mapset.NewSet[types.StopID]()

		// traverse each route left to right
		for routeId, firstStopIdx := range routeEarliestStop {
			stopsForRoute := rt.StopsForRoute(routeId)

			// the current trip we're on for the route
			// we might discover we can board an earlier trip
			// and then we will switch to that
			currentTripIdx := -1
			boardStopId := types.StopID(0)
			boardStopIdx := uint32(0)

			for currStopIdx := int(firstStopIdx); currStopIdx < len(stopsForRoute); currStopIdx++ {
				currStopId := stopsForRoute[currStopIdx]

				// arrival: if we're on a trip, check if we can get to any stop earlier than we could before
				if currentTripIdx >= 0 {
					arrivalTime := rt.StopEventsForTrip(routeId, uint32(currentTripIdx))[currStopIdx].ArrivalTime
					if arrivalTime < rounds[round][currStopId] && arrivalTime < best[end] {
						rounds[round][currStopId] = arrivalTime

						parents[round][currStopId] = Label{
							RouteId:       routeId,
							TripIdx:       uint32(currentTripIdx),
							BoardStopId:   boardStopId,
							BoardStopIdx:  StopIndex(boardStopIdx),
							AlightStopIdx: StopIndex(currStopIdx),
						}
						// we only need tp mark for next round if the is the best arrival time
						// over all rounds
						if arrivalTime < best[currStopId] {
							best[currStopId] = arrivalTime
							stopsUpdatedByRoute.Add(currStopId)
						}
					}
				}

				// if we haven't ever arrived at this stop before
				// we don't need to consider boarding a new trip here
				prevArrival := rounds[round-1][currStopId]
				if prevArrival == types.INFINITY {
					continue
				}

				// boarding: check if we can catch an earlier trip by boarding here
				earliestBoard := prevArrival + types.Timestamp(rt.MinTransferTime[currStopId])

				// check each trip for the first one departing after our arrival
				numTrips := rt.NumTripsInRoute[routeId]
				for potentialTripIdx := range numTrips {
					stopEvents := rt.StopEventsForTrip(routeId, potentialTripIdx)
					if stopEvents[currStopIdx].DepartureTime >= earliestBoard {
						if currentTripIdx < 0 || potentialTripIdx < uint32(currentTripIdx) {
							currentTripIdx = int(potentialTripIdx)
							boardStopId = currStopId
							boardStopIdx = uint32(currStopIdx)
						}

						break
					}
				}
			}
		}

		stopsUpdatedByTransfer := mapset.NewSet[types.StopID]()

		// interate over the stops updates and add all footpath transfers
		// if that improves the best time for the transfer target
		for stopId := range stopsUpdatedByRoute.Iter() {
			offset := rt.Transfers.OffsetOfStop[stopId]

			nextOffset := rt.Transfers.OffsetOfStop[stopId+1]
			for idx := offset; idx < nextOffset; idx++ {
				target := rt.Transfers.TransferTarget[idx]

				arrival := rounds[round][stopId] + types.Timestamp(rt.Transfers.TransferWeights[idx].RealTime)
				if arrival < best[target] {
					rounds[round][target] = arrival
					best[target] = arrival
					stopsUpdatedByTransfer.Add(target)
				}
			}
		}

		// seed the next round with the updated stops for this round
		stopsUpdated = stopsUpdatedByRoute.Union(stopsUpdatedByTransfer).ToSlice()
	}

	// if we never got to the destination, fail
	if best[end] == types.INFINITY {
		return nil
	}

	var pareto []Journey

	// reconstruct pareto frontier from parents
	for round := 1; round <= MAX_ROUNDS; round++ {
		if rounds[round][end] < rounds[round-1][end] {
			pareto = append(pareto, Journey{
				NumLegs:     round,
				ArrivalTime: rounds[round][end],
				Legs:        reconstructLegs(start, end, round, parents),
			})
		}
	}

	return pareto
}

func reconstructLegs(start, end types.StopID, round int, parents [][]Label) []Label {
	var legs []Label

	stop := end
	for round > 0 && stop != start {
		label := parents[round][stop]
		legs = append(legs, label)
		stop = label.BoardStopId
		round--
	}

	slices.Reverse(legs)

	return legs
}
