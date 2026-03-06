package main

//go:generate swag init

import (
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/satheeshds/accounting/db"
	_ "github.com/satheeshds/accounting/docs"
	"github.com/satheeshds/accounting/handlers"
	httpSwagger "github.com/swaggo/http-swagger"
)

//go:embed static/*
var staticFiles embed.FS

// @title           Accounting Software API
// @version         1.0.0
// @description     API for managing accounts, contacts, bills, invoices, and transactions.
// @host            localhost:8080
// @BasePath        /api/v1
// @securityDefinitions.basic  BasicAuth

func main() {
	// Configure structured logging
	level := slog.LevelInfo
	if os.Getenv("LOG_LEVEL") == "debug" {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})))

	// Open database
	database, err := db.Open()
	if err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	// Run migrations
	if err := db.Migrate(database); err != nil {
		slog.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	// Set shared DB for handlers
	handlers.DB = database

	// Router setup
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// API routes with basic auth
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(handlers.BasicAuth)

		// Accounts
		r.Get("/accounts", handlers.ListAccounts)
		r.Post("/accounts", handlers.CreateAccount)
		r.Get("/accounts/{id}", handlers.GetAccount)
		r.Put("/accounts/{id}", handlers.UpdateAccount)
		r.Delete("/accounts/{id}", handlers.DeleteAccount)

		// Contacts
		r.Get("/contacts", handlers.ListContacts)
		r.Post("/contacts", handlers.CreateContact)
		r.Get("/contacts/{id}", handlers.GetContact)
		r.Put("/contacts/{id}", handlers.UpdateContact)
		r.Delete("/contacts/{id}", handlers.DeleteContact)

		// Bills
		r.Get("/bills", handlers.ListBills)
		r.Post("/bills", handlers.CreateBill)
		r.Get("/bills/{id}", handlers.GetBill)
		r.Put("/bills/{id}", handlers.UpdateBill)
		r.Delete("/bills/{id}", handlers.DeleteBill)
		r.Get("/bills/{id}/links", handlers.GetBillLinks)

		// Invoices
		r.Get("/invoices", handlers.ListInvoices)
		r.Post("/invoices", handlers.CreateInvoice)
		r.Get("/invoices/{id}", handlers.GetInvoice)
		r.Put("/invoices/{id}", handlers.UpdateInvoice)
		r.Delete("/invoices/{id}", handlers.DeleteInvoice)
		r.Get("/invoices/{id}/links", handlers.GetInvoiceLinks)

		// Transactions
		r.Get("/transactions", handlers.ListTransactions)
		r.Post("/transactions", handlers.CreateTransaction)
		r.Get("/transactions/{id}", handlers.GetTransaction)
		r.Put("/transactions/{id}", handlers.UpdateTransaction)
		r.Delete("/transactions/{id}", handlers.DeleteTransaction)

		// Transaction document links
		r.Get("/transactions/{id}/links", handlers.ListTransactionLinks)
		r.Post("/transactions/{id}/links", handlers.CreateTransactionLink)
		r.Delete("/transactions/{id}/links/{linkId}", handlers.DeleteTransactionLink)

		// Payment matching
		r.Get("/transactions/{id}/match-suggestions", handlers.SuggestMatches)
		r.Post("/transactions/{id}/auto-match", handlers.AutoMatch)

		// Payouts
		r.Get("/payouts", handlers.ListPayouts)
		r.Post("/payouts", handlers.CreatePayout)
		r.Get("/payouts/{id}", handlers.GetPayout)
		r.Put("/payouts/{id}", handlers.UpdatePayout)
		r.Delete("/payouts/{id}", handlers.DeletePayout)
		r.Get("/payouts/{id}/links", handlers.GetPayoutLinks)

		// Recurring Payments
		r.Get("/recurring-payments", handlers.ListRecurringPayments)
		r.Post("/recurring-payments", handlers.CreateRecurringPayment)
		r.Get("/recurring-payments/{id}", handlers.GetRecurringPayment)
		r.Put("/recurring-payments/{id}", handlers.UpdateRecurringPayment)
		r.Delete("/recurring-payments/{id}", handlers.DeleteRecurringPayment)

		// Dashboard
		r.Get("/dashboard", handlers.GetDashboard)
	})

	// Serve static files (UI)
	staticFS, _ := fs.Sub(staticFiles, "static")
	r.Handle("/*", http.FileServer(http.FS(staticFS)))

	// Swagger UI
	r.Get("/swagger/*", httpSwagger.WrapHandler)

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := fmt.Sprintf(":%s", port)
	slog.Info("server starting", "address", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}
