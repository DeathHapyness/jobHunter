# syntax=docker/dockerfile:1
FROM golang:1.22-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOTOOLCHAIN=local
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/vagas ./cmd/vagas \
 && mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/vagas /app/vagas
COPY --from=build --chown=nonroot:nonroot /out/data /data
COPY config.yaml companies.yaml /app/
ENV VAGAS_DB=/data/vagas.db \
    GOMEMLIMIT=300MiB \
    VAGAS_LOG_FORMAT=json
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD ["/app/vagas", "healthcheck"]
ENTRYPOINT ["/app/vagas"]
CMD ["serve"]
