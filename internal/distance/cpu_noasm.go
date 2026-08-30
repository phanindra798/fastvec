//go:build !amd64 || purego

package distance

// Anywhere without the assembly, the scalar path is the only path.
const useAVX2 = false

func l2SquaredAVX2(a, b []float32) float32 { return L2SquaredScalar(a, b) }
