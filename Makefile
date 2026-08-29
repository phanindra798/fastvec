GO   ?= go
DATA ?= /mnt/d/data

.PHONY: test race vet fmt build clean bench baseline plots subset

test:
	$(GO) test ./...

race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

fmt:
	$(GO) fmt ./...

build:
	mkdir -p bin
	$(GO) build -o bin/ ./cmd/...

clean:
	rm -rf bin

# Nothing else should be running on the machine while these go. They measure
# throughput, so anything competing for CPU shows up in the numbers.

subset:
	$(GO) run ./cmd/fastvec-subset -src $(DATA)/sift -dst $(DATA)/sift100k

# Reuses $(DATA)/sift.fvi when it exists, so a re-measure costs seconds
# instead of a 23 minute rebuild. Delete that file to force a fresh build.
bench:
	$(GO) run ./cmd/fastvec-bench -data $(DATA)/sift -name sift -index $(DATA)/sift.fvi

baseline:
	py/.venv/bin/python py/bench_baselines.py --data $(DATA)/sift --name sift

plots:
	py/.venv/bin/python py/plots.py
