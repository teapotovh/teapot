ARG root=.
FROM golang:1.22 AS build

WORKDIR /go/src/app
COPY $root .

RUN go mod download
RUN go vet -v

RUN CGO_ENABLED=0 go build -o /go/bin/kontakted ./cmd/kontakted

FROM gcr.io/distroless/static-debian12

COPY --from=build /go/bin/kontakted /
ENTRYPOINT ["/kontakted"]
