GO ?= go
GOFMT ?= gofmt
V ?=
PREFIX ?= /usr/local
PKGS = $(shell go list ./... | tr '\n' ',' | head -c -1)
PKGNAME = mender-artifact
PKGFILES = $(shell find . \( -path ./vendor -o -path ./Godeps \) -prune \
		-o -type f -name '*.go' -print)
PKGFILES_notest = $(shell echo $(PKGFILES) | tr ' ' '\n' | grep -v _test.go)
GOCYCLO ?= 20

GOARCH ?= $(shell go env GOARCH)
GOOS ?= $(shell go env GOOS)

CGO_ENABLED=0
export CGO_ENABLED

TOOLS = \
	github.com/fzipp/gocyclo/... \
	github.com/opennota/check/cmd/varcheck \
	github.com/mendersoftware/deadcode \
	github.com/mendersoftware/gobinarycoverage

VERSION = $(shell git describe --tags --dirty --exact-match 2>/dev/null || git rev-parse --short HEAD)

GO_LDFLAGS = \
	-ldflags "-X github.com/mendersoftware/mender-artifact/cli.Version=$(VERSION)"

ifeq ($(V),1)
BUILDV = -v
endif

TAGS ?=
ifneq ($(GOOS),linux)
	TAGS += nopkcs11
endif

build:
	$(GO) build $(GO_LDFLAGS) $(BUILDV) -tags '$(TAGS)'

PLATFORMS := darwin linux windows

$(PKGNAME)-%:
	env CGO_ENABLED=$(CGO_ENABLED) \
		GOARCH=$(GOARCH) \
		GOOS=$(GOOS) \
		go build \
		    -a $(GO_LDFLAGS) $(BUILDV) -tags '$(TAGS)' \
		    -o $@

.nopkcs11:
	$(warning "WARNING: Building without pkcs11 support")

build-native-linux: $(PKGNAME)-linux

build-native-mac: GOOS = darwin
build-native-mac: TAGS = nopkcs11
build-native-mac: CGO_ENABLED = 0
build-native-mac: .nopkcs11 $(PKGNAME)-darwin

build-native-windows: GOOS = windows
build-native-windows: TAGS = nopkcs11
build-native-windows: GO_LDFLAGS = -ldflags "-X github.com/mendersoftware/mender-artifact/cli.Version=44d6905 -linkmode=internal -s -w -extldflags '-static' -extld=x86_64-w64-mingw32-gcc"
build-native-windows: .nopkcs11 $(PKGNAME)-windows.exe

build-natives: build-native-linux build-native-mac build-native-windows

build-contained:
	rm -f mender-artifact && \
	image_id=$$(docker build -f Dockerfile -q .) && \
	docker run --rm --entrypoint "/bin/sh" -v $(shell pwd):/binary $$image_id -c "cp /go/bin/mender-artifact /binary" && \
	docker image rm $$image_id

build-natives-contained:
	rm -f mender-artifact-darwin mender-artifact-linux mender-artifact-windows.exe && \
	image_id=$$(docker build -f Dockerfile.binaries -q .) && \
	docker run --rm --entrypoint "/bin/sh" -v $(shell pwd):/binary $$image_id -c "cp /go/bin/mender-artifact* /binary" && \
	docker image rm $$image_id

install:
	@$(GO) install $(GO_LDFLAGS) $(BUILDV) $(BUILDTAGS)

install-autocomplete-scripts:
	@echo "Installing Bash auto-complete script into $(DESTDIR)/etc/bash_completion.d/"
	@install -Dm0644 ./autocomplete/bash_autocomplete $(DESTDIR)/etc/bash_completion.d/mender-artifact
	@if which zsh >/dev/null 2>&1 ; then \
		echo "Installing zsh auto-complete script into $(DESTDIR)$(PREFIX)/share/zsh/site-functions/" && \
		install -Dm0644 ./autocomplete/zsh_autocomplete $(DESTDIR)$(PREFIX)/share/zsh/site-functions/_mender-artifact \
	; fi


clean:
	$(GO) clean
	rm -f mender-artifact-darwin mender-artifact-linux mender-artifact-windows.exe
	rm -f coverage.txt coverage-tmp.txt

get-tools:
	set -e ; for t in $(TOOLS); do \
		echo "-- go getting $$t"; \
		go get -u $$t; \
	done
	go mod vendor

get-build-deps:
	apt-get update -qq
	apt-get install -yyq $(shell cat deb-requirements.txt)

check: test extracheck

tooldep:
	echo "check if mtools is installed on the system"
	mtools --version

test: tooldep
	$(GO) test -v $(PKGS)

extracheck:
	echo "-- checking if code is gofmt'ed"
	@if [ -n "$$($(GOFMT) -d $(PKGFILES))" ]; then \
		"$$($(GOFMT) -d $(PKGFILES))" \
		echo "-- gofmt check failed"; \
		/bin/false; \
	fi
	echo "-- checking with govet"
	$(GO) vet -unsafeptr=false
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

instrument-binary-contained:
	docker run --rm --name instrument-binary --entrypoint "/bin/sh" -v $(shell pwd):/go/src/github.com/mendersoftware/mender-artifact golang:1.18 -c "cd /go/src/github.com/mendersoftware/mender-artifact && go install github.com/mendersoftware/gobinarycoverage@latest && make instrument-binary"

instrument-binary:
	git apply patches/0001-Instrument-with-coverage.patch
	gobinarycoverage github.com/mendersoftware/mender-artifact

coverage:
	rm -f coverage.txt
	go test -tags '$(TAGS)' -covermode=atomic -coverpkg=$(PKGS) -coverprofile=coverage.txt ./...

.PHONY: build clean get-tools test check \
	cover htmlcover coverage tooldep install-autocomplete-scripts \
	instrument-binary
