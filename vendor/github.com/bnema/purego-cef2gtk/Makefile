.PHONY: fmt fmt-check vet test build check ci

# format source files (mutating)
fmt:
	go fmt ./...

# check formatting without mutating files
fmt-check:
	@files="$$(gofmt -l .)" || exit $$?; \
	if [ -n "$$files" ]; then \
		echo "Unformatted files:"; \
		echo "$$files"; \
		exit 1; \
	fi

vet:
	go vet ./...

test:
	go test ./...

build:
	go build ./...

# non-mutating verification target
check: fmt-check vet test build

# CI alias (non-mutating)
ci: check
