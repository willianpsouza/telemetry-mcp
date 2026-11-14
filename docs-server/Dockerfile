FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/docs-server .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/docs-server /docs-server
EXPOSE 8081
ENTRYPOINT ["/docs-server"]
