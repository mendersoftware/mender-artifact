GO ?= go
GOFMT ?= gofmt
V ?=
PKGS = $(shell go list ./... | grep -v vendor)
BUILDFILES = $(shell find cli/mender-artifact \( -path ./vendor -o -path ./Godeps \) -prune \
	                     -o -type f -name '*.go' -print |  tr ' ' '\n' | grep -v _test.go)
PKGNAME = mender-artifact
PKGFILES = $(shell find . \( -path ./vendor -o -path ./Godeps \) -prune \
		-o -type f -name '*.go' -print)
PKGFILES_notest = $(shell echo $(PKGFILES) | tr ' ' '\n' | grep -v _test.go)
GOCYCLO ?= 15

CGO_ENABLED=1
export CGO_ENABLED

TOOLS = \
	github.com/fzipp/gocyclo \
	github.com/opennota/check/cmd/varcheck \
	github.com/mendersoftware/deadcode

VERSION = $(shell git describe --tags --dirty --exact-match 2>/dev/null || git rev-parse --short HEAD)

GO_LDFLAGS = \
	-ldflags "-X main.Version=$(VERSION)"

ifeq ($(V),1)
BUILDV = -v
endif

TAGS =
ifeq ($(LOCAL),1)
TAGS += local
endif

ifneq ($(TAGS),)
BUILDTAGS = -tags '$(TAGS)'
endif

build:
	$(GO) build $(GO_LDFLAGS) $(BUILDV) $(BUILDTAGS) -o $(PKGNAME) $(BUILDFILES)  

install:
	$(GO) install $(GO_LDFLAGS) $(BUILDV) $(BUILDTAGS)

clean:
	$(GO) clean
	rm -f coverage.txt coverage-tmp.txt

get-tools:
	set -e ; for t in $(TOOLS); do \
		echo "-- go getting $$t"; \
		go get -u $$t; \
	done

check: test extracheck

test:
	$(GO) test -v $(PKGS)

extracheck:
	echo "-- checking if code is gofmt'ed"
	if [ -n "$$($(GOFMT) -d $(PKGFILES))" ]; then \
		echo "-- gofmt check failed"; \
		echo $GOFMT -d $PKGFILES \
		/bin/false; \
	fi
	echo "-- checking with govet"
	$(GO) tool vet -unsafeptr=false $(PKGFILES_notest)
	echo "-- checking for dead code"
	deadcode 
	echo "-- checking with varcheck"
	varcheck .
	echo "-- checking cyclometric complexity > $(GOCYCLO)"
	gocyclo -over $(GOCYCLO) $(PKGFILES_notest)

cover: coverage
	$(GO) tool cover -func=coverage.txt

htmlcover: coverage
	$(GO) tool cover -html=coverage.txt

coverage:
	rm -f coverage.txt
	echo 'mode: set' > coverage.txt
	set -e ; for p in $(PKGS); do \
		rm -f coverage-tmp.txt;  \
		$(GO) test -coverprofile=coverage-tmp.txt $$p ; \
		if [ -f coverage-tmp.txt ]; then \
			cat coverage-tmp.txt |grep -v 'mode:' | cat >> coverage.txt; \
		fi; \
	done
	rm -f coverage-tmp.txt

.PHONY: build clean get-tools test check \
	cover htmlcover coverage
