FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/zums-proxy-relay ./cmd/server

FROM scratch
COPY --from=build /out/zums-proxy-relay /zums-proxy-relay
EXPOSE 8080
ENTRYPOINT ["/zums-proxy-relay"]
