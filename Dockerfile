FROM golang AS build

WORKDIR /go/src/github.com/Snawoot/hola-proxy
COPY . .
RUN CGO_ENABLED=0 go build -a -tags netgo -ldflags '-s -w -extldflags "-static"'
ADD https://curl.haxx.se/ca/cacert.pem /certs.crt
RUN chmod 0644 /certs.crt

FROM scratch
COPY --from=build /go/src/github.com/Snawoot/hola-proxy/hola-proxy /
COPY --from=build /certs.crt /etc/ssl/certs/ca-certificates.crt
USER 9999:9999
EXPOSE 8080/tcp
ENTRYPOINT ["/hola-proxy", "-bind-address", "0.0.0.0:8080"]
