FROM golang:1.23-bookworm AS base
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
FROM base AS test
CMD ["go", "test", "-race", "-count=1", "-v", "./..."]
FROM base AS build
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /server ./cmd/server
FROM gcr.io/distroless/static-debian12:nonroot AS runtime
COPY --from=build /server /server
EXPOSE 8080
ENTRYPOINT ["/server"]
