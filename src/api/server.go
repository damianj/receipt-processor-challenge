package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	_ "github.com/marcboeker/go-duckdb"
	"log"
	"log/slog"
	"math"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

type errorResponse struct {
	StatusCode int    `json:"status_code"`
	Message    string `json:"message"`
}

type processReceiptResponse struct {
	ID string `json:"id"`
}

type getReceiptPointsResponse struct {
	Points int64 `json:"points"`
}

type item struct {
	Id               string `json:"-"`
	ReceiptId        string `json:"-"`
	ShortDescription string `json:"shortDescription" validate:"regex=^[\\w\\s\\-]+$"`
	Price            string `json:"price" validate:"regex=^\\d+\\.\\d{2}$"`
}

type receipt struct {
	ID           string `json:"-"`
	Retailer     string `json:"retailer" validate:"regex=^[\\w\\s\\-&]+$"`
	PurchaseDate string `json:"purchaseDate" validate:"regex=\\d{4}-\\d{2}-\\d{2}"`
	PurchaseTime string `json:"purchaseTime" validate:"regex=\\d{2}:\\d{2}"`
	Items        []item `json:"items" validate:"len=gte:1"`
	Total        string `json:"total" validate:"regex=^\\d+\\.\\d{2}$"`
}

type apiHandler struct {
	db     *sql.DB
	logger *slog.Logger
}

func Bootstrap() (*http.ServeMux, *sql.DB) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS receipts (id VARCHAR PRIMARY KEY NOT NULL, retailer VARCHAR, purchase_date VARCHAR, purchase_time VARCHAR, total VARCHAR)`)
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS items (id VARCHAR NOT NULL, receipt_id VARCHAR NOT NULL, short_description VARCHAR, price VARCHAR, PRIMARY KEY (id, receipt_id))`)
	if err != nil {
		log.Fatal(err)
	}

	h := apiHandler{
		db:     db,
		logger: slog.New(slog.NewJSONHandler(os.Stdout, nil)),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/receipts/process", h.processReceipt)
	mux.HandleFunc("/receipts/{id}/points", h.getReceiptPoints)

	return mux, db
}

func structValidator[T any](s T) error {
	rv := reflect.Indirect(reflect.ValueOf(&s))
	t := reflect.TypeOf(s)
	numFields := t.NumField() - 1

	for numFields >= 0 {
		f := t.Field(numFields)
		tag := f.Tag.Get("validate")

		if tag == "" {
			numFields--
			continue
		}

		v := strings.SplitN(tag, "=", 2)

		switch v[0] {
		case "regex":
			fv := rv.FieldByName(f.Name).Addr().Interface()
			nv, _ := fv.(*string)
			ok, _ := regexp.Match(v[1], []byte(*nv))

			if !ok {
				return errors.New("invalid schema")
			}
		case "len":
			fv := rv.FieldByName(f.Name).Addr().Interface()
			nv, _ := fv.(*[]item)

			op := strings.SplitN(v[1], ":", 2)
			arg, _ := strconv.Atoi(op[1])
			switch op[0] {
			case "gte":
				if len(*nv) < arg {
					return errors.New("invalid schema")
				}
			}

			for _, i := range *nv {
				if err := structValidator[item](i); err != nil {
					return err
				}
			}
		}

		numFields--
	}

	return nil
}

func (r *receipt) calculatePoints() int {
	points := 0
	tLen := len(r.Total)

	// One point for every alphanumeric character in the retailer name.
	for _, b := range []byte(r.Retailer) {
		if b >= 48 && b < 57 || b >= 65 && b < 90 || b >= 97 && b < 122 {
			points++
		}
	}

	// 50 points if the total is a round dollar amount with no cents.
	if r.Total[tLen-2:] == "00" {
		points += 50
	}

	// 25 points if the total is a multiple of 0.25.
	if r.Total[tLen-2:] == "25" || r.Total[tLen-2:] == "50" || r.Total[tLen-2:] == "75" || r.Total[tLen-2:] == "00" {
		points += 25
	}

	// 5 points for every two items on the receipt.
	points += 5 * (len(r.Items) / 2)

	// If the trimmed length of the item description is a multiple of 3, multiply the price by 0.2 and round up to the nearest integer. The result is the number of points earned.
	for _, i := range r.Items {
		if len(strings.Trim(i.ShortDescription, " \t\n\r"))%3 == 0 {
			// Ignore error since we are validating schema which guarantees valid float string
			iP, _ := strconv.ParseFloat(i.Price, 64)
			points += int(math.Ceil(iP * 0.2))
		}
	}

	// 6 points if the day in the purchase date is odd.
	if strings.Contains("13579", r.PurchaseDate[len(r.PurchaseDate)-1:]) {
		points += 6
	}

	// 10 points if the time of purchase is after 2:00pm and before 4:00pm.
	if r.PurchaseTime[0:2] == "14" || r.PurchaseTime[0:2] == "15" {
		points += 10
	}

	return points
}

func (a *apiHandler) returnError(w http.ResponseWriter, e errorResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.StatusCode)

	if err := json.NewEncoder(w).Encode(e); err != nil {
		a.logger.Error("error encoding response for returnError")
	}
}

func (a *apiHandler) getReceiptById(id string) (*receipt, error) {
	r := receipt{}

	row := a.db.QueryRow(fmt.Sprintf(`SELECT * from receipts where receipts.id = '%s'`, id))
	err := row.Scan(&r.ID, &r.Retailer, &r.PurchaseDate, &r.PurchaseTime, &r.Total)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	rows, err := a.db.Query(fmt.Sprintf(`SELECT * from items where items.receipt_id = '%s'`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		i := item{}

		err := rows.Scan(&i.Id, &i.ReceiptId, &i.ShortDescription, &i.Price)
		if err != nil {
			return nil, err
		}

		r.Items = append(r.Items, i)
	}

	if rows.Err() != nil {
		return nil, err
	}

	return &r, nil
}

func (a *apiHandler) processReceipt(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		a.returnError(w, errorResponse{StatusCode: 405, Message: "method not allowed, /receipts/process only accepts POST requests"})
		return
	}

	sR := receipt{ID: uuid.New().String()}

	d := json.NewDecoder(r.Body)
	err := d.Decode(&sR)
	if err != nil {
		a.returnError(w, errorResponse{StatusCode: 500, Message: err.Error()})
		return
	}

	err = structValidator[receipt](sR)
	if err != nil {
		a.returnError(w, errorResponse{StatusCode: 400, Message: "The receipt is invalid"})
		return
	}

	txn, err := a.db.Begin()
	if err != nil {
		log.Fatal(err)
	}
	defer txn.Rollback()

	_, err = txn.Exec(fmt.Sprintf(`INSERT INTO receipts VALUES ('%s', '%s', '%s', '%s', '%s')`, sR.ID, sR.Retailer, sR.PurchaseDate, sR.PurchaseTime, sR.Total))
	if err != nil {
		a.returnError(w, errorResponse{StatusCode: 500, Message: err.Error()})
		return
	}

	for _, i := range sR.Items {
		_, err = txn.Exec(fmt.Sprintf(`INSERT INTO items VALUES ('%s', '%s', '%s', '%s')`, uuid.New().String(), sR.ID, i.ShortDescription, i.Price))
		if err != nil {
			a.returnError(w, errorResponse{StatusCode: 500, Message: err.Error()})
			return
		}
	}

	txn.Commit()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(processReceiptResponse{ID: sR.ID}); err != nil {
		a.logger.Error("error encoding response for processReceipt")
	}
}

func (a *apiHandler) getReceiptPoints(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		a.returnError(w, errorResponse{StatusCode: 405, Message: "method not allowed, /receipts/{id}/points only accepts GET requests"})
		return
	}

	id := r.PathValue("id")
	if id == "" {
		a.returnError(w, errorResponse{StatusCode: 404, Message: "A valid id must be specified, cannot be an empty string"})
		return
	}

	rR, err := a.getReceiptById(id)
	if err != nil {
		a.returnError(w, errorResponse{StatusCode: 500, Message: err.Error()})
		return
	} else if rR == nil {
		a.returnError(w, errorResponse{StatusCode: 404, Message: "No receipt found for that id"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(getReceiptPointsResponse{Points: int64(rR.calculatePoints())}); err != nil {
		a.logger.Error("error encoding response for processReceipt")
	}
}
