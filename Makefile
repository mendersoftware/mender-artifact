GO ?= go
GOFMT ?= gofmt
V ?=
PREFIX ?= /usr/local
PKGS = $(shell go list ./... | grep -v vendor)
SUBPKGS = $(shell go list ./... | sed '1d' | tr '\n' ',' | sed 's/,$$//1')
PKGNAME = mender-artifact
PKGFILES = $(shell find . \( -path ./vendor -o -path ./Godeps \) -prune \
		-o -type f -name '*.go' -print)
PKGFILES_notest = $(shell echo $(PKGFILES) | tr ' ' '\n' | grep -v _test.go)
GOCYCLO ?= 20

CGO_ENABLED=1
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

TAGS =
ifeq ($(LOCAL),1)
TAGS += local
endif

ifneq ($(TAGS),)
BUILDTAGS = -tags '$(TAGS)'
endif

build:
	$(GO) build $(GO_LDFLAGS) $(BUILDV) $(BUILDTAGS)

PLATFORMS := darwin linux windows

GO_LDFLAGS_WIN = -ldflags "-X github.com/mendersoftware/mender-artifact/cli.Version=$(VERSION) -linkmode=internal -s -w -extldflags '-static' -extld=x86_64-w64-mingw32-gcc"

build-native-linux:
	 @arch="amd64";
	 @echo "building linux";
	 @env GOOS=linux GOARCH=$$arch \
        $(GO) build -a $(GO_LDFLAGS) $(BUILDV) $(BUILDTAGS) -o $(PKGNAME)-linux ;

build-native-mac:
	@arch="amd64";
	@echo "building mac";
	@env GOOS=darwin GOARCH=$$arch CGO_ENABLED=0 \
        $(GO) build -a $(GO_LDFLAGS) $(BUILDV) $(BUILDTAGS) -o $(PKGNAME)-darwin ;

build-native-windows:
	@arch="amd64";
	@echo "building windows";
	@env GOOS=windows GOARCH=$$arch CC=x86_64-w64-mingw32-gcc CXX=x86_64-w64-mingw32-g++ \
        $(GO) build $(GO_LDFLAGS_WIN) $(BUILDV) -tags $(TAGS) nolzma -o $(PKGNAME)-windows.exe ;

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
	$(GO) list ./... | \
		grep -v vendor | \
		xargs -n1 -I {} go test -v -covermode=atomic -coverprofile=../../../{}/coverage.txt {} 2>&1 | \
		tee /dev/stderr | \
		go-junit-report > \
		test-results.xml || exit $?
	mkdir -p tests/unit-coverage && find . -name 'coverage.txt' -exec rsync -R {} ./tests/unit-coverage \;
	tar -cvf unit-coverage.tar tests/unit-coverage

.PHONY: build clean get-tools test check \
	cover htmlcover coverage tooldep install-autocomplete-scripts \
	instrument-binary
