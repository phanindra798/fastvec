// Package distance holds the vector comparison functions. Hottest code in the
// project, so keep it boring.
package distance

// L2Squared returns the squared Euclidean distance between a and b, which must
// be the same length.
//
// No square root. It is monotonic, so the ranking comes out identical, and that
// saves an op on every comparison.
//
// Dispatches to an AVX2 kernel where the CPU has one. The branch is on a
// package variable set once at startup, so it predicts perfectly and costs
// nothing measurable next to the loop it guards.
func L2Squared(a, b []float32) float32 {
	if useAVX2 && len(a) >= 8 {
		return l2SquaredAVX2(a, b)
	}
	return L2SquaredScalar(a, b)
}

// L2SquaredScalar is the plain Go version. Exported because it is the reference
// the assembly gets checked against, and because it is what runs on anything
// that is not amd64.
func L2SquaredScalar(a, b []float32) float32 {
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

// UsingAVX2 reports whether the AVX2 kernel is in use. For benchmarks and for
// the environment block on a result file.
func UsingAVX2() bool { return useAVX2 }
