FROM golang:1.27-alpine AS build
WORKDIR /src
COPY shared/actions/go.mod shared/actions/*.go ./shared/actions/
WORKDIR /src/collector
COPY collector/go.mod collector/go.sum ./
COPY collector/*.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/collector .

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/collector /collector
USER 65532:65532
EXPOSE 8090
ENTRYPOINT ["/collector"]
