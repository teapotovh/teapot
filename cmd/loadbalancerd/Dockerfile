FROM golang:1.24 AS build

RUN apt-get update && apt-get install -y iptables

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

COPY --from=build /usr/sbin/ip6tables /usr/sbin/ip6tables
COPY --from=build /usr/sbin/iptables /usr/sbin/iptables

COPY --from=build /lib/x86_64-linux-gnu/libxtables.so.12 /lib/x86_64-linux-gnu/
COPY --from=build /lib/x86_64-linux-gnu/libmnl.so.0 /lib/x86_64-linux-gnu/
COPY --from=build /lib/x86_64-linux-gnu/libnftnl.so.11 /lib/x86_64-linux-gnu/
COPY --from=build /lib/x86_64-linux-gnu/libc.so.6 /lib/x86_64-linux-gnu/
COPY --from=build /lib64/ld-linux-x86-64.so.2 /lib64/
COPY --from=build /usr/lib/x86_64-linux-gnu/xtables/ /usr/lib/x86_64-linux-gnu/xtables/

COPY --from=build /go/bin/loadbalancerd /

ENV PATH="/usr/sbin:/usr/bin:/sbin:/bin"

ENTRYPOINT ["/loadbalancerd"]
