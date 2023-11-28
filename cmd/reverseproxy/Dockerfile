FROM docker.io/golang:1.20-alpine

WORKDIR /build
COPY go.mod .
ADD ./cmd ./cmd
RUN go build ./cmd/reverseproxy

ENTRYPOINT [ "./reverseproxy" ]