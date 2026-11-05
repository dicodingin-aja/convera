# Build dari folder backend/:  docker build -t convera-backend ./backend
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN mkdir /uploads
RUN for c in api worker admin adminctl migrate waqr; do \
      CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/$c ./cmd/$c || exit 1; \
    done

# distroless/static sudah berisi sertifikat CA (untuk graph.facebook.com) dan tzdata.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/ /app/
# Folder bukti transfer: dimiliki user nonroot agar volume Docker yang dipasang di sini bisa ditulis.
COPY --from=build --chown=nonroot:nonroot /uploads /data/uploads
WORKDIR /app
ENTRYPOINT ["/app/api"]
