# build stage
FROM golang:latest AS build-env
WORKDIR /go/src/github.com/probably-not/google_adexchangebuyer_exporter
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-extldflags "-static"' -tags netgo -o google-adexchangebuyer-exporter && go test ./...

# final stage
FROM gcr.io/distroless/static:latest
WORKDIR /app
COPY --from=build-env /go/src/github.com/probably-not/google_adexchangebuyer_exporter/google-adexchangebuyer-exporter /app/
ENTRYPOINT ["./google-adexchangebuyer-exporter"]
