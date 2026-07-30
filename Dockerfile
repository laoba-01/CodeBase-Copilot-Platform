FROM alpine:3.19
RUN apk add --no-cache ca-certificates git ctags
# NOTE: If your environment uses a custom CA, mount it and configure:
#   COPY your-ca.pem /usr/local/share/ca-certificates/your-ca.crt
#   RUN update-ca-certificates
#   RUN git config --global http.sslCAInfo /etc/ssl/certs/ca-certificates.crt
RUN git config --global http.version HTTP/1.1

# Create non-root user
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

WORKDIR /app
COPY server /usr/local/bin/server
COPY web/dist ./web/dist
COPY migrations /migrations

# Ensure data directory exists and is writable
RUN mkdir -p /data/repos && chown -R appuser:appgroup /data/repos /app

EXPOSE 8080
USER appuser
CMD ["server"]
