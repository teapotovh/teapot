FROM debian:12-slim AS bird

RUN apt update
RUN apt install -y bird2

FROM golang:1.24 AS build

WORKDIR /go/src/app

COPY --from=root go.mod go.mod
COPY --from=root go.sum go.sum
RUN go mod download

COPY --from=root cmd cmd
COPY --from=root service service
COPY --from=root lib lib

RUN go vet -v ./...

RUN CGO_ENABLED=0 go build -o /go/bin/netd ./cmd/netd

FROM gcr.io/distroless/static-debian12

COPY --from=bird /lib/x86_64-linux-gnu/libssh-gcrypt.so.4 /lib/x86_64-linux-gnu/
COPY --from=bird /lib/x86_64-linux-gnu/libc.so.6 /lib/x86_64-linux-gnu/
COPY --from=bird /lib64/ld-linux-x86-64.so.2 /lib64/
COPY --from=bird /lib/x86_64-linux-gnu/libgcrypt.so.20 /lib/x86_64-linux-gnu/
COPY --from=bird /lib/x86_64-linux-gnu/libgpg-error.so.0 /lib/x86_64-linux-gnu/
COPY --from=bird /lib/x86_64-linux-gnu/libz.so.1 /lib/x86_64-linux-gnu/
COPY --from=bird /lib/x86_64-linux-gnu/libgssapi_krb5.so.2 /lib/x86_64-linux-gnu/
COPY --from=bird /lib/x86_64-linux-gnu/libkrb5.so.3 /lib/x86_64-linux-gnu/
COPY --from=bird /lib/x86_64-linux-gnu/libk5crypto.so.3 /lib/x86_64-linux-gnu/
COPY --from=bird /lib/x86_64-linux-gnu/libcom_err.so.2 /lib/x86_64-linux-gnu/
COPY --from=bird /lib/x86_64-linux-gnu/libkrb5support.so.0 /lib/x86_64-linux-gnu/
COPY --from=bird /lib/x86_64-linux-gnu/libkeyutils.so.1 /lib/x86_64-linux-gnu/
COPY --from=bird /lib/x86_64-linux-gnu/libresolv.so.2 /lib/x86_64-linux-gnu/
COPY --from=bird /usr/sbin/bird /usr/bin

ENV PATH="$PATH:/usr/bin"

COPY --from=build /go/bin/netd /
ENTRYPOINT ["/netd"]
