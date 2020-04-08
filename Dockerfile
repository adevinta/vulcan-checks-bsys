FROM golang:1.13.3-alpine3.10 as builder

WORKDIR /app

COPY go.mod .
COPY go.sum .

RUN go mod download

COPY . .

RUN cd cmd/vulcan-build-images/ && GOOS=linux GOARCH=amd64 go build . && cd -

FROM alpine:3.10

RUN apk add --no-cache --update gettext ca-certificates

WORKDIR /app

ARG BUILD_RFC3339="1970-01-01T00:00:00Z"
ARG COMMIT="local"

ENV BUILD_RFC3339 "$BUILD_RFC3339"
ENV COMMIT "$COMMIT"

COPY --from=builder /app/cmd/vulcan-build-images/vulcan-build-images .

ADD run.sh .
ADD config.toml .

CMD [ "./run.sh" ]
