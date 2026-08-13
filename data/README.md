# Datasets

Not committed. Download into /mnt/d/data.

SIFT1M and SIFT small: http://corpus-texmex.irisa.fr/

SIFT1M is 1M base vectors, 10k queries, 128 dimensions, Euclidean. Ground truth
ships with it (top 100 per query).

Note to self: I'm going to cut a 100k subset for faster testing. The ground
truth for that has to be recomputed with brute force. I can't just slice
SIFT1M's ground truth, because those neighbour IDs point into the full 1M set
and would be wrong for a subset.
