package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
)

func TestServer(t *testing.T) {
	mux, db := Bootstrap()
	defer db.Close()

	tests := map[string]int{
		`{
			"retailer": "Walgreens",
			"purchaseDate": "2022-01-02",
			"purchaseTime": "08:13",
			"total": "2.65",
			"items": [
				{"shortDescription": "Pepsi - 12-oz", "price": "1.25"},
				{"shortDescription": "Dasani", "price": "1.40"}
			]
		}`: 15,
		`{
			"retailer": "Target",
			"purchaseDate": "2022-01-02",
			"purchaseTime": "13:13",
			"total": "1.25",
			"items": [
				{"shortDescription": "Pepsi - 12-oz", "price": "1.25"}
			]
		}`: 31,
		`{
			"retailer": "Target",
			"purchaseDate": "2022-01-01",
			"purchaseTime": "13:01",
			"items": [
				{"shortDescription": "Mountain Dew 12PK", "price": "6.49"},
				{"shortDescription": "Emils Cheese Pizza", "price": "12.25"},
				{"shortDescription": "Knorr Creamy Chicken", "price": "1.26"},
				{"shortDescription": "Doritos Nacho Cheese", "price": "3.35"},
				{"shortDescription": "   Klarbrunn 12-PK 12 FL OZ  ", "price": "12.00"}
			],
			"total": "35.35"
		}`: 28,
		`{
			"retailer": "M&M Corner Market",
			"purchaseDate": "2022-03-20",
			"purchaseTime": "14:33",
			"items": [
				{"shortDescription": "Gatorade", "price": "2.25"},
				{"shortDescription": "Gatorade", "price": "2.25"},
				{"shortDescription": "Gatorade", "price": "2.25"},
				{"shortDescription": "Gatorade", "price": "2.25"}
			],
			"total": "9.00"
		}`: 109,
	}

	for test, expected := range tests {
		resp := httptest.NewRecorder()

		mux.ServeHTTP(resp, httptest.NewRequest("POST", "/receipts/process", bytes.NewReader([]byte(test))))
		pRR := processReceiptResponse{}
		_ = json.NewDecoder(resp.Body).Decode(&pRR)

		mux.ServeHTTP(resp, httptest.NewRequest("GET", fmt.Sprintf("/receipts/%s/points", pRR.ID), nil))
		gRR := getReceiptPointsResponse{}
		_ = json.NewDecoder(resp.Body).Decode(&gRR)

		assert.Equal(t, int64(expected), gRR.Points)
	}
}
