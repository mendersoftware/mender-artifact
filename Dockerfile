FROM golang:1.20.2-bullseye as builder
RUN mkdir -p /go/src/github.com/mendersoftware/mender-artifact
WORKDIR /go/src/github.com/mendersoftware/mender-artifact
ADD ./ .
RUN make get-build-deps && \
    make build && \
    make install
ENTRYPOINT [ "/go/bin/mender-artifact" ]
