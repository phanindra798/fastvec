//go:build amd64 && !purego

#include "textflag.h"

// func l2SquaredAVX2(a, b []float32) float32
//
// Sum of (a[i]-b[i])^2, eight lanes at a time in a YMM register.
//
// Two accumulators rather than one. The multiply and the add each have several
// cycles of latency, so a single dependency chain stalls waiting on itself.
// Two independent chains keep the pipeline fed and cost one register.
//
// Frame: a is a slice header at 0, b at 24, the float32 result at 48.
TEXT ·l2SquaredAVX2(SB), NOSPLIT, $0-52
	MOVQ a_base+0(FP), SI
	MOVQ b_base+24(FP), DI
	MOVQ a_len+8(FP), CX

	VXORPS Y0, Y0, Y0
	VXORPS Y4, Y4, Y4

	MOVQ CX, BX
	SHRQ $4, BX            // how many 16-float blocks
	JZ   tail8

loop16:
	VMOVUPS (SI), Y1
	VMOVUPS (DI), Y2
	VSUBPS  Y2, Y1, Y1
	VMULPS  Y1, Y1, Y1
	VADDPS  Y1, Y0, Y0

	VMOVUPS 32(SI), Y5
	VMOVUPS 32(DI), Y6
	VSUBPS  Y6, Y5, Y5
	VMULPS  Y5, Y5, Y5
	VADDPS  Y5, Y4, Y4

	ADDQ $64, SI
	ADDQ $64, DI
	DECQ BX
	JNZ  loop16

	VADDPS Y4, Y0, Y0

tail8:
	MOVQ CX, BX
	ANDQ $15, BX
	SHRQ $3, BX            // one more 8-float block, or none
	JZ   reduce

	VMOVUPS (SI), Y1
	VMOVUPS (DI), Y2
	VSUBPS  Y2, Y1, Y1
	VMULPS  Y1, Y1, Y1
	VADDPS  Y1, Y0, Y0

	ADDQ $32, SI
	ADDQ $32, DI

reduce:
	VEXTRACTF128 $1, Y0, X1
	VADDPS       X1, X0, X0
	VHADDPS      X0, X0, X0
	VHADDPS      X0, X0, X0

	ANDQ $7, CX            // whatever is left, one at a time
	JZ   done

scalar:
	VMOVSS (SI), X2
	VMOVSS (DI), X3
	VSUBSS X3, X2, X2
	VMULSS X2, X2, X2
	VADDSS X2, X0, X0

	ADDQ $4, SI
	ADDQ $4, DI
	DECQ CX
	JNZ  scalar

done:
	VZEROUPPER
	MOVSS X0, ret+48(FP)
	RET
