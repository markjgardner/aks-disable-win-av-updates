FROM golang:alpine3.19 as build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o operator .

FROM golang:alpine3.19 as runtime
COPY --from=build /app/operator /app/operator
RUN chmod +x /app/operator
CMD ["/app/operator"]