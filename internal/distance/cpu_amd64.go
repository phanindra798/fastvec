//go:build amd64 && !purego

package distance

// Feature detection done here rather than through golang.org/x/sys/cpu, which
// would be the only third-party dependency in the project.
//
// Three things have to hold before YMM registers are safe to touch:
//
//	the CPU reports AVX2         CPUID leaf 7, EBX bit 5
//	the CPU reports AVX          CPUID leaf 1, ECX bit 28
//	the OS saves YMM on switch   OSXSAVE set, then XGETBV bits 1 and 2
//
// The last one is the trap. A CPU can advertise AVX2 while the OS has not
// enabled the wider register state, and using YMM then corrupts registers
// across a context switch rather than faulting cleanly.

//go:noescape
func cpuid(eaxArg, ecxArg uint32) (eax, ebx, ecx, edx uint32)

//go:noescape
func xgetbv() (eax, edx uint32)

var useAVX2 = detectAVX2()

func detectAVX2() bool {
	maxLeaf, _, _, _ := cpuid(0, 0)
	if maxLeaf < 7 {
		return false
	}

	const (
		bitAVX     = 1 << 28
		bitOSXSAVE = 1 << 27
	)
	_, _, ecx1, _ := cpuid(1, 0)
	if ecx1&(bitAVX|bitOSXSAVE) != bitAVX|bitOSXSAVE {
		return false
	}

	// Bit 1 is XMM state, bit 2 is YMM state. Both must be enabled.
	eax, _ := xgetbv()
	if eax&0x6 != 0x6 {
		return false
	}

	const bitAVX2 = 1 << 5
	_, ebx7, _, _ := cpuid(7, 0)
	return ebx7&bitAVX2 != 0
}
