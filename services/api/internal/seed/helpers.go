package seed

import "math/rand"

// Deterministic helpers shared by all seed phases.

func pick(r *rand.Rand, options ...string) string {
	return options[r.Intn(len(options))]
}

func pickWeighted(r *rand.Rand, prob float64, a, b string) string {
	if r.Float64() < prob {
		return a
	}
	return b
}

func jitter(r *rand.Rand, amount float64) float64 {
	return (r.Float64()*2 - 1) * amount
}

func pickInt(r *rand.Rand, options ...int) int {
	return options[r.Intn(len(options))]
}

// r2 rounds to 2 decimals to keep seeded coordinates tidy.
func r2(v float64) float64 { return float64(int(v*100)) / 100 }
