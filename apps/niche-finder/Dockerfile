# niche-finder — niche discovery + saturation meter (Go).
#
# Multi-stage: build a static, CGO-free binary, then ship it on a tiny
# distroless base. Templates are embedded in the binary (go:embed), so the
# runtime image is just the binary + a /data volume for the niche DB.
FROM golang:1.23 AS build
WORKDIR /src

# Deps first so source edits don't re-download modules.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
    -o /out/niche-finder ./cmd/niche-finder

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/niche-finder /niche-finder
EXPOSE 8820
VOLUME ["/data"]
ENTRYPOINT ["/niche-finder"]
