# syntax=docker/dockerfile:1
#
# One Dockerfile builds any of the services. Pick which with the SERVICE build
# arg, e.g.:
#   docker build -f deploy/Dockerfile --build-arg SERVICE=gateway -t iot_ai/gateway .
#
# Build the runtime stage for another CPU with buildx, e.g. for the Jetson:
#   docker buildx build --platform linux/arm64 --build-arg SERVICE=ai-service ...

# ---- build stage ----
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS build
ARG SERVICE
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src

# Download dependencies first so they are cached across source-only changes.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/${SERVICE}

# ---- runtime stage ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
USER nonroot:nonroot
ENTRYPOINT ["/app"]
