FROM golang:1.21.1 AS build

RUN apt update && apt install -yyy --no-install-recommends libpcap0.8 libpcap-dev

RUN mkdir /app
WORKDIR /app
COPY . .

WORKDIR /app/nodelet
RUN go mod download
RUN go build -o /nodelet ./cmd/main.go

WORKDIR /app/dispatch
RUN go mod download
RUN go build -o /dispatch ./cmd/main.go


FROM ubuntu:latest
LABEL authors="Vito Sartori <hey@vito.io>"
LABEL org.opencontainers.image.source=https://github.com/udpfw/udpfw
LABEL org.opencontainers.image.description="UDPfw Binaries"
LABEL org.opencontainers.image.licenses=MIT

RUN apt update && apt install -yyy --no-install-recommends libpcap0.8 libpcap-dev

RUN mkdir /opt/udpfw
COPY --from=build /nodelet /opt/udpfw/nodelet
COPY --from=build /dispatch /opt/udpfw/dispatch
