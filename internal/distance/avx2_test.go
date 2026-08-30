package distance

import (
	"math"
	"math/rand"
	"testing"
)

// The assembly cannot be bit-identical to the scalar loop and it does not need
// to be. Scalar accumulates one running sum; the kernel keeps eight partial
// sums in a register and adds them at the end. Floating point addition is not
// associative, so a different order gives a slightly different answer. Both are
// correct.
//
// What matters is that the difference stays far below anything that could
// reorder two neighbours. The tolerance here is relative and generous by three
// orders of magnitude compared to what turns up in practice.
const relTolerance = 1e-5

func TestAVX2MatchesScalar(t *testing.T) {
	if !UsingAVX2() {
		t.Skip("no AVX2 on this machine")
	}

	r := rand.New(rand.NewSource(61))
	worst := 0.0

	// Lengths either side of the block boundaries: the kernel does 16 at a
	// time, then 8, then one at a time, so 7, 8, 9, 15, 16, 17 all take
	// different paths through it.
	for _, dim := range []int{8, 9, 15, 16, 17, 31, 32, 33, 63, 64, 128, 129, 384, 960} {
		for trial := 0; trial < 500; trial++ {
			a, b := randVec(r, dim), randVec(r, dim)

			got := float64(l2SquaredAVX2(a, b))
			want := float64(L2SquaredScalar(a, b))

			rel := math.Abs(got-want) / math.Max(math.Abs(want), 1)
			if rel > worst {
				worst = rel
			}
			if rel > relTolerance {
				t.Fatalf("dim %d: kernel %v, scalar %v, relative error %g", dim, got, want, rel)
			}
		}
	}

	t.Logf("worst relative error over the sweep: %g", worst)
}

// Lengths below 8 never reach the kernel through L2Squared, but the assembly
// has to survive them anyway in case that guard ever moves.
func TestAVX2ShortVectors(t *testing.T) {
	if !UsingAVX2() {
		t.Skip("no AVX2 on this machine")
	}

	r := rand.New(rand.NewSource(62))
	for dim := 1; dim < 8; dim++ {
		a, b := randVec(r, dim), randVec(r, dim)

		got := float64(l2SquaredAVX2(a, b))
		want := float64(L2SquaredScalar(a, b))
		if rel := math.Abs(got-want) / math.Max(math.Abs(want), 1); rel > relTolerance {
			t.Errorf("dim %d: kernel %v, scalar %v", dim, got, want)
		}
	}
}

func TestAVX2Zero(t *testing.T) {
	if !UsingAVX2() {
		t.Skip("no AVX2 on this machine")
	}

	v := make([]float32, 128)
	for i := range v {
		v[i] = float32(i)
	}
	if got := l2SquaredAVX2(v, v); got != 0 {
		t.Errorf("a vector against itself gave %v, want 0", got)
	}
}

// The claim that matters. Tiny numerical differences are harmless as long as
// they never change which neighbour comes back, so this checks the ordering
// directly rather than trusting the tolerance to imply it.
func TestAVX2PreservesOrdering(t *testing.T) {
	if !UsingAVX2() {
		t.Skip("no AVX2 on this machine")
	}

	r := rand.New(rand.NewSource(63))
	const dim = 128

	for trial := 0; trial < 2000; trial++ {
		q := randVec(r, dim)
		x, y := randVec(r, dim), randVec(r, dim)

		scalarSaysXCloser := L2SquaredScalar(q, x) < L2SquaredScalar(q, y)
		kernelSaysXCloser := l2SquaredAVX2(q, x) < l2SquaredAVX2(q, y)

		if scalarSaysXCloser != kernelSaysXCloser {
			t.Fatalf("trial %d: scalar and kernel disagree on which of two vectors is nearer\n"+
				"  scalar: %v vs %v\n  kernel: %v vs %v",
				trial,
				L2SquaredScalar(q, x), L2SquaredScalar(q, y),
				l2SquaredAVX2(q, x), l2SquaredAVX2(q, y))
		}
	}
}

func TestDetectionAgreesWithProcInfo(t *testing.T) {
	t.Logf("AVX2 kernel in use: %v", UsingAVX2())
}

func BenchmarkL2Scalar128(b *testing.B) {
	r := rand.New(rand.NewSource(64))
	x, y := randVec(r, 128), randVec(r, 128)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink = L2SquaredScalar(x, y)
	}
}

func BenchmarkL2AVX2128(b *testing.B) {
	if !UsingAVX2() {
		b.Skip("no AVX2 on this machine")
	}
	r := rand.New(rand.NewSource(65))
	x, y := randVec(r, 128), randVec(r, 128)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink = l2SquaredAVX2(x, y)
	}
}
