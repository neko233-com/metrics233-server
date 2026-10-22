package hnsw

import "math"

type DistanceFunc func(a, b []float32) float32

func EuclideanDistance(a, b []float32) float32 {
	var sum float64
	for i, value := range a {
		delta := float64(value) - float64(b[i])
		sum += delta * delta
	}
	return float32(math.Sqrt(sum))
}

func CosineDistance(a, b []float32) float32 {
	var dot, aa, bb float64
	for i, value := range a {
		x, y := float64(value), float64(b[i])
		dot += x * y
		aa += x * x
		bb += y * y
	}
	if aa == 0 || bb == 0 {
		if aa == bb {
			return 0
		}
		return 1
	}
	return float32(max(0, min(2, 1-dot/math.Sqrt(aa*bb))))
}
