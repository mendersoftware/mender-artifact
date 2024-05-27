FROM golang:1.22.4 as builder-deb
WORKDIR /build
COPY . .
RUN apt update && apt install -qy libssl-dev && \
    make build

FROM debian:12.5-slim
COPY --from=builder-deb /build/mender-artifact /usr/bin/mender-artifact
RUN apt update && apt install -qy openssl
ENTRYPOINT ["/usr/bin/mender-artifact"]
