FROM golang:1.23-alpine as builder
RUN mkdir /build
WORKDIR /build
COPY . .

ENV GOOS=linux GOARCH=amd64 CGO_ENABLED=0

RUN set -ex && \
  apk add --no-progress --no-cache \
  gcc \
  musl-dev
RUN go install --tags musl ./...

FROM alpine:3.15.0
RUN apk --no-cache add ca-certificates
RUN ln -fs /usr/share/zoneinfo/Asia/Ho_Chi_Minh /etc/localtime
WORKDIR /

COPY --from=builder /go/bin/* /usr/bin/
COPY migrations /migrations
COPY docs /docs

CMD [ "server" ]
