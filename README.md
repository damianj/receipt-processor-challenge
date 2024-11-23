# Running the project

If the necessary dependencies are found in the host system then it should be sufficient to run the following command from the project root:

```sh
cd src && go run ./cmd/main.go
```

If you are facing issues then the Dockerfile that is provided can compile and run the application as it has any
necessary dependencies already installed. To build the image and run the container simply run the following command from the project root:

```sh
docker build . --tag api:latest && docker run -d -p 8080:8080 api:latest
```

Once started the application will spin up an in memory instance of DuckDB and will be exposed on `http://localhost:8080/`.
