# One image for the simulator, the UDP member, and the sink.
# The stepper is configuration, not a second build.

FROM golang:1.27.4 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/sim ./cmd/sim \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/node ./cmd/node \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/sink ./cmd/sink

FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=build /out/sim /sim
COPY --from=build /out/node /node
COPY --from=build /out/sink /sink
EXPOSE 8080 9100 9000/udp
USER nonroot
ENTRYPOINT ["/node"]
