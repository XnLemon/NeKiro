FROM golang:1.26.4-bookworm@sha256:b305420a68d0f229d91eb3b3ed9e519fcf2cf5461da4bef997bf927e8c0bfd2b AS build

WORKDIR /src
ARG GOPROXY=https://proxy.golang.org
ARG GOSUMDB=sum.golang.org
COPY go.mod go.sum ./
RUN GOPROXY="$GOPROXY" GOSUMDB="$GOSUMDB" go mod download
COPY contracts ./contracts
COPY agents/internal/challengeproof ./agents/internal/challengeproof
COPY agents/runtime-b ./agents/runtime-b
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/runtime-b ./agents/runtime-b/cmd/runtime-b

FROM alpine:3.22

RUN apk add --no-cache wget \
    && adduser -D -H -u 10002 nekiro
COPY --from=build /out/runtime-b /usr/local/bin/runtime-b
USER nekiro
EXPOSE 8092
ENTRYPOINT ["/usr/local/bin/runtime-b"]
