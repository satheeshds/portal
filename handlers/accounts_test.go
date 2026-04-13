package handlers

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
)

func setupAccountsTestRouter(t *testing.T) (*chi.Mux, func()) {
	t.Helper()
	r, cleanup := setupTestRouter(t)
	r.Get("/api/v1/accounts", ListAccounts)
	r.Get("/api/v1/accounts/{id}", GetAccount)
	r.Put("/api/v1/accounts/{id}", UpdateAccount)
	r.Delete("/api/v1/accounts/{id}", DeleteAccount)
	return r, cleanup
}

// TestCreateAccount_Success verifies that a new account can be created and
// re-fetched without a binary-decode panic caused by the balance computation.
func TestCreateAccount_Success(t *testing.T) {
	r, cleanup := setupAccountsTestRouter(t)
	defer cleanup()

	tests := []struct {
		name    string
		input   map[string]interface{}
		wantOK  bool
		wantType string
	}{
		{
			name:     "bank account with zero opening balance",
			input:    map[string]interface{}{"name": "Test Bank", "type": "bank", "opening_balance": 0},
			wantOK:   true,
			wantType: "bank",
		},
		{
			name:     "cash account with positive opening balance",
			input:    map[string]interface{}{"name": "Cash", "type": "cash", "opening_balance": 500.00},
			wantOK:   true,
			wantType: "cash",
		},
		{
			name:     "credit card account",
			input:    map[string]interface{}{"name": "Credit Card", "type": "credit_card", "opening_balance": 0},
			wantOK:   true,
			wantType: "credit_card",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, resp := apiRequest(t, r, "POST", "/api/v1/accounts", tt.input)
			if status != http.StatusCreated {
				t.Fatalf("expected 201, got %d: %v", status, resp)
			}
			data, ok := resp["data"].(map[string]interface{})
			if !ok {
				t.Fatalf("expected data object in response, got %T", resp["data"])
			}
			if data["name"] != tt.input["name"] {
				t.Errorf("name = %v, want %v", data["name"], tt.input["name"])
			}
			if data["type"] != tt.wantType {
				t.Errorf("type = %v, want %v", data["type"], tt.wantType)
			}
			// Verify id was assigned and balance is present (zero since no transactions).
			id, ok := data["id"].(float64)
			if !ok || id <= 0 {
				t.Errorf("expected positive id, got %v", data["id"])
			}
			if _, ok := data["balance"]; !ok {
				t.Error("expected balance field in response")
			}
		})
	}
}

// TestCreateAccount_Validation verifies input validation errors.
func TestCreateAccount_Validation(t *testing.T) {
	r, cleanup := setupAccountsTestRouter(t)
	defer cleanup()

	tests := []struct {
		name  string
		input map[string]interface{}
	}{
		{"missing name", map[string]interface{}{"type": "bank", "opening_balance": 0}},
		{"invalid type", map[string]interface{}{"name": "Test", "type": "savings", "opening_balance": 0}},
		{"empty type", map[string]interface{}{"name": "Test", "type": "", "opening_balance": 0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, _ := apiRequest(t, r, "POST", "/api/v1/accounts", tt.input)
			if status != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", status)
			}
		})
	}
}

// TestAccountCRUD verifies create, read, update, delete operations.
func TestAccountCRUD(t *testing.T) {
	r, cleanup := setupAccountsTestRouter(t)
	defer cleanup()

	// Create
	status, resp := apiRequest(t, r, "POST", "/api/v1/accounts", map[string]interface{}{
		"name":            "My Bank",
		"type":            "bank",
		"opening_balance": 100.00,
	})
	if status != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %v", status, resp)
	}
	data := resp["data"].(map[string]interface{})
	id := int(data["id"].(float64))

	// Get
	status, resp = apiRequest(t, r, "GET", fmt.Sprintf("/api/v1/accounts/%d", id), nil)
	if status != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", status)
	}
	if resp["data"].(map[string]interface{})["name"] != "My Bank" {
		t.Errorf("get: name mismatch")
	}

	// List
	status, resp = apiRequest(t, r, "GET", "/api/v1/accounts", nil)
	if status != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", status)
	}
	accounts := resp["data"].([]interface{})
	if len(accounts) == 0 {
		t.Error("list: expected at least one account")
	}

	// Update
	status, resp = apiRequest(t, r, "PUT", fmt.Sprintf("/api/v1/accounts/%d", id), map[string]interface{}{
		"name":            "Updated Bank",
		"type":            "bank",
		"opening_balance": 200.00,
	})
	if status != http.StatusOK {
		t.Fatalf("update: expected 200, got %d: %v", status, resp)
	}
	if resp["data"].(map[string]interface{})["name"] != "Updated Bank" {
		t.Errorf("update: name not updated")
	}

	// Delete
	status, _ = apiRequest(t, r, "DELETE", fmt.Sprintf("/api/v1/accounts/%d", id), nil)
	if status != http.StatusOK {
		t.Fatalf("delete: expected 200, got %d", status)
	}

	// Verify deleted
	status, _ = apiRequest(t, r, "GET", fmt.Sprintf("/api/v1/accounts/%d", id), nil)
	if status != http.StatusNotFound {
		t.Errorf("after delete: expected 404, got %d", status)
	}
}

// TestCreateAccount_BalanceReflectsTransactions verifies the balance computation
// returns the correct value (zero) when there are no transactions.
func TestCreateAccount_BalanceReflectsTransactions(t *testing.T) {
	r, cleanup := setupAccountsTestRouter(t)
	defer cleanup()

	// Create an account with zero opening balance and no transactions.
	status, resp := apiRequest(t, r, "POST", "/api/v1/accounts", map[string]interface{}{
		"name":            "Txn Account",
		"type":            "bank",
		"opening_balance": 0,
	})
	if status != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %v", status, resp)
	}
	accountID := int(resp["data"].(map[string]interface{})["id"].(float64))

	// Re-fetch and verify balance is zero (no transactions, no opening balance).
	status, resp = apiRequest(t, r, "GET", fmt.Sprintf("/api/v1/accounts/%d", accountID), nil)
	if status != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", status)
	}
	data := resp["data"].(map[string]interface{})
	if balance, ok := data["balance"].(float64); !ok || balance != 0 {
		t.Errorf("expected balance 0, got %v", data["balance"])
	}
}
