FROM 533267185808.dkr.ecr.us-west-2.amazonaws.com/docker.io/central/library/golang:1.23-alpine AS builder
WORKDIR /app
ARG IMAGE_TAG
RUN apk add --no-cache git tzdata
COPY . .
RUN GOPROXY=direct go build -o api .

FROM 533267185808.dkr.ecr.us-west-2.amazonaws.com/docker.io/central/library/alpine:3
WORKDIR /app
RUN apk add --no-cache curl

COPY --from=builder /app/api /app/
RUN chmod +x /app/api
EXPOSE 3000
ENTRYPOINT ["/app/api"]
