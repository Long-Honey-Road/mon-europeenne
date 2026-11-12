# Go version matches go.mod.
FROM golang:1.27.3-alpine3.23 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/buddy ./cmd/buddy

FROM alpine:3.23.8
RUN adduser -D -u 10001 buddy
COPY --from=build /out/buddy /usr/local/bin/buddy
USER buddy
EXPOSE 8080
ENTRYPOINT ["buddy"]
CMD ["api"]
