package geo

import (
	"math"
	"testing"
)

func TestDistanceKnownRoute(t *testing.T) {
	// ~3 km apart latitudinally at the equator (0.027° lat ≈ 3.0 km).
	a := Point{Lat: 0, Lng: 0}
	b := Point{Lat: 0.027, Lng: 0}
	d := DistanceKm(a, b)
	if d < 2.9 || d > 3.1 {
		t.Fatalf("expected ~3.0 km, got %.2f", d)
	}
}

func TestDistanceZero(t *testing.T) {
	p := Point{Lat: 17.4126, Lng: 78.4632}
	if d := DistanceKm(p, p); d != 0 {
		t.Fatalf("same point must be 0 km, got %.4f", d)
	}
}

func TestEstimateDeterministic(t *testing.T) {
	d := NewDemo()
	a := Point{Lat: 17.4126, Lng: 78.4632}
	b := Point{Lat: 17.44, Lng: 78.48}
	e1 := d.Estimate(a, b)
	e2 := d.Estimate(a, b)
	if e1 != e2 {
		t.Fatal("demo estimates must be deterministic")
	}
	if !e1.Simulated {
		t.Fatal("demo estimates must be labelled simulated")
	}
	if e1.MinutesMin <= 0 || e1.MinutesMax < e1.MinutesMin {
		t.Fatalf("unreasonable ETA range %+v", e1)
	}
}

func TestSimulateJourneyEndpoints(t *testing.T) {
	a := Point{Lat: 1, Lng: 2}
	b := Point{Lat: 3, Lng: 4}
	if p := SimulateJourney(a, b, 0); p != a {
		t.Fatalf("t=0 must be origin, got %+v", p)
	}
	p := SimulateJourney(a, b, 1)
	if math.Abs(p.Lat-3) > 1e-9 || math.Abs(p.Lng-4) > 1e-9 {
		t.Fatalf("t=1 must be destination, got %+v", p)
	}
	mid := SimulateJourney(a, b, 0.5)
	if math.Abs(mid.Lat-2) > 1e-9 {
		t.Fatalf("midpoint lat should be 2, got %v", mid.Lat)
	}
	// out-of-range t is clamped
	if p := SimulateJourney(a, b, 9); p != b {
		t.Fatal("t>1 must clamp to destination")
	}
}
