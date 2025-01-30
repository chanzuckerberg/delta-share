# Builder Stage
FROM 533267185808.dkr.ecr.us-west-2.amazonaws.com/docker.io/central/library/golang:1.23-alpine AS builder
WORKDIR /app

# Build argument for the image tag (if required)
ARG IMAGE_TAG

# Install dependencies for building the app
RUN apk add --no-cache git tzdata

# Copy source code into the container
COPY . .

# Build the Go binary
RUN GOPROXY=direct go build -o api .

# Final Stage
FROM docker.io/library/alpine:3
WORKDIR /app

# Install runtime dependencies
RUN apk add --no-cache curl

# Copy the built binary from the builder stage
COPY --from=builder /app/api /app/

# Ensure the binary is executable
RUN chmod +x /app/api

# Expose port 3000 for the application
EXPOSE 3000

# Entry point for the container
ENTRYPOINT ["/app/api"]
