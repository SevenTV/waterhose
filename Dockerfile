FROM golang:1.18 as builder

WORKDIR /tmp/waterhose

COPY . .

ARG BUILDER
ARG VERSION

ENV WATERHOSE_BUILDER=${BUILDER}
ENV WATERHOSE_VERSION=${VERSION}

RUN apt-get update && apt-get install make git gcc protobuf-compiler ca-certificates -y && \
    make

FROM ubuntu:21.04

WORKDIR /app

RUN apt-get update && apt-get install -y ca-certificates

COPY --from=builder /usr/sbin/update-ca-certificates /usr/sbin/update-ca-certificates
COPY --from=builder /tmp/waterhose/bin/waterhose .


CMD ["/app/waterhose"]
