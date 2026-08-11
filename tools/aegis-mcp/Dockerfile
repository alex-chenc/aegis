FROM python:3.13-alpine

WORKDIR /app
COPY aegis_mcp.py /app/aegis_mcp.py
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

RUN apk add --no-cache su-exec \
    && chmod 0555 /app/aegis_mcp.py /usr/local/bin/docker-entrypoint.sh \
    && addgroup -S aegis \
    && adduser -S -G aegis aegis

EXPOSE 8085
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["python", "/app/aegis_mcp.py", "--transport", "http", "--host", "0.0.0.0", "--port", "8085"]
