# syntax=docker/dockerfile:1

# The build stage runs on the builder's own platform and cross-compiles:
# there is no CGO, so one Dockerfile gives both linux/arm64 and linux/amd64.
FROM --platform=$BUILDPLATFORM golang:1.27.2 AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/linksblog ./cmd/linksblog
# The image has no shell, so the data directory is prepared here.
RUN mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/linksblog /linksblog
COPY --from=build --chown=nonroot:nonroot /out/data /data
ENV LINKSBLOG_DB=/data/linksblog.db \
    LINKSBLOG_ADDR=:8080
EXPOSE 8080
ENTRYPOINT ["/linksblog"]
CMD ["serve"]
