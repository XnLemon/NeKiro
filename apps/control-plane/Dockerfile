FROM golang:1.26.4-bookworm@sha256:b305420a68d0f229d91eb3b3ed9e519fcf2cf5461da4bef997bf927e8c0bfd2b AS build

WORKDIR /src
ARG GOPROXY=https://proxy.golang.org
ARG GOSUMDB=sum.golang.org
COPY go.mod go.sum ./
RUN GOPROXY="$GOPROXY" GOSUMDB="$GOSUMDB" go mod download
COPY contracts ./contracts
COPY apps/control-plane ./apps/control-plane
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/control-plane ./apps/control-plane/cmd/control-plane

FROM scratch

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/control-plane /control-plane
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/control-plane"]
CMD ["serve"]
