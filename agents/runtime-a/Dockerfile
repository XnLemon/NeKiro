FROM golang:1.26.4-bookworm@sha256:b305420a68d0f229d91eb3b3ed9e519fcf2cf5461da4bef997bf927e8c0bfd2b AS build

WORKDIR /src
ARG GOPROXY=https://proxy.golang.org
ARG GOSUMDB=sum.golang.org
COPY go.mod go.sum ./
COPY contracts ./contracts
COPY sdks/agent-sdk ./sdks/agent-sdk
COPY agents/internal/challengeproof ./agents/internal/challengeproof
COPY agents/runtime-a/go.mod agents/runtime-a/go.sum ./agents/runtime-a/
WORKDIR /src/agents/runtime-a
RUN GOPROXY="$GOPROXY" GOSUMDB="$GOSUMDB" go mod download
COPY agents/runtime-a ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/runtime-a ./cmd/runtime-a

FROM alpine:3.22

RUN apk add --no-cache ca-certificates wget \
    && adduser -D -H -u 10003 nekiro
COPY --from=build /out/runtime-a /usr/local/bin/runtime-a
USER nekiro
EXPOSE 8091
ENTRYPOINT ["/usr/local/bin/runtime-a"]
