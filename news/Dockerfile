# The news service is a single static binary with no shell.
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY news/go.mod ./
COPY news/*.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/news .

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/news /news
USER 65532:65532
EXPOSE 8091
ENTRYPOINT ["/news"]
