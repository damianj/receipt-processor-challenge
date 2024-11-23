FROM golang:bookworm

WORKDIR /app

COPY ./src ./src

RUN cd ./src &&\
 go mod download &&\
 CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o ../api ./cmd &&\
 cd .. &&\
 rm -rf ./src

EXPOSE 8080:8080

CMD ["./api"]
