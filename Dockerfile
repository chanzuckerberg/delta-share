FROM 533267185808.dkr.ecr.us-west-2.amazonaws.com/docker.io/central/library/golang:1.22-alpine AS builder
WORKDIR /app

ARG IMAGE_TAG

RUN apk update 
RUN apk --update add --no-cache git tzdata
ADD . .
RUN GOPROXY=direct go build -ldflags "-X 'main.tag=${IMAGE_TAG}'" -o api

FROM 533267185808.dkr.ecr.us-west-2.amazonaws.com/docker.io/central/library/alpine:3
WORKDIR /app
RUN apk update && apk upgrade && apk --no-cache add curl

COPY --from=builder /app/api /app/
EXPOSE 3000
ENTRYPOINT ./api
