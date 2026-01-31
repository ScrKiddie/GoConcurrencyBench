# stage pertama untuk build binary
# menggunakan golang alpine dengan dependency libvips
FROM golang:1.24-alpine AS builder
RUN apk add --no-cache build-base pkgconfig vips-dev gcc musl-dev
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go mod tidy
RUN CGO_ENABLED=1 GOOS=linux go build -o worker_bin cmd/worker/main.go
RUN CGO_ENABLED=1 GOOS=linux go build -o seeder_bin cmd/seeder/main.go

# stage kedua untuk runtime
# hanya berisi binary dan dependency minimal
FROM alpine:3.19
RUN apk add --no-cache vips bash tzdata
WORKDIR /root/
COPY --from=builder /app/worker_bin .
COPY --from=builder /app/seeder_bin .
RUN mkdir -p storage/uploads storage/compressed results
CMD ["./worker_bin"]
