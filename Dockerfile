FROM golang:1.27.1-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
COPY internal/webapp ./internal/webapp
COPY 01-go/orta/http-api ./01-go/orta/http-api
RUN CGO_ENABLED=0 go build -trimpath -o /server ./01-go/orta/http-api
FROM scratch
COPY --from=build /server /server
USER 65532:65532
ENV ADDR=:8080
EXPOSE 8080
ENTRYPOINT ["/server"]
