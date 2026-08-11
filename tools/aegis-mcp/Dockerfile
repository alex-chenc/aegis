FROM python:3.13-alpine

WORKDIR /app
COPY aegis_mcp.py /app/aegis_mcp.py

RUN chmod 0555 /app/aegis_mcp.py \
    && addgroup -S aegis \
    && adduser -S -G aegis aegis
USER aegis

EXPOSE 8085
CMD ["python", "/app/aegis_mcp.py", "--transport", "http", "--host", "0.0.0.0", "--port", "8085"]
