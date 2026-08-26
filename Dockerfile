FROM golang:1.24-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates git && go install github.com/a-h/templ/cmd/templ@v0.3.960
COPY go.mod ./
RUN go mod download
COPY . .
RUN templ generate && go test ./... && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/fitness-tracker ./cmd/fitness-tracker

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata && addgroup -S app && adduser -S app -G app
WORKDIR /app
COPY --from=build /out/fitness-tracker /app/fitness-tracker
COPY static /app/static
USER app
EXPOSE 8080
ENTRYPOINT ["/app/fitness-tracker"]
