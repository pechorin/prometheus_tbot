FROM golang:1.11.3-alpine3.8 as builder
RUN \
    cd / && \
    apk update && \
    apk add --no-cache git ca-certificates make tzdata && \
    git clone https://github.com/pechorin/prometheus_tbot && \
    cd prometheus_tbot && \
    go get -d -v && \
    CGO_ENABLED=0 GOOS=linux go build -v -a -installsuffix cgo -o prometheus_tbot 

FROM alpine:3.8
COPY --from=builder /prometheus_tbot/prometheus_tbot /usr/local/bin/
RUN addgroup -g 5001 tbot && \
    adduser -D -u 5001 -G tbot tbot
RUN chown -R tbot:tbot /usr/local/bin/prometheus_tbot
RUN apk add --no-cache ca-certificates tzdata
USER tbot
EXPOSE 9087
ENTRYPOINT ["/usr/local/bin/prometheus_tbot"]
