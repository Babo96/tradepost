FROM golang:1.27.3-alpine AS build

WORKDIR /src
RUN apk add --no-cache ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/tradepostd ./cmd/tradepostd

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/tradepostd /tradepostd

EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/tradepostd"]
