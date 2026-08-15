package distance

import (
	"math"
	"math/rand"
	"testing"
)

// float64 references. Slow, but obviously right, so the float32 versions get
// checked against these instead of against numbers I typed in myself.
func refL2(a, b []float32) float64 {
	var sum float64
	for i := range a {
		d := float64(a[i]) - float64(b[i])
		sum += d * d
	}
	return sum
}

func refDot(a, b []float32) float64 {
	var sum float64
	for i := range a {
		sum += float64(a[i]) * float64(b[i])
	}
	return sum
}

func randVec(r *rand.Rand, dim int) []float32 {
	v := make([]float32, dim)
	for i := range v {
		v[i] = r.Float32()*200 - 100
	}
	return v
}

func TestL2SquaredAgainstReference(t *testing.T) {
	r := rand.New(rand.NewSource(1))

	for _, dim := range []int{1, 2, 7, 8, 16, 128, 960} {
		for trial := 0; trial < 200; trial++ {
			a, b := randVec(r, dim), randVec(r, dim)

			got := float64(L2Squared(a, b))
			want := refL2(a, b)

			// float32 accumulation drifts from float64, and the drift grows
			// with the number of terms, so the tolerance is relative.
			if math.Abs(got-want) > 1e-4*math.Abs(want) {
				t.Fatalf("dim %d: got %v, want %v", dim, got, want)
			}
		}
	}
}

func TestDotAgainstReference(t *testing.T) {
	r := rand.New(rand.NewSource(2))

	for _, dim := range []int{1, 3, 128, 384} {
		for trial := 0; trial < 200; trial++ {
			a, b := randVec(r, dim), randVec(r, dim)

			got := float64(Dot(a, b))
			want := refDot(a, b)

			// Dot products of mixed sign vectors cancel, so a relative
			// tolerance is meaningless when the answer is near zero. Scale by
			// the size of the inputs instead.
			var scale float64
			for i := range a {
				scale += math.Abs(float64(a[i]) * float64(b[i]))
			}
			if math.Abs(got-want) > 1e-4*scale {
				t.Fatalf("dim %d: got %v, want %v", dim, got, want)
			}
		}
	}
}

func TestL2SquaredIdenticalVectorsIsZero(t *testing.T) {
	v := []float32{1.5, -2, 300, 0}
	if got := L2Squared(v, v); got != 0 {
		t.Errorf("L2Squared(v, v) = %v, want 0", got)
	}
}

func TestL2SquaredIsSymmetric(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	a, b := randVec(r, 128), randVec(r, 128)

	if L2Squared(a, b) != L2Squared(b, a) {
		t.Error("L2Squared is not symmetric")
	}
}

func BenchmarkL2Squared128(b *testing.B) {
	r := rand.New(rand.NewSource(4))
	x, y := randVec(r, 128), randVec(r, 128)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink = L2Squared(x, y)
	}
}

// Package level so the compiler cannot decide the benchmark's work is unused
// and delete the call.
var sink float32
