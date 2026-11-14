# Build
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/telemetry-mcp .

# Run
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/telemetry-mcp /telemetry-mcp
EXPOSE 8080
ENTRYPOINT ["/telemetry-mcp"]
