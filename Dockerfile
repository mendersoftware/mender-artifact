FROM buildpack-deps:stretch-curl as tini

ENV TINI_VERSION v0.18.0

RUN mkdir -p /tmp/build && \
    cd /tmp/build && \
    wget -O tini https://github.com/krallin/tini/releases/download/${TINI_VERSION}/tini && \
    wget -O tini.asc https://github.com/krallin/tini/releases/download/${TINI_VERSION}/tini.asc && \
    gpg --keyserver hkp://p80.pool.sks-keyservers.net:80 --recv-keys 595E85A6B1B4779EA4DAAEC70B588DFF0527A9B7 && \
    gpg --verify tini.asc && \
    cp tini /sbin/ && \
    chmod +x /sbin/tini && \
    cd /tmp && \
    rm -rf /tmp/build && \
    rm -rf /root/.gnupg

FROM debian:latest

COPY --from=tini /sbin/tini /sbin/

ENV MENDER_ARTIFACT_VERSION 2.4.0

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        wget \
        ca-certificates \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/* \
    && cd /usr/bin \
    && wget https://d1b0l86ne08fsf.cloudfront.net/mender-artifact/${MENDER_ARTIFACT_VERSION}/mender-artifact \
    && chmod +x mender-artifact

ENTRYPOINT ["/sbin/tini", "-g", "--", "/usr/bin/mender-artifact"]
CMD ["-h"]
