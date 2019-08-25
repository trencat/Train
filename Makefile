.PHONY: build
build:
	go build github.com/trencat/train

.PHONY: docs
docs:
	# Docs available at http://localhost:6060/pkg/github.com/trencat/train/
	godoc -index -play -goroot=$(GOPATH)/src/github.com/trencat/train

.PHONY: install-deps
install-deps:
	go get -u github.com/google/go-cmp/cmp

# do not use test-clean directly. Intended only as dependency
.PHONY: test-clean
test-clean:
ifdef NOCACHE
		go clean -testcache
endif

.PHONY: test
test: test-clean
	go test github.com/trencat/train

.PHONY: test-core
test-core: test-clean
	go test github.com/trencat/train/core

.PHONY: test-core-update
test-core-update: test-clean
	go test github.com/trencat/train/core --update

.PHONY: test-atp
test-atp: test-clean
	go test github.com/trencat/train/atp
