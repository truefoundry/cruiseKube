# Build stage
FROM public.ecr.aws/docker/library/golang:1.24-alpine3.22 AS builder
RUN apk add --no-cache git gcc musl-dev
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY ./cmd ./cmd
COPY ./pkg ./pkg
# The sqlite driver wouldn't work without CGO_ENABLED=1
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o cruisekube cmd/cruisekube/main.go

# Runtime stage
FROM public.ecr.aws/docker/library/alpine:3.22
RUN apk --no-cache add ca-certificates tzdata sqlite
WORKDIR /app
COPY --from=builder /app/cruisekube .
COPY config.yaml /app/config.yaml
COPY web /app/web
RUN mkdir -p /app/data
EXPOSE 8080
CMD ["./cruisekube"]