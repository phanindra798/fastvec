package index

import (
	"bytes"
	"math"
	"testing"

	"github.com/phanindra798/fastvec/internal/fvecs"
)

func TestSaveRejectsUnrepresentableParameters(t *testing.T) {
	values := []int{-1}
	if largest := int(^uint(0) >> 1); uint64(largest) > math.MaxUint32 {
		values = append(values, largest)
	}
	for _, value := range values {
		for _, field := range []string{"M", "MMax", "MMax0", "efConstruction", "efSearch"} {
			h, err := BuildHNSW(&fvecs.Float{Dim: 1, Data: []float32{0, 1}}, DefaultParams())
			if err != nil {
				t.Fatal(err)
			}
			switch field {
			case "M":
				h.m = value
			case "MMax":
				h.mMax = value
			case "MMax0":
				h.mMax0 = value
			case "efConstruction":
				h.efConstruct = value
			case "efSearch":
				h.SetEfSearch(value)
			}
			var output bytes.Buffer
			if err := h.Save(&output); err == nil {
				t.Fatalf("Save accepted %s=%d", field, value)
			}
			if output.Len() != 0 {
				t.Fatal("Save wrote bytes before validating parameters")
			}
		}
	}
}
