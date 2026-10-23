FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/api ./cmd/api && CGO_ENABLED=0 go build -o /out/seed ./cmd/seed

FROM alpine:3
RUN adduser -D app && mkdir /uploads && chown app /uploads
COPY --from=build /out/ /usr/local/bin/
USER app
ENV UPLOAD_DIR=/uploads
EXPOSE 8080
CMD ["api"]
