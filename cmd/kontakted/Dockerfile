FROM golang:1.24 AS build

WORKDIR /go/src/app

COPY --from=root go.mod go.mod
COPY --from=root go.sum go.sum
RUN go mod download

COPY --from=root cmd cmd
COPY --from=root service service
COPY --from=root lib lib

RUN go vet -v ./cmd/kontakted

RUN CGO_ENABLED=0 go build -o /go/bin/kontakted ./cmd/kontakted

FROM gcr.io/distroless/static-debian12

COPY --from=build /go/bin/kontakted /

ENTRYPOINT ["/kontakted"]
