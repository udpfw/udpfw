FROM golang:1.21.1 AS build

RUN mkdir /app
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o udpfw-dispatch ./cmd/main.go

FROM ubuntu:latest
LABEL authors="Vito Sartori <hey@vito.io>"
LABEL org.opencontainers.image.source=https://github.com/udpfw/udpfw
LABEL org.opencontainers.image.description="UDPfw Dispatch Image"
LABEL org.opencontainers.image.licenses=MIT

RUN mkdir /opt/udpfw
COPY --from=build /app/udpfw-dispatch /opt/udpfw/dispatch
CMD ["/opt/udpfw/dispatch"]
