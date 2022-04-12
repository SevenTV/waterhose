FROM golang:1.18 as builder

WORKDIR /tmp/twitch-chat-controller

COPY . .

ARG BUILDER
ARG VERSION

ENV TWITCH_CHAT_CONTROLLER_BUILDER=${BUILDER}
ENV TWITCH_CHAT_CONTROLLER_VERSION=${VERSION}

RUN apt-get update && apt-get install make git gcc protobuf-compiler -y && \
    make

FROM ubuntu:21.04

WORKDIR /app

COPY --from=builder /tmp/twitch-chat-controller/bin/twitch-chat-controller .

CMD ["/app/twitch-chat-controller"]
