FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN go build -o /bin/deliveries ./cmd/server

FROM alpine:3.20
WORKDIR /app
COPY --from=builder /bin/deliveries /deliveries
COPY config/config.yaml /app/config/config.yaml
COPY docs/swagger.yaml /app/docs/swagger.yaml
EXPOSE 8080
CMD ["/deliveries"]
