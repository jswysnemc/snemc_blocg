# syntax=docker/dockerfile:1.7

FROM node:22-alpine AS frontend-builder
WORKDIR /src/frontend

COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci

COPY frontend/ ./
RUN npm run build


FROM golang:1.26-alpine AS go-builder
WORKDIR /src

ARG TARGETOS=linux
ARG TARGETARCH

COPY go.mod go.sum ./
RUN go mod download

COPY . ./
COPY --from=frontend-builder /src/frontend/dist ./frontend/dist

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o /out/snemc-blog ./cmd/server


FROM alpine:3.22
WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY --from=go-builder /out/snemc-blog /app/snemc-blog

ENV BLOG_ADDR=:8080 \
    BLOG_DB_PATH=/app/data/blog.sqlite3 \
    BLOG_MEDIA_DIR=/app/data/media \
    BLOG_UPLOADS_DIR=/app/data/uploads

VOLUME ["/app/data"]

EXPOSE 8080

ENTRYPOINT ["/app/snemc-blog"]
