package raptor

import (
	"slices"

	"router/pkg/types"
)

// Label is an internal RAPTOR reconstruction record. No json tags: nothing
// marshals it, and omitempty would drop a valid index 0.
type Label struct {
	RouteId          types.RouteID
	TripIdx          uint32
	BoardStopId      types.StopID
	BoardStopIdx     StopIndex
	AlightStopIdx    StopIndex
	TransferMode     types.TransferMode
	TransferToStopId types.StopID
	TransferWeight   types.DualWeight
}

func (l Label) IsTransfer() bool { return l.TransferMode != types.TransferModeNone }

func transitLabel(
	routeId types.RouteID,
	tripIdx uint32,
	boardStopId types.StopID,
	boardStopIdx, alightStopIdx StopIndex,
) Label {
	return Label{
		RouteId:          routeId,
		TripIdx:          tripIdx,
		BoardStopId:      boardStopId,
		BoardStopIdx:     boardStopIdx,
		AlightStopIdx:    alightStopIdx,
		TransferMode:     types.TransferModeNone,
		TransferToStopId: 0,
		TransferWeight:   types.DualWeight{RealTime: 0, PenalizedCost: 0},
	}
}

func transferLabel(from, to types.StopID, weight types.DualWeight, mode types.TransferMode) Label {
	return Label{
		RouteId:          0,
		TripIdx:          0,
		BoardStopId:      from,
		BoardStopIdx:     0,
		AlightStopIdx:    0,
		TransferMode:     mode,
		TransferToStopId: to,
		TransferWeight:   weight,
	}
}

type Journey struct {
	NumLegs     int
	ArrivalTime types.Timestamp
	Legs        []Label
}

const MAX_ROUNDS = 10

// MaxTransferSeconds caps one continuous connecting-leg segment (origin->first
// boarding, or between two transit legs). It bounds the per-round transfer
// relaxation so journeys can't chain unlimited walking. ~2-3 chained 500 m hops.
const MaxTransferSeconds = 900

// relaxTransfers runs a bounded worklist relaxation over the transfer graph,
// improving rounds[round][*] for every stop reachable from a seed. Positive
// weights + the strict best[] guard bound improvements, so it drains.
// Returns the improved stops (deduped).
func (rt *RaptorTable) relaxTransfers(
	round int,
	seeds []types.StopID,
	rounds [][]types.Timestamp,
	best []types.Timestamp,
	parents [][]Label,
) []types.StopID {
	numStops := rt.NumStops()
	queued := make([]bool, numStops)
	improvedSet := make([]bool, numStops)

	// walkAccum[s]: connecting-leg RealTime seconds accumulated on the path
	// that reached s. Seeds start at 0; a worklist-reached stop carries its
	// predecessor's accum + the edge weight.
	walkAccum := make([]uint32, numStops)

	queue := make([]types.StopID, 0, len(seeds))
	for _, s := range seeds {
		if !queued[s] {
			queued[s] = true
			walkAccum[s] = 0
			queue = append(queue, s)
		}
	}

	var improved []types.StopID

	for head := 0; head < len(queue); head++ {
		stopID := queue[head]
		queued[stopID] = false

		base := rounds[round][stopID]
		if base == types.INFINITY {
			continue
		}

		start := rt.Transfers.OffsetOfStop[stopID]
		end := rt.Transfers.OffsetOfStop[stopID+1]

		for idx := start; idx < end; idx++ {
			target := rt.Transfers.TransferTarget[idx]
			weight := rt.Transfers.TransferWeights[idx]

			// bound the continuous connecting-leg segment before anything else:
			// pruning here keeps journeys from chaining unlimited walking
			accum := walkAccum[stopID] + weight.RealTime
			if accum > MaxTransferSeconds {
				continue
			}

			arrival := base + types.Timestamp(weight.RealTime)

			if arrival >= best[target] {
				continue
			}

			rounds[round][target] = arrival
			best[target] = arrival
			walkAccum[target] = accum
			parents[round][target] = transferLabel(stopID, target, weight, rt.Transfers.TransferModes[idx])

			if !improvedSet[target] {
				improvedSet[target] = true
				improved = append(improved, target)
			}

			if !queued[target] {
				queued[target] = true
				queue = append(queue, target)
			}
		}
	}

	return improved
}

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

	// origin walks: relax transfers out of the start stop so round 1's boarding
	// search sees stops reachable on foot from the origin.
	originTransfers := rt.relaxTransfers(0, stopsUpdated, rounds, best, parents)
	stopsUpdated = append(stopsUpdated, originTransfers...)

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
		stopsUpdatedByRoute := make([]types.StopID, 0)
		routeImproved := make([]bool, numStops)

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

						parents[round][currStopId] = transitLabel(
							routeId,
							uint32(currentTripIdx),
							boardStopId,
							StopIndex(boardStopIdx),
							StopIndex(currStopIdx),
						)
						// we only need tp mark for next round if the is the best arrival time
						// over all rounds
						if arrivalTime < best[currStopId] {
							best[currStopId] = arrivalTime
							if !routeImproved[currStopId] {
								routeImproved[currStopId] = true
								stopsUpdatedByRoute = append(stopsUpdatedByRoute, currStopId)
							}
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

		// relax footpath transfers out of this round's transit-improved stops,
		// re-enqueueing improved targets so multi-hop A->B->C walk chains resolve
		// within the round.
		// TODO: use precomputed street network transfers to update
		// the best[s] and rounds[k][s] entries and add to nextStopsUpdated
		transferImproved := rt.relaxTransfers(round, stopsUpdatedByRoute, rounds, best, parents)

		// seed the next round with the updated stops for this round
		stopsUpdated = make([]types.StopID, 0, len(stopsUpdatedByRoute)+len(transferImproved))
		stopsUpdated = append(stopsUpdated, stopsUpdatedByRoute...)
		stopsUpdated = append(stopsUpdated, transferImproved...)
	}

	// if we never got to the destination, fail
	if best[end] == types.INFINITY {
		return nil
	}

	var pareto []Journey

	// reconstruct pareto frontier from parents
	for round := 1; round <= MAX_ROUNDS; round++ {
		if rounds[round][end] < rounds[round-1][end] {
			legs := reconstructLegs(start, end, round, numStops, parents)
			pareto = append(pareto, Journey{
				NumLegs:     len(legs),
				ArrivalTime: rounds[round][end],
				Legs:        legs,
			})
		}
	}

	return pareto
}

// reconstructLegs walks parents backwards from end to start, returning legs
// in forward order. Returns nil on corrupt parents (iteration cap or round<0
// guard hit) instead of a truncated, mis-starting slice.
func reconstructLegs(start, end types.StopID, round, numStops int, parents [][]Label) []Label {
	var legs []Label

	stop := end
	for i := 0; stop != start && i < numStops+MAX_ROUNDS; i++ {
		if round < 0 {
			break
		}

		label := parents[round][stop]
		legs = append(legs, label)
		stop = label.BoardStopId

		if !label.IsTransfer() {
			round--
		}
	}

	if stop != start {
		return nil // corrupt parents: didn't reconstruct to start
	}

	slices.Reverse(legs)

	return legs
}
