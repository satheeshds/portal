package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2"
	"github.com/go-chi/chi/v5"
	"github.com/satheeshds/portal/db"
)

func setupTestRouter(t *testing.T) (*chi.Mux, func()) {
	t.Helper()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, fmt.Sprintf("test_portal_%s.db", t.Name()))

	rawDB, err := sql.Open("duckdb", dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	database := db.WrapDB(rawDB)
	if err := db.MigrateDB(database); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Save and restore the global DB variable.
	prevDB := DB
	DB = database

	r := chi.NewRouter()
	r.Post("/api/v1/accounts", CreateAccount)
	r.Post("/api/v1/transactions", CreateTransaction)
	r.Get("/api/v1/transactions/{id}/links", ListTransactionLinks)
	r.Post("/api/v1/transactions/{id}/links", CreateTransactionLink)
	r.Delete("/api/v1/transactions/{id}", DeleteTransaction)
	r.Post("/api/v1/payouts", CreatePayout)
	r.Get("/api/v1/payouts/{id}", GetPayout)
	r.Get("/api/v1/payouts/{id}/links", GetPayoutLinks)
	r.Delete("/api/v1/payouts/{id}", DeletePayout)

	cleanup := func() {
		DB = prevDB
		database.Close()
	}
	return r, cleanup
}

func apiRequest(t *testing.T, r http.Handler, method, path string, body interface{}) (int, map[string]interface{}) {
	t.Helper()
	var reqBody *bytes.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reqBody = bytes.NewReader(data)
	} else {
		reqBody = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reqBody)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var result map[string]interface{}
	json.NewDecoder(w.Body).Decode(&result)
	return w.Code, result
}

// TestPayoutLinksReturnedAfterLinking verifies that GET /payouts/{id}/links
// returns the linked transaction after a link is created via POST /transactions/{id}/links.
func TestPayoutLinksReturnedAfterLinking(t *testing.T) {
	r, cleanup := setupTestRouter(t)
	defer cleanup()

	// Create an account.
	status, resp := apiRequest(t, r, "POST", "/api/v1/accounts", map[string]interface{}{
		"name": "Bank Account", "type": "bank", "opening_balance": 0,
	})
	if status != http.StatusCreated {
		t.Fatalf("create account: status %d, error %v", status, resp["error"])
	}
	accID := int(resp["data"].(map[string]interface{})["id"].(float64))

	// Create a payout.
	status, resp = apiRequest(t, r, "POST", "/api/v1/payouts", map[string]interface{}{
		"outlet_name": "Test Restaurant", "platform": "swiggy",
		"final_payout_amt": 100.0, "total_orders": 5,
		"gross_sales_amt": 120.0, "restaurant_discount_amt": 5.0,
		"platform_commission_amt": 10.0, "taxes_tcs_tds_amt": 5.0,
		"marketing_ads_amt": 0.0, "utr_number": "UTR12345",
	})
	if status != http.StatusCreated {
		t.Fatalf("create payout: status %d, error %v", status, resp["error"])
	}
	payoutID := int(resp["data"].(map[string]interface{})["id"].(float64))

	// Create an income transaction.
	status, resp = apiRequest(t, r, "POST", "/api/v1/transactions", map[string]interface{}{
		"account_id": accID, "type": "income", "amount": 100.0,
		"transaction_date": "2024-01-15", "description": "Swiggy payout",
	})
	if status != http.StatusCreated {
		t.Fatalf("create transaction: status %d, error %v", status, resp["error"])
	}
	txnID := int(resp["data"].(map[string]interface{})["id"].(float64))

	// Link the transaction to the payout.
	status, resp = apiRequest(t, r, "POST", fmt.Sprintf("/api/v1/transactions/%d/links", txnID), map[string]interface{}{
		"document_type": "payout", "document_id": payoutID, "amount": 100.0,
	})
	if status != http.StatusCreated {
		t.Fatalf("create link: status %d, error %v", status, resp["error"])
	}

	// GET /payouts/{id}/links must return the linked transaction.
	status, resp = apiRequest(t, r, "GET", fmt.Sprintf("/api/v1/payouts/%d/links", payoutID), nil)
	if status != http.StatusOK {
		t.Fatalf("get payout links: status %d, error %v", status, resp["error"])
	}
	links := resp["data"].([]interface{})
	if len(links) != 1 {
		t.Errorf("expected 1 payout link, got %d", len(links))
	}
}

// TestPayoutLinksEmptyAfterTransactionDeleted verifies that GET /payouts/{id}/links
// returns an empty list after the linked transaction is deleted, and that the
// payout's allocated amount is reset to zero (no stale allocation).
func TestPayoutLinksEmptyAfterTransactionDeleted(t *testing.T) {
	r, cleanup := setupTestRouter(t)
	defer cleanup()

	// Create an account.
	_, resp := apiRequest(t, r, "POST", "/api/v1/accounts", map[string]interface{}{
		"name": "Bank Account", "type": "bank", "opening_balance": 0,
	})
	accID := int(resp["data"].(map[string]interface{})["id"].(float64))

	// Create a payout.
	_, resp = apiRequest(t, r, "POST", "/api/v1/payouts", map[string]interface{}{
		"outlet_name": "Test Restaurant", "platform": "swiggy",
		"final_payout_amt": 100.0, "total_orders": 5,
		"gross_sales_amt": 120.0, "restaurant_discount_amt": 5.0,
		"platform_commission_amt": 10.0, "taxes_tcs_tds_amt": 5.0,
		"marketing_ads_amt": 0.0, "utr_number": "UTR12345",
	})
	payoutID := int(resp["data"].(map[string]interface{})["id"].(float64))

	// Create a transaction and link it.
	_, resp = apiRequest(t, r, "POST", "/api/v1/transactions", map[string]interface{}{
		"account_id": accID, "type": "income", "amount": 100.0,
		"transaction_date": "2024-01-15", "description": "Swiggy payout",
	})
	txnID := int(resp["data"].(map[string]interface{})["id"].(float64))

	apiRequest(t, r, "POST", fmt.Sprintf("/api/v1/transactions/%d/links", txnID), map[string]interface{}{
		"document_type": "payout", "document_id": payoutID, "amount": 100.0,
	})

	// Verify the link exists before deletion.
	_, resp = apiRequest(t, r, "GET", fmt.Sprintf("/api/v1/payouts/%d/links", payoutID), nil)
	if len(resp["data"].([]interface{})) != 1 {
		t.Fatal("expected 1 link before transaction deletion")
	}

	// Delete the transaction.
	status, resp := apiRequest(t, r, "DELETE", fmt.Sprintf("/api/v1/transactions/%d", txnID), nil)
	if status != http.StatusOK {
		t.Fatalf("delete transaction: status %d, error %v", status, resp["error"])
	}

	// After deletion, GET /payouts/{id}/links must return empty (not stale data).
	status, resp = apiRequest(t, r, "GET", fmt.Sprintf("/api/v1/payouts/%d/links", payoutID), nil)
	if status != http.StatusOK {
		t.Fatalf("get payout links after deletion: status %d", status)
	}
	links := resp["data"].([]interface{})
	if len(links) != 0 {
		t.Errorf("expected 0 payout links after transaction deletion, got %d", len(links))
	}

	// The payout's allocated amount must also be reset to zero.
	status, resp = apiRequest(t, r, "GET", fmt.Sprintf("/api/v1/payouts/%d", payoutID), nil)
	if status != http.StatusOK {
		t.Fatalf("get payout: status %d", status)
	}
	payoutData := resp["data"].(map[string]interface{})
	allocated := int(payoutData["allocated"].(float64))
	if allocated != 0 {
		t.Errorf("expected payout allocated=0 after transaction deletion, got %d", allocated)
	}
}

// TestPayoutLinksEmptyAfterPayoutDeleted verifies that deleting a payout also
// removes its transaction_documents entries, keeping transaction allocated amounts correct.
func TestPayoutLinksEmptyAfterPayoutDeleted(t *testing.T) {
	r, cleanup := setupTestRouter(t)
	defer cleanup()

	_, resp := apiRequest(t, r, "POST", "/api/v1/accounts", map[string]interface{}{
		"name": "Bank Account", "type": "bank", "opening_balance": 0,
	})
	accID := int(resp["data"].(map[string]interface{})["id"].(float64))

	_, resp = apiRequest(t, r, "POST", "/api/v1/payouts", map[string]interface{}{
		"outlet_name": "Test Restaurant", "platform": "swiggy",
		"final_payout_amt": 100.0, "total_orders": 5,
		"gross_sales_amt": 120.0, "restaurant_discount_amt": 5.0,
		"platform_commission_amt": 10.0, "taxes_tcs_tds_amt": 5.0,
		"marketing_ads_amt": 0.0,
	})
	payoutID := int(resp["data"].(map[string]interface{})["id"].(float64))

	_, resp = apiRequest(t, r, "POST", "/api/v1/transactions", map[string]interface{}{
		"account_id": accID, "type": "income", "amount": 100.0,
		"transaction_date": "2024-01-15", "description": "Swiggy payout",
	})
	txnID := int(resp["data"].(map[string]interface{})["id"].(float64))

	apiRequest(t, r, "POST", fmt.Sprintf("/api/v1/transactions/%d/links", txnID), map[string]interface{}{
		"document_type": "payout", "document_id": payoutID, "amount": 100.0,
	})

	// Verify link and allocation before deletion.
	_, resp = apiRequest(t, r, "GET", fmt.Sprintf("/api/v1/payouts/%d/links", payoutID), nil)
	if len(resp["data"].([]interface{})) != 1 {
		t.Fatal("expected 1 link before payout deletion")
	}

	// Delete the payout.
	status, resp := apiRequest(t, r, "DELETE", fmt.Sprintf("/api/v1/payouts/%d", payoutID), nil)
	if status != http.StatusOK {
		t.Fatalf("delete payout: status %d, error %v", status, resp["error"])
	}

	// Transaction's allocated must be 0 after payout deletion (link was cleaned up).
	status, resp = apiRequest(t, r, "GET", fmt.Sprintf("/api/v1/transactions/%d/links", txnID), nil)
	if status != http.StatusOK {
		t.Fatalf("get transaction links: status %d", status)
	}
	links := resp["data"].([]interface{})
	if len(links) != 0 {
		t.Errorf("expected 0 transaction links after payout deletion, got %d", len(links))
	}
}
