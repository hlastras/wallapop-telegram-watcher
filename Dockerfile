FROM golang:alpine AS builder

WORKDIR /app

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o app .

FROM alpine:latest

RUN apk add --no-cache chromium chromium-chromedriver
RUN mkdir -p /dev/shm && chmod 1777 /dev/shm
ENV CHROME_BIN=/usr/bin/chromium-browser
ENV CHROME_PATH=/usr/lib/chromium/
ENV CHROMEDP_HEADLESS=true

WORKDIR /root/

COPY --from=builder /app/app .

RUN chmod +x app

CMD ["./app"]