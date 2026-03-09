FROM alpine:latest

WORKDIR /root/

# Install ca-certificates for HTTPS connections
RUN apk add --no-cache ca-certificates

# Copy the pre-built binary
COPY backend .

# Create config directory and copy config file
RUN mkdir -p config
COPY config/config.yaml config/

# Expose ports
EXPOSE 8080
EXPOSE 9090

# Run command
CMD ["./backend"]
