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
COPY --from=builder /prometheus_tbot/prometheus_tbot /
RUN apk add --no-cache ca-certificates tzdata
EXPOSE 9087
ENTRYPOINT ["/prometheus_tbot"]
