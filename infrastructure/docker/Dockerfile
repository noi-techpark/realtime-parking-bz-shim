FROM golang:1.23-bookworm AS base

# build executable
FROM base AS build-env
WORKDIR /app
COPY src/. .
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o main

# build published image with binary from previous stage
FROM alpine:latest AS build
RUN apk add curl
WORKDIR /app
COPY --from=build-env /app/main .
ENTRYPOINT [ "./main" ]

FROM base AS debug
WORKDIR /code
CMD [ "go", "run", "."]
