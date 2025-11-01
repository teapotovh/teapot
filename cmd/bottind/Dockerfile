FROM golang:1.22 AS build

WORKDIR /go/src/app
COPY . .

RUN go mod download
RUN go vet -v

RUN CGO_ENABLED=0 go build -o /go/bin/bottin ./cmd/bottin

FROM gcr.io/distroless/static-debian12

COPY --from=build /go/bin/bottin /
ENTRYPOINT ["/bottin"]
