# Dependency stage (this project has none, but it keeps the layer cache useful)
FROM golang:1.26.5 AS deps

WORKDIR /app
COPY go.mod ./
RUN go mod download 

# Testing phase / target
FROM deps AS test

# WORKDIR is inherited from deps
COPY . .
RUN go vet ./... && go test ./... && touch /tmp/tests-passed


# Building go code
FROM deps AS build

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
# Forces the test stage to run first: a failing test stops the whole build
COPY --from=test /tmp/tests-passed /tmp/tests-passed
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildDate=${BUILD_DATE}" \
    -o /out/app .


# Final stage of building for the dockerized image
FROM gcr.io/distroless/static-debian13:nonroot AS final

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

LABEL org.opencontainers.image.title="go-lab" \
      org.opencontainers.image.description="Small Go HTTP service for the Docker + ECR workshop" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.source="https://github.com/Alejoles/commit-lab-docker"

COPY --from=build /out/app /app

USER nonroot
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD ["/app", "healthcheck"]


ENTRYPOINT ["/app"]