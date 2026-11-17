# Builds a dynamically linked Bifrost and the Heimdall plugin in one Go
# workspace, so both share identical package builds (Go plugins require it).
ARG BIFROST_REF=transports/v2.5.1

FROM node:25-alpine3.23 AS ui
ARG BIFROST_REF
RUN apk add --no-cache git
RUN git clone --depth 1 --branch ${BIFROST_REF} https://github.com/maximhq/bifrost.git /bifrost
WORKDIR /bifrost/ui
RUN npm ci && npm run build-enterprise

FROM golang:1.27.3-alpine3.24 AS build
ARG BIFROST_REF
RUN apk add --no-cache git gcc musl-dev sqlite-dev
RUN git clone --depth 1 --branch ${BIFROST_REF} https://github.com/maximhq/bifrost.git /bifrost
COPY --from=ui /bifrost/ui/out /bifrost/transports/bifrost-http/ui
# go.mod first: the workspace (and so the Bifrost build) only changes when
# the plugin's dependencies do, not on every source edit.
COPY plugin/go.mod plugin/go.sum /bifrost/heimdall/
WORKDIR /bifrost
RUN go work init ./transports ./heimdall
ENV CGO_ENABLED=1
RUN cd transports && go build -trimpath -ldflags="-w -s" -o /out/bifrost-http ./bifrost-http
COPY plugin /bifrost/heimdall
RUN cd heimdall && go build -trimpath -ldflags="-w -s" -buildmode=plugin -o /out/heimdall.so ./cmd/heimdall

FROM alpine:3.23
RUN apk add --no-cache musl libgcc ca-certificates zlib sqlite-libs
WORKDIR /app
COPY --from=build /out/bifrost-http /app/bifrost-http
COPY --from=build /out/heimdall.so /app/plugins/heimdall.so
ENV APP_DIR=/app/data
EXPOSE 8080
ENTRYPOINT ["/app/bifrost-http", "-app-dir", "/app/data", "-host", "0.0.0.0", "-port", "8080"]
