FROM golang:1.11-alpine3.9 as builder
RUN apk update &&  apk add gcc musl-dev libc6-compat git make xz-dev
RUN mkdir -p /go/src/github.com/mendersoftware/mender-artifact
WORKDIR /go/src/github.com/mendersoftware/mender-artifact
ADD ./ .
RUN make build

FROM alpine:3.9
RUN apk add xz-dev
COPY --from=builder /go/src/github.com/mendersoftware/mender-artifact/mender-artifact /go/bin/mender-artifact
ENTRYPOINT [ "/go/bin/mender-artifact" ]
