FROM golang:nanoserver-ltsc2022 as build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o main.exe .

FROM golang:nanoserver-ltsc2022 as runtime
COPY --from=build /app/main.exe /app/main.exe
CMD ["/app/main.exe"]