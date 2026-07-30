FROM alpine:3.19
RUN apk add --no-cache ca-certificates git ctags
# Configure git: disable SSL verification for environments with custom CA.
# Proxy is configured via HTTP_PROXY/HTTPS_PROXY env vars at runtime,
# not hardcoded in git config (so it works without a proxy too).
RUN git config --global http.sslVerify false && \
    git config --global http.version HTTP/1.1
WORKDIR /app
COPY server /usr/local/bin/server
COPY web/dist ./web/dist
COPY migrations /migrations
EXPOSE 8080
CMD ["server"]
