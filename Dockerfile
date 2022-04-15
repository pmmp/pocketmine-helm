FROM golang:1.18-alpine AS builder
WORKDIR /usr/src/app
RUN addgroup -S plugindl && \
	adduser -S plugindl \
	-G plugindl \
	-u 2000 # intentionally different from pmmp/pocketmine-mp

RUN apk add --no-cache make

ADD go.mod go.mod
ADD go.sum go.sum
ADD cmd cmd
ADD pkg pkg
ADD Makefile Makefile

RUN ln -s /usr/bin bin
RUN make bin/plugin-downloader bin/server-manager

FROM gcr.io/distroless/static AS plugin-downloader
COPY --from=builder /usr/bin/plugin-downloader /usr/bin/plugin-downloader

FROM gcr.io/distroless/static AS server-manager
COPY --from=builder /usr/bin/server-manager /usr/bin/server-manager
