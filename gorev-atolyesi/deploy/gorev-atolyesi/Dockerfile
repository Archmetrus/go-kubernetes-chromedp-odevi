FROM golang:1.27.1-alpine AS build
RUN apk add --no-cache ca-certificates
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY internal/tasks ./internal/tasks
COPY cmd/gorev-atolyesi ./cmd/gorev-atolyesi
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /server ./cmd/gorev-atolyesi
FROM scratch
COPY --from=build /server /server
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
USER 65532:65532
ENV ADDR=:8090 DB_PATH=/data/gorevler.db
EXPOSE 8090
ENTRYPOINT ["/server"]
