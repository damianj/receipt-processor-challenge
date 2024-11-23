package main

import (
	"log"
	"net/http"
	"receipt-processor-challenge/api"
)

func main() {
	mux, db := api.Bootstrap()
	defer db.Close()

	log.Fatal(http.ListenAndServe(":8080", mux))
}
