FROM golang:1.11 as builder
RUN apt-get update && \
    apt-get install -y \
        gcc gcc-mingw-w64 gcc-multilib \
        git make \
        musl-dev liblzma-dev
RUN mkdir -p /go/src/github.com/mendersoftware/mender-artifact
WORKDIR /go/src/github.com/mendersoftware/mender-artifact
RUN env GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc CXX=x86_64-w64-mingw32-g++ go get 'golang.org/x/sys/windows'
ADD ./ .
RUN make build-natives

FROM alpine:3.9
RUN apk add xz-dev
COPY --from=builder /go/src/github.com/mendersoftware/mender-artifact/mender-artifact* /go/bin/
ENTRYPOINT [ "/go/bin/mender-artifact-linux" ]
