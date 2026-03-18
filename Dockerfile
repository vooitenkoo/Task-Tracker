FROM golang:1.22 AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/gosystem-api ./cmd/api

FROM gcr.io/distroless/base-debian12
WORKDIR /
COPY --from=build /out/gosystem-api /gosystem-api
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/gosystem-api"]
