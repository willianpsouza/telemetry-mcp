FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/registry .

# base (root) so the SQLite data volume is writable; no shell in the image.
FROM gcr.io/distroless/base-debian12:latest
COPY --from=build /out/registry /registry
EXPOSE 8082 4319
VOLUME ["/data"]
ENTRYPOINT ["/registry"]
