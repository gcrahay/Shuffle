FROM golang as builder

# Add files
RUN mkdir /app

COPY . /app/

WORKDIR /app

RUN go mod download

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o shuffle cmd/shuffle/main.go

# Certificate build
FROM alpine:latest as certs
RUN apk --update add ca-certificates

from scratch
COPY --from=builder /app/shuffle /
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

EXPOSE 5001
CMD ["./shuffle"]
