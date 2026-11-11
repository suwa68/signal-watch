# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM golang:1.25-bookworm AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY integration/monitor/ ./integration/monitor/
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -o /fixture ./integration/monitor
FROM scratch
COPY --from=build /fixture /fixture
ENTRYPOINT ["/fixture"]
CMD ["serve"]
