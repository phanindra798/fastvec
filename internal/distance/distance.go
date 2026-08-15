// Package distance holds the vector comparison functions. Hottest code in the
// project, so keep it boring.
package distance

// L2Squared returns the squared Euclidean distance between a and b, which must
// be the same length.
//
// No square root. It's monotonic, so the ranking comes out identical, and that
// saves an op on every comparison.
func L2Squared(a, b []float32) float32 {
	b = b[:len(a)] // lets the compiler drop the bounds check on b

	var sum float32
	for i := range a {
		d := a[i] - b[i]
		sum += d * d
	}
	return sum
}

// Dot returns the inner product of a and b, which must be the same length.
//
// Only equals cosine similarity when both are unit length. On unnormalised
// vectors it ranks by magnitude as well as direction, which looks fine and
// returns the wrong neighbours.
func Dot(a, b []float32) float32 {
	b = b[:len(a)]

	var sum float32
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}
