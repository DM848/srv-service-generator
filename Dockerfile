FROM golang:1.11.2 as builder

WORKDIR /app
COPY . /app

RUN go test ./...
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o webserver cmd/server/main.go

FROM alpine:3.8

WORKDIR /server
COPY --from=builder /app/webserver .

# Install ContainerPilot
ENV CONTAINERPILOT_VER 3.8.0
ENV CONTAINERPILOT /etc/containerpilot.json5

RUN export CONTAINERPILOT_CHECKSUM=84642c13683ddae6ccb63386e6160e8cb2439c26 && \
    wget "https://github.com/joyent/containerpilot/releases/download/${CONTAINERPILOT_VER}/containerpilot-${CONTAINERPILOT_VER}.tar.gz" \
        -O /tmp/containerpilot.tar.gz && \
    echo "${CONTAINERPILOT_CHECKSUM}  /tmp/containerpilot.tar.gz" | sha1sum -c && \
    tar zxf /tmp/containerpilot.tar.gz -C /bin && \
    rm /tmp/containerpilot.tar.gz

# COPY ContainerPilot configuration
COPY containerpilot.json5 /etc/containerpilot.json5
ENV CONTAINERPILOT=/etc/containerpilot.json5

ENV WEB_SERVER_PORT 5678
EXPOSE 5678
CMD ["/bin/containerpilot"]
