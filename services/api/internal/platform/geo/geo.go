// Package geo is the maps/geolocation provider abstraction. The demo adapter
// computes deterministic approximate (straight-line) distances between seeded
// coordinates and generates a simulated rider journey. It never invents live
// traffic, turn-by-turn directions or real GPS fixes, and every consumer of
// demo data must label it as simulated.
package geo

import "math"

// Point is a lat/lng pair in decimal degrees.
type Point struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

// DistanceKm returns approximate great-circle distance. Callers must present
// this as an ESTIMATE, never as exact road distance.
func DistanceKm(a, b Point) float64 {
	const earthRadiusKm = 6371.0
	dLat := radians(b.Lat - a.Lat)
	dLng := radians(b.Lng - a.Lng)
	lat1 := radians(a.Lat)
	lat2 := radians(b.Lat)
	h := sin2(dLat/2) + cos(lat1)*cos(lat2)*sin2(dLng/2)
	return 2 * earthRadiusKm * math.Asin(math.Sqrt(h))
}

func radians(d float64) float64 { return d * math.Pi / 180 }
func sin2(x float64) float64    { s := math.Sin(x); return s * s }
func cos(x float64) float64     { return math.Cos(x) }

// RouteEstimate is the honest output of the demo adapter.
type RouteEstimate struct {
	DistanceKm    float64 `json:"distance_km"`    // straight-line estimate
	TrafficFactor float64 `json:"traffic_factor"` // deterministic multiplier (demo)
	MinutesMin    int     `json:"minutes_min"`
	MinutesMax    int     `json:"minutes_max"`
	Simulated     bool    `json:"simulated"`
}

// DemoAdapter is the local maps provider. Deterministic: same inputs always
// yield the same outputs, so tests and demos are reproducible.
type DemoAdapter struct{}

func NewDemo() *DemoAdapter { return &DemoAdapter{} }

func (d *DemoAdapter) Estimate(a, b Point) RouteEstimate {
	direct := DistanceKm(a, b)
	// Deterministic road-ish factor: 1.3x straight-line + fixed handling time.
	road := direct * 1.3
	minutes := road / 18.0 * 60 // ~18 km/h city average for two-wheelers
	var lo, hi int
	if minutes < 1 {
		lo = 1
	} else {
		lo = int(minutes)
	}
	hi = lo + 4
	return RouteEstimate{
		DistanceKm:    math.Round(road*10) / 10,
		TrafficFactor: 1.0,
		MinutesMin:    lo,
		MinutesMax:    hi,
		Simulated:     true,
	}
}

// SimulateJourney returns a deterministic sequence of points along the
// straight line from a to b at fraction t ∈ [0,1]. Labelled as demo movement.
func SimulateJourney(a, b Point, t float64) Point {
	t = clamp01(t)
	return Point{
		Lat: a.Lat + (b.Lat-a.Lat)*t,
		Lng: a.Lng + (b.Lng-a.Lng)*t,
	}
}

func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}
