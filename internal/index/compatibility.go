package index

import (
	"fmt"
	"math"

	"github.com/phanindra798/fastvec/internal/fvecs"
)

// ValidateReuse checks the dataset and persisted construction settings before
// a benchmark reuses an index. Vector order matters because IDs are row numbers.
// EfSearch is a query setting and may change independently. BuildWorkers is not
// stored in format v1, so this cannot certify the original worker count.
func (h *HNSW) ValidateReuse(set *fvecs.Float, p Params) error {
	if set == nil || set.Dim != h.dim || len(set.Data) != len(h.data) {
		return fmt.Errorf("cached index dataset shape differs from requested dataset")
	}
	if p.M < 2 {
		return fmt.Errorf("requested M must be at least 2")
	}
	if p.MMax == 0 {
		p.MMax = p.M
	}
	if p.MMax0 == 0 {
		if p.M > int(^uint(0)>>1)/2 {
			return fmt.Errorf("requested M is too large to compute default MMax0")
		}
		p.MMax0 = 2 * p.M
	}
	for _, setting := range []struct {
		name              string
		cached, requested int
	}{
		{"M", h.m, p.M},
		{"MMax", h.mMax, p.MMax},
		{"MMax0", h.mMax0, p.MMax0},
		{"efConstruction", h.efConstruct, p.EfConstruct},
	} {
		if setting.cached != setting.requested {
			return fmt.Errorf("cached index %s=%d, requested %d", setting.name, setting.cached, setting.requested)
		}
	}
	if h.seed != p.Seed {
		return fmt.Errorf("cached index seed=%d, requested %d", h.seed, p.Seed)
	}
	if h.singleLayer != p.SingleLayer || h.nearestM != p.NearestM {
		return fmt.Errorf("cached index construction modes differ from requested modes")
	}
	// The file already contains every vector. Compare its exact bits directly
	// rather than introducing a sidecar or changing the file format for a hash.
	for i, value := range set.Data {
		if math.Float32bits(value) != math.Float32bits(h.data[i]) {
			return fmt.Errorf("cached index dataset differs at vector %d coordinate %d", i/h.dim, i%h.dim)
		}
	}
	return nil
}
