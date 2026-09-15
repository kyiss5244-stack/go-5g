FROM golang:1.25-alpine AS builder
ENV GOPROXY=https://goproxy.cn,direct
WORKDIR /src
COPY controller ./controller
RUN cd controller && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/controller .

FROM alpine:3.22
COPY --from=builder /out/controller /usr/local/bin/controller
EXPOSE 8090
ENTRYPOINT ["controller"]
