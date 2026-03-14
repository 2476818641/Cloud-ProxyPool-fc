FROM golang:1.25 AS builder

WORKDIR /src/client

COPY client/go.mod client/go.sum ./
RUN go mod download

COPY client/ ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/cloud-proxy .

FROM python:3.12-slim

WORKDIR /app

ENV PYTHONDONTWRITEBYTECODE=1
ENV PYTHONUNBUFFERED=1

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY deploy/requirements.txt /app/deploy/requirements.txt
RUN pip install --no-cache-dir -r /app/deploy/requirements.txt

COPY --from=builder /out/cloud-proxy /usr/local/bin/cloud-proxy
COPY deploy /app/deploy
COPY server /app/server
COPY client/certs /app/client/certs
COPY docker/entrypoint.sh /usr/local/bin/entrypoint.sh

RUN chmod +x /usr/local/bin/entrypoint.sh \
    && mkdir -p /app/config /app/runtime /app/client

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
