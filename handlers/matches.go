package handlers

import (
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/satheeshds/portal/models"
	"github.com/satheeshds/portal/store"
)

const maxSuggestions = 5

// staleDateWindow is the number of days beyond windowDays at which the stale-date score
// reaches zero. Transactions between windowDays and staleDateWindow days from the document
// date receive a reduced "stale_date_proximity" score instead of being hard-excluded.
const staleDateWindow = 180

// MatchSuggestion represents a candidate document that could match a bank statement entry.
type MatchSuggestion struct {
	DocumentType string       `json:"document_type"` // bill, invoice, payout, recurring_payment
	DocumentID   int          `json:"document_id"`
	DocumentRef  string       `json:"document_ref"`  // reference number or name
	DocumentDate string       `json:"document_date"` // relevant date for matching
	Amount       models.Money `json:"amount"`        // total document amount
	Unallocated  models.Money `json:"unallocated"`   // unallocated amount (0 for recurring_payment)
	Confidence   float64      `json:"confidence"`    // 0.0 – 1.0
	MatchReasons []string     `json:"match_reasons"` // human-readable match reasons
	Linkable     bool         `json:"linkable"`      // whether this suggestion can be auto-linked
}

// AutoMatchResult is the result of an auto-match operation.
type AutoMatchResult struct {
	Matched    bool                        `json:"matched"`
	Link       *models.TransactionDocument `json:"link,omitempty"`
	Suggestion *MatchSuggestion            `json:"suggestion,omitempty"`
}

// SuggestMatches returns matching document suggestions for a bank statement entry.
//	@Summary		Suggest document matches for a transaction
//	@Description	Returns a ranked list of bills, invoices, payouts, and recurring payments that could match a bank statement entry, scored by amount, date, and description similarity. Expense transactions match against bills; income transactions match against invoices and payouts. Recurring payments are suggested for both types as informational matches (linkable=false).
//	@Tags			transactions
//	@Produce		json
//	@Param			id	path		int	true	"Transaction ID"
//	@Success		200	{object}	Response{data=[]MatchSuggestion}
//	@Failure		404	{object}	Response{error=string}
//	@Router			/transactions/{id}/match-suggestions [get]
//	@Security		BearerAuth
func SuggestMatches(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
	txnID, _ := strconv.Atoi(chi.URLParam(r, "id"))
	txn, err := s.GetTransaction(txnID)
	if err != nil {
		writeError(w, http.StatusNotFound, "transaction not found")
		return
	}

	if txn.Unallocated <= 0 {
		writeJSON(w, http.StatusOK, []MatchSuggestion{})
		return
	}

	var txnDate time.Time
	if !txn.TransactionDate.IsZero() {
		txnDate = txn.TransactionDate.Time
	}
	txnSearchText := buildMatchSearchText(txn.Description, txn.Reference)

	suggestions := buildMatchSuggestionsStore(s, txn.Type, txn.Unallocated, txnDate, txnSearchText)

	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Confidence > suggestions[j].Confidence
	})

	if len(suggestions) > maxSuggestions {
		suggestions = suggestions[:maxSuggestions]
	}
	if suggestions == nil {
		suggestions = []MatchSuggestion{}
	}
	writeJSON(w, http.StatusOK, suggestions)
}

// AutoMatch automatically links a transaction to the best matching document.
//	@Summary		Auto-match a transaction to a document
//	@Description	Finds the best matching bill, invoice, or payout for a bank statement entry and automatically creates a transaction link when confidence is at least 0.7. Returns the created link on success, or the top suggestion without linking when confidence is below the threshold.
//	@Tags			transactions
//	@Produce		json
//	@Param			id	path		int	true	"Transaction ID"
//	@Success		200	{object}	Response{data=AutoMatchResult}
//	@Failure		404	{object}	Response{error=string}
//	@Router			/transactions/{id}/auto-match [post]
//	@Security		BearerAuth
func AutoMatch(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
	txnID, _ := strconv.Atoi(chi.URLParam(r, "id"))
	txn, err := s.GetTransaction(txnID)
	if err != nil {
		writeError(w, http.StatusNotFound, "transaction not found")
		return
	}

	if txn.Unallocated <= 0 {
		writeJSON(w, http.StatusOK, AutoMatchResult{Matched: false})
		return
	}

	var txnDate time.Time
	if !txn.TransactionDate.IsZero() {
		txnDate = txn.TransactionDate.Time
	}
	txnSearchText := buildMatchSearchText(txn.Description, txn.Reference)

	suggestions := buildMatchSuggestionsStore(s, txn.Type, txn.Unallocated, txnDate, txnSearchText)

	// Sort all suggestions by confidence descending so we can always return the best candidate.
	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Confidence > suggestions[j].Confidence
	})

	// Identify the best overall suggestion (used in matched:false responses for manual review).
	var bestOverall *MatchSuggestion
	if len(suggestions) > 0 {
		bestOverall = &suggestions[0]
	}

	// Only linkable suggestions can be auto-linked; find the best one.
	var bestLinkable *MatchSuggestion
	for i := range suggestions {
		if suggestions[i].Linkable {
			bestLinkable = &suggestions[i]
			break
		}
	}

	if bestLinkable == nil {
		writeJSON(w, http.StatusOK, AutoMatchResult{Matched: false, Suggestion: bestOverall})
		return
	}

	const autoMatchThreshold = 0.7
	if bestLinkable.Confidence < autoMatchThreshold {
		writeJSON(w, http.StatusOK, AutoMatchResult{Matched: false, Suggestion: bestLinkable})
		return
	}

	// Create the transaction link using the full unallocated amount of the transaction
	linkAmount := txn.Unallocated
	td, err := s.CreateTransactionDocumentLink(txnID, bestLinkable.DocumentType, bestLinkable.DocumentID, linkAmount)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.UpdateDocumentStatus(bestLinkable.DocumentType, bestLinkable.DocumentID)
	writeJSON(w, http.StatusOK, AutoMatchResult{Matched: true, Link: &td, Suggestion: bestLinkable})
}

// buildMatchSearchText combines description and reference into a single lowercase search string.
func buildMatchSearchText(desc, ref *string) string {
	var parts []string
	if desc != nil && *desc != "" {
		parts = append(parts, strings.ToLower(*desc))
	}
	if ref != nil && *ref != "" {
		parts = append(parts, strings.ToLower(*ref))
	}
	return strings.Join(parts, " ")
}

// matchDateScore parses docDateStr and delegates to matchDateScoreTime.
func matchDateScore(txnDate time.Time, docDateStr string, windowDays int) (float64, string) {
	if txnDate.IsZero() || docDateStr == "" {
		return 0, ""
	}
	docDate, err := time.Parse("2006-01-02", docDateStr)
	if err != nil {
		return 0, ""
	}
	return matchDateScoreTime(txnDate, docDate, windowDays)
}

// matchDateScoreTime returns a date-proximity score (0–0.4) for two already-parsed dates,
// avoiding redundant string parsing and diff computation in callers that already hold time.Time values.
//
//   - diff == 0              → 0.4,  "exact_date_match"
//   - 0 < diff < windowDays → linear 0.3→0, "date_proximity"
//   - windowDays ≤ diff < staleDateWindow → linear 0.1→0, "stale_date_proximity"
//   - diff ≥ staleDateWindow → 0
func matchDateScoreTime(txnDate, docDate time.Time, windowDays int) (float64, string) {
	if txnDate.IsZero() || docDate.IsZero() {
		return 0, ""
	}
	diff := math.Abs(txnDate.Sub(docDate).Hours() / 24)
	if diff == 0 {
		return 0.4, "exact_date_match"
	}
	if diff < float64(windowDays) {
		score := 0.3 * (1.0 - diff/float64(windowDays))
		return score, "date_proximity"
	}
	if diff < staleDateWindow {
		staleRange := float64(staleDateWindow - windowDays)
		score := 0.1 * (1.0 - (diff-float64(windowDays))/staleRange)
		return score, "stale_date_proximity"
	}
	return 0, ""
}

// matchDescScore returns a score (0–0.2) based on whether docRef or docContext tokens
// appear in txnSearchText.
func matchDescScore(txnSearchText, docRef, docContext string) (float64, string) {
	if txnSearchText == "" || (docRef == "" && docContext == "") {
		return 0, ""
	}
	ref := strings.ToLower(docRef)
	ctx := strings.ToLower(docContext)
	// Strong match: reference number found verbatim in transaction search text
	if ref != "" && strings.Contains(txnSearchText, ref) {
		return 0.2, "reference_match"
	}
	// Weaker match: meaningful token from context appears in search text
	for _, token := range strings.Fields(ctx) {
		if len(token) >= 4 && strings.Contains(txnSearchText, token) {
			return 0.1, "description_match"
		}
	}
	return 0, ""
}

// buildMatchSuggestions collects and scores candidates across all document types.
// It uses the global DB variable for backward compatibility with tests.
func buildMatchSuggestions(txnType string, amount models.Money, txnDate time.Time, txnSearchText string) []MatchSuggestion {
	return buildMatchSuggestionsStore(store.New(DB), txnType, amount, txnDate, txnSearchText)
}

// buildMatchSuggestionsStore collects and scores candidates using the provided Store.
func buildMatchSuggestionsStore(s *store.Store, txnType string, amount models.Money, txnDate time.Time, txnSearchText string) []MatchSuggestion {
	var suggestions []MatchSuggestion
	switch txnType {
	case "expense":
		suggestions = append(suggestions, suggestBills(s, amount, txnDate, txnSearchText)...)
	case "income":
		suggestions = append(suggestions, suggestInvoices(s, amount, txnDate, txnSearchText)...)
		suggestions = append(suggestions, suggestPayouts(s, amount, txnDate, txnSearchText)...)
	}
	// Recurring payments are suggested for both income and expense as informational matches
	suggestions = append(suggestions, suggestRecurringPayments(s, txnType, amount, txnDate, txnSearchText)...)
	return suggestions
}

// suggestBills returns match suggestions from unallocated bills (for expense transactions).
func suggestBills(s *store.Store, amount models.Money, txnDate time.Time, txnSearchText string) []MatchSuggestion {
	if s == nil {
		return nil
	}
	candidates, err := s.SuggestBills(amount, txnDate, txnSearchText)
	if err != nil {
		return nil
	}

	var suggestions []MatchSuggestion
	for _, c := range candidates {
		unallocated := models.Money(int64(c.Amount) - int64(c.Allocated))
		if unallocated <= 0 {
			continue
		}

		var confidence float64
		var reasons []string
		linkable := amount <= unallocated

		if amount == unallocated {
			confidence += 0.5
			reasons = append(reasons, "exact_amount_match")
		} else if amount < unallocated {
			confidence += 0.3
			reasons = append(reasons, "partial_amount_match")
		} else {
			confidence += 0.2
			reasons = append(reasons, "amount_exceeds_unallocated")
		}

		docDate := derefDate(c.DueDate, c.IssueDate)
		if ds, reason := matchDateScore(txnDate, docDate, 30); ds > 0 {
			confidence += ds
			reasons = append(reasons, reason)
		}

		if ds, reason := matchDescScore(txnSearchText, c.BillNumber, c.ContactName+" "+c.Notes); ds > 0 {
			confidence += ds
			reasons = append(reasons, reason)
		}

		suggestions = append(suggestions, MatchSuggestion{
			DocumentType: "bill",
			DocumentID:   c.ID,
			DocumentRef:  c.BillNumber,
			DocumentDate: docDate,
			Amount:       c.Amount,
			Unallocated:  unallocated,
			Confidence:   math.Min(1.0, math.Round(confidence*100)/100),
			MatchReasons: reasons,
			Linkable:     linkable,
		})
	}
	return suggestions
}

// suggestInvoices returns match suggestions from unallocated invoices (for income transactions).
func suggestInvoices(s *store.Store, amount models.Money, txnDate time.Time, txnSearchText string) []MatchSuggestion {
	if s == nil {
		return nil
	}
	candidates, err := s.SuggestInvoices(amount, txnDate, txnSearchText)
	if err != nil {
		return nil
	}

	var suggestions []MatchSuggestion
	for _, c := range candidates {
		unallocated := models.Money(int64(c.Amount) - int64(c.Allocated))
		if unallocated <= 0 {
			continue
		}

		var confidence float64
		var reasons []string
		linkable := amount <= unallocated

		if amount == unallocated {
			confidence += 0.5
			reasons = append(reasons, "exact_amount_match")
		} else if amount < unallocated {
			confidence += 0.3
			reasons = append(reasons, "partial_amount_match")
		} else {
			confidence += 0.2
			reasons = append(reasons, "amount_exceeds_unallocated")
		}

		docDate := derefDate(c.DueDate, c.IssueDate)
		if ds, reason := matchDateScore(txnDate, docDate, 30); ds > 0 {
			confidence += ds
			reasons = append(reasons, reason)
		}

		if ds, reason := matchDescScore(txnSearchText, c.InvoiceNumber, c.ContactName+" "+c.Notes); ds > 0 {
			confidence += ds
			reasons = append(reasons, reason)
		}

		suggestions = append(suggestions, MatchSuggestion{
			DocumentType: "invoice",
			DocumentID:   c.ID,
			DocumentRef:  c.InvoiceNumber,
			DocumentDate: docDate,
			Amount:       c.Amount,
			Unallocated:  unallocated,
			Confidence:   math.Min(1.0, math.Round(confidence*100)/100),
			MatchReasons: reasons,
			Linkable:     linkable,
		})
	}
	return suggestions
}

// suggestPayouts returns match suggestions from unallocated payouts (for income transactions).
func suggestPayouts(s *store.Store, amount models.Money, txnDate time.Time, txnSearchText string) []MatchSuggestion {
	if s == nil {
		return nil
	}
	candidates, err := s.SuggestPayouts(amount, txnDate, txnSearchText)
	if err != nil {
		return nil
	}

	var suggestions []MatchSuggestion
	for _, c := range candidates {
		unallocated := models.Money(int64(c.Amount) - int64(c.Allocated))
		if unallocated <= 0 {
			continue
		}

		var confidence float64
		var reasons []string
		linkable := amount <= unallocated

		if amount == unallocated {
			confidence += 0.5
			reasons = append(reasons, "exact_amount_match")
		} else if amount < unallocated {
			confidence += 0.3
			reasons = append(reasons, "partial_amount_match")
		} else {
			confidence += 0.2
			reasons = append(reasons, "amount_exceeds_unallocated")
		}

		docDate := c.SettlementDate.String()
		if ds, reason := matchDateScore(txnDate, docDate, 7); ds > 0 {
			confidence += ds
			reasons = append(reasons, reason)
		}

		if ds, reason := matchDescScore(txnSearchText, c.UtrNumber, c.OutletName); ds > 0 {
			confidence += ds
			reasons = append(reasons, reason)
		}

		suggestions = append(suggestions, MatchSuggestion{
			DocumentType: "payout",
			DocumentID:   c.ID,
			DocumentRef:  c.UtrNumber,
			DocumentDate: docDate,
			Amount:       c.Amount,
			Unallocated:  unallocated,
			Confidence:   math.Min(1.0, math.Round(confidence*100)/100),
			MatchReasons: reasons,
			Linkable:     linkable,
		})
	}
	return suggestions
}

// suggestRecurringPayments returns match suggestions from pending recurring_payment_occurrences.
func suggestRecurringPayments(s *store.Store, txnType string, amount models.Money, txnDate time.Time, txnSearchText string) []MatchSuggestion {
	if s == nil {
		return nil
	}
	candidates, err := s.SuggestRecurringPaymentOccurrences(txnType, amount, txnDate)
	if err != nil {
		return nil
	}

	var suggestions []MatchSuggestion
	for _, c := range candidates {
		occUnallocated := models.Money(int64(c.Amount) - int64(c.Allocated))

		// Allow a small tolerance of ±2% to accommodate minor variations (taxes, fees, rounding)
		tolerance := models.Money(int64(c.Amount) * 2 / 100)
		diff := int64(amount) - int64(c.Amount)
		if diff < 0 {
			diff = -diff
		}
		if diff > int64(tolerance) {
			continue
		}

		var confidence float64
		var reasons []string

		if amount == c.Amount {
			confidence += 0.5
			reasons = append(reasons, "exact_amount_match")
		} else {
			confidence += 0.3
			reasons = append(reasons, "approximate_amount_match")
		}

		if ds, reason := matchDateScore(txnDate, c.DueDate.String(), 7); ds > 0 {
			confidence += ds
			reasons = append(reasons, reason)
		}

		if ds, reason := matchDescScore(txnSearchText, c.Reference, c.Name+" "+c.Description); ds > 0 {
			confidence += ds
			reasons = append(reasons, reason)
		}

		suggestions = append(suggestions, MatchSuggestion{
			DocumentType: "recurring_payment_occurrence",
			DocumentID:   c.ID,
			DocumentRef:  c.Name,
			DocumentDate: c.DueDate.String(),
			Amount:       c.Amount,
			Unallocated:  occUnallocated,
			Confidence:   math.Min(1.0, math.Round(confidence*100)/100),
			MatchReasons: reasons,
			Linkable:     occUnallocated >= amount,
		})
	}
	return suggestions
}

// TransactionSuggestion represents a candidate bank transaction that could match a financial document.
type TransactionSuggestion struct {
	TransactionID   int          `json:"transaction_id"`
	TransactionDate string       `json:"transaction_date"`
	Description     string       `json:"description"`
	Reference       string       `json:"reference"`
	Amount          models.Money `json:"amount"`      // total transaction amount
	Unallocated     models.Money `json:"unallocated"` // unallocated amount remaining
	Confidence      float64      `json:"confidence"`
	MatchReasons    []string     `json:"match_reasons"`
	Linkable        bool         `json:"linkable"`
}

// getDocumentAllocated returns the total amount already allocated to a document via transaction_documents.
// Kept for use in handlers that haven't been fully migrated.
func getDocumentAllocated(s *store.Store, docType string, docID int) models.Money {
	v, _ := s.GetDocumentAllocated(docType, docID)
	return v
}

// SuggestTransactionsForBill returns ranked bank transaction suggestions for a specific bill.
//	@Summary		Suggest transactions for a bill
//	@Description	Returns a ranked list of unallocated bank transactions that could match a bill, scored by amount, date, and description similarity. Already-linked transactions are excluded.
//	@Tags			bills
//	@Produce		json
//	@Param			id	path		int	true	"Bill ID"
//	@Success		200	{object}	Response{data=[]TransactionSuggestion}
//	@Failure		404	{object}	Response{error=string}
//	@Router			/bills/{id}/match-suggestions [get]
//	@Security		BearerAuth
func SuggestTransactionsForBill(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
	billID, _ := strconv.Atoi(chi.URLParam(r, "id"))

	info, err := s.GetBillForMatching(billID)
	if err != nil {
		writeError(w, http.StatusNotFound, "bill not found")
		return
	}

	unallocated := models.Money(int64(info.Amount) - int64(getDocumentAllocated(s, "bill", billID)))
	if unallocated <= 0 {
		writeJSON(w, http.StatusOK, []TransactionSuggestion{})
		return
	}

	docDate := derefDate(info.DueDate, info.IssueDate)
	docRef := info.BillNumber
	docContext := info.ContactName + " " + info.Notes

	suggestions := suggestTransactionsForDocumentStore(s, "expense", "bill", billID, unallocated, docDate, 30, docRef, docContext)
	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Confidence > suggestions[j].Confidence
	})
	if len(suggestions) > maxSuggestions {
		suggestions = suggestions[:maxSuggestions]
	}
	if suggestions == nil {
		suggestions = []TransactionSuggestion{}
	}
	writeJSON(w, http.StatusOK, suggestions)
}

// SuggestTransactionsForInvoice returns ranked bank transaction suggestions for a specific invoice.
//	@Summary		Suggest transactions for an invoice
//	@Description	Returns a ranked list of unallocated bank transactions that could match an invoice, scored by amount, date, and description similarity. Already-linked transactions are excluded.
//	@Tags			invoices
//	@Produce		json
//	@Param			id	path		int	true	"Invoice ID"
//	@Success		200	{object}	Response{data=[]TransactionSuggestion}
//	@Failure		404	{object}	Response{error=string}
//	@Router			/invoices/{id}/match-suggestions [get]
//	@Security		BearerAuth
func SuggestTransactionsForInvoice(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
	invoiceID, _ := strconv.Atoi(chi.URLParam(r, "id"))

	info, err := s.GetInvoiceForMatching(invoiceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "invoice not found")
		return
	}

	unallocated := models.Money(int64(info.Amount) - int64(getDocumentAllocated(s, "invoice", invoiceID)))
	if unallocated <= 0 {
		writeJSON(w, http.StatusOK, []TransactionSuggestion{})
		return
	}

	docDate := derefDate(info.DueDate, info.IssueDate)
	docRef := info.InvoiceNumber
	docContext := info.ContactName + " " + info.Notes

	suggestions := suggestTransactionsForDocumentStore(s, "income", "invoice", invoiceID, unallocated, docDate, 30, docRef, docContext)
	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Confidence > suggestions[j].Confidence
	})
	if len(suggestions) > maxSuggestions {
		suggestions = suggestions[:maxSuggestions]
	}
	if suggestions == nil {
		suggestions = []TransactionSuggestion{}
	}
	writeJSON(w, http.StatusOK, suggestions)
}

// SuggestTransactionsForPayout returns ranked bank transaction suggestions for a specific payout.
//	@Summary		Suggest transactions for a payout
//	@Description	Returns a ranked list of unallocated bank transactions that could match a payout, scored by amount, date, and UTR/description similarity. Already-linked transactions are excluded.
//	@Tags			payouts
//	@Produce		json
//	@Param			id	path		int	true	"Payout ID"
//	@Success		200	{object}	Response{data=[]TransactionSuggestion}
//	@Failure		404	{object}	Response{error=string}
//	@Router			/payouts/{id}/match-suggestions [get]
//	@Security		BearerAuth
func SuggestTransactionsForPayout(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
	payoutID, _ := strconv.Atoi(chi.URLParam(r, "id"))

	info, err := s.GetPayoutForMatching(payoutID)
	if err != nil {
		writeError(w, http.StatusNotFound, "payout not found")
		return
	}

	unallocated := models.Money(int64(info.Amount) - int64(getDocumentAllocated(s, "payout", payoutID)))
	if unallocated <= 0 {
		writeJSON(w, http.StatusOK, []TransactionSuggestion{})
		return
	}

	docDate := info.SettlementDate.String()

	suggestions := suggestTransactionsForDocumentStore(s, "income", "payout", payoutID, unallocated, docDate, 7, info.UtrNumber, info.OutletName)
	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Confidence > suggestions[j].Confidence
	})
	if len(suggestions) > maxSuggestions {
		suggestions = suggestions[:maxSuggestions]
	}
	if suggestions == nil {
		suggestions = []TransactionSuggestion{}
	}
	writeJSON(w, http.StatusOK, suggestions)
}

// suggestTransactionsForDocumentStore queries transactions that could match the given document using the store.
func suggestTransactionsForDocumentStore(s *store.Store, txnType, docType string, docID int, docUnallocated models.Money,
	docDate string, windowDays int, docRef, docContext string) []TransactionSuggestion {
	if s == nil {
		return nil
	}

	candidates, err := s.SuggestTransactionsForDocument(txnType, docType, docID)
	if err != nil {
		return nil
	}

	var docDate_ time.Time
	if docDate != "" {
		docDate_, _ = time.Parse("2006-01-02", docDate)
	}

	var suggestions []TransactionSuggestion
	for _, c := range candidates {
		txnDateStr := c.Date.String()
		txnUnallocated := models.Money(int64(c.Amount) - int64(c.Allocated))
		if txnUnallocated <= 0 {
			continue
		}

		var confidence float64
		var reasons []string
		linkable := true

		if txnUnallocated == docUnallocated {
			confidence += 0.5
			reasons = append(reasons, "exact_amount_match")
		} else if txnUnallocated < docUnallocated {
			confidence += 0.3
			reasons = append(reasons, "partial_amount_match")
		} else {
			confidence += 0.2
			reasons = append(reasons, "amount_exceeds_unallocated")
		}

		if !docDate_.IsZero() && !c.Date.IsZero() {
			if ds, reason := matchDateScoreTime(c.Date.Time, docDate_, windowDays); ds > 0 {
				confidence += ds
				reasons = append(reasons, reason)
			}
		}

		txnSearchText := strings.ToLower(c.Description + " " + c.Reference)
		if ds, reason := matchDescScore(txnSearchText, docRef, docContext); ds > 0 {
			confidence += ds
			reasons = append(reasons, reason)
		}

		suggestions = append(suggestions, TransactionSuggestion{
			TransactionID:   c.ID,
			TransactionDate: txnDateStr,
			Description:     c.Description,
			Reference:       c.Reference,
			Amount:          c.Amount,
			Unallocated:     txnUnallocated,
			Confidence:      math.Min(1.0, math.Round(confidence*100)/100),
			MatchReasons:    reasons,
			Linkable:        linkable,
		})
	}
	return suggestions
}

// SuggestTransactionsForRecurringPayment returns ranked bank transaction suggestions for a recurring payment.
//	@Summary		Suggest transactions for a recurring payment
//	@Description	Returns a ranked list of bank transactions that could correspond to a recurring payment occurrence, scored by amount (±2% tolerance), date proximity to next_due_date, and description/reference similarity. Excludes transactions already linked to this recurring payment.
//	@Tags			recurring_payments
//	@Produce		json
//	@Param			id	path		int	true	"Recurring Payment ID"
//	@Success		200	{object}	Response{data=[]TransactionSuggestion}
//	@Failure		404	{object}	Response{error=string}
//	@Router			/recurring-payments/{id}/match-suggestions [get]
//	@Security		BearerAuth
func SuggestTransactionsForRecurringPayment(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
	rpID, _ := strconv.Atoi(chi.URLParam(r, "id"))

	info, err := s.GetRecurringPaymentForMatching(rpID)
	if err != nil {
		writeError(w, http.StatusNotFound, "recurring payment not found")
		return
	}

	docDate_ := info.NextDueDate.Time

	candidates, err := s.SuggestTransactionsForRecurringPayment(info.Type, rpID)
	if err != nil {
		writeJSON(w, http.StatusOK, []TransactionSuggestion{})
		return
	}

	tolerance := models.Money(int64(info.Amount) * 2 / 100)

	var suggestions []TransactionSuggestion
	for _, c := range candidates {
		diff := int64(c.Amount) - int64(info.Amount)
		if diff < 0 {
			diff = -diff
		}
		if diff > int64(tolerance) {
			continue
		}

		txnDateStr := c.Date.String()

		var confidence float64
		var reasons []string

		if c.Amount == info.Amount {
			confidence += 0.5
			reasons = append(reasons, "exact_amount_match")
		} else {
			confidence += 0.3
			reasons = append(reasons, "approximate_amount_match")
		}

		if !docDate_.IsZero() && !c.Date.IsZero() {
			if ds, reason := matchDateScoreTime(c.Date.Time, docDate_, 7); ds > 0 {
				confidence += ds
				reasons = append(reasons, reason)
			}
		}

		txnSearchText := strings.ToLower(c.Description + " " + c.Reference)
		if ds, reason := matchDescScore(txnSearchText, info.Reference, info.Name+" "+info.Description); ds > 0 {
			confidence += ds
			reasons = append(reasons, reason)
		}

		txnUnallocated := models.Money(int64(c.Amount) - int64(c.Allocated))

		suggestions = append(suggestions, TransactionSuggestion{
			TransactionID:   c.ID,
			TransactionDate: txnDateStr,
			Description:     c.Description,
			Reference:       c.Reference,
			Amount:          c.Amount,
			Unallocated:     txnUnallocated,
			Confidence:      math.Min(1.0, math.Round(confidence*100)/100),
			MatchReasons:    reasons,
			Linkable:        txnUnallocated > 0,
		})
	}

	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Confidence > suggestions[j].Confidence
	})
	if len(suggestions) > maxSuggestions {
		suggestions = suggestions[:maxSuggestions]
	}
	if suggestions == nil {
		suggestions = []TransactionSuggestion{}
	}
	writeJSON(w, http.StatusOK, suggestions)
}

// derefDate returns the first non-zero date formatted as "YYYY-MM-DD", or "" if all are zero.
func derefDate(dates ...models.Date) string {
	for _, d := range dates {
		if !d.IsZero() {
			return d.String()
		}
	}
	return ""
}
