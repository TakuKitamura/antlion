ARG IMAGE=arm32v7/golang:1.15-alpine
FROM $IMAGE

WORKDIR /go/src/app
COPY . .

RUN go get -d -v ./...
RUN go install -v ./...

CMD ["app"]
