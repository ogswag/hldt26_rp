FROM golang:1.27.1-bookworm AS build
WORKDIR /src
COPY api/go.mod api/go.sum ./
RUN go mod download
COPY api/ ./
# GOAMD64=v1 keeps the compiler from fusing a multiply and an add, so the server and the browser build agree.
RUN CGO_ENABLED=0 GOOS=linux GOAMD64=v1 go build -o /out/api ./cmd/api
RUN GOOS=js GOARCH=wasm go build -o /out/engine/engine.wasm ./cmd/engine-wasm \
    && cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" /out/engine/wasm_exec.js

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/api /api
COPY --from=build /out/engine/ /engine/
COPY data/catalog.csv /data/catalog.csv
COPY data/seeds/ /data/seeds/
USER nonroot
EXPOSE 8080
ENTRYPOINT ["/api"]
