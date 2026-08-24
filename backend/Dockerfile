FROM golang:1.25-alpine AS build
RUN apk add --no-cache build-base
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/ai-workbench ./cmd/server

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata && addgroup -S workbench && adduser -S -G workbench workbench
WORKDIR /app
COPY --from=build /out/ai-workbench /usr/local/bin/ai-workbench
RUN mkdir -p /data/attachments && chown -R workbench:workbench /data
USER workbench
EXPOSE 8087
ENTRYPOINT ["/usr/local/bin/ai-workbench"]
