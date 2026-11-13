.PHONY: cover

# Line coverage of Go and frontend together, 80% threshold; report in coverage/.
cover:
	scripts/coverage.sh
