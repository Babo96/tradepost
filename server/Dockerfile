FROM golang:1.27.3-alpine AS build

WORKDIR /src
RUN apk add --no-cache ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/bfd ./cmd/bfd

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/bfd /bfd

EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/bfd"]
