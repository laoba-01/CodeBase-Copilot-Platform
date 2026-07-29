FROM alpine:3.19
RUN apk add --no-cache ca-certificates git ctags
RUN git config --global http.proxy http://host.docker.internal:7897 && \
    git config --global https.proxy http://host.docker.internal:7897 && \
    git config --global http.version HTTP/1.1 && \
    git config --global http.sslVerify false
WORKDIR /app
COPY server /usr/local/bin/server
COPY web/dist ./web/dist
COPY migrations /migrations
EXPOSE 8080
CMD ["server"]
