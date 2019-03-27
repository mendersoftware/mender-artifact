FROM golang:1.11 as builder
RUN apt-get update && apt-get install --no-install-recommends -y musl-dev libc6 git liblzma-dev
RUN mkdir -p /go/src/github.com/mendersoftware/mender-artifact
WORKDIR /go/src/github.com/mendersoftware/mender-artifact
ADD ./ .
RUN make build

FROM busybox
COPY --from=builder /go/src/github.com/mendersoftware/mender-artifact/mender-artifact /go/bin/mender-artifact
ENTRYPOINT [ "/go/bin/mender-artifact" ]
