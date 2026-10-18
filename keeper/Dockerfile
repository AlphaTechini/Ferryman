# syntax=docker/dockerfile:1

FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ferryman ./cmd/ferryman \
 && mkdir -p /out/data

# Distroless, non-root, no shell. The deployments file is mounted, not baked in:
#   docker run -v "$PWD/../deployments/testnet.json:/etc/ferryman/testnet.json:ro" ...
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/ferryman /usr/local/bin/ferryman
COPY --from=build --chown=nonroot:nonroot /out/data /data
ENV DATA_DIR=/data \
    LISTEN_ADDR=:8787 \
    DEPLOYMENTS=/etc/ferryman/testnet.json
EXPOSE 8787
VOLUME ["/data"]
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/ferryman"]
