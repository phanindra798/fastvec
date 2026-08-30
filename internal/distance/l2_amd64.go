//go:build amd64 && !purego

package distance

//go:noescape
func l2SquaredAVX2(a, b []float32) float32
