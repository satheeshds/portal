package handlers

import (
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/satheeshds/accounting/models"
)

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
// @Summary      Suggest document matches for a transaction
// @Description  Returns a ranked list of bills, invoices, payouts, and recurring payments that could match a bank statement entry, scored by amount, date, and description similarity. Expense transactions match against bills; income transactions match against invoices and payouts. Recurring payments are suggested for both types as informational matches (linkable=false).
// @Tags         transactions
// @Produce      json
// @Param        id   path      int  true  "Transaction ID"
// @Success      200  {object}  Response{data=[]MatchSuggestion}
// @Failure      404  {object}  Response{error=string}
// @Router       /transactions/{id}/match-suggestions [get]
// @Security     BasicAuth
func SuggestMatches(w http.ResponseWriter, r *http.Request) {
	txnID, _ := strconv.Atoi(chi.URLParam(r, "id"))
	txn, err := getTransactionByID(txnID)
	if err != nil {
		writeError(w, http.StatusNotFound, "transaction not found")
		return
	}

	if txn.Unallocated <= 0 {
		writeJSON(w, http.StatusOK, []MatchSuggestion{})
		return
	}

	var txnDate time.Time
	if txn.TransactionDate != nil && *txn.TransactionDate != "" {
		txnDate, _ = time.Parse("2006-01-02", *txn.TransactionDate)
	}
	txnSearchText := buildMatchSearchText(txn.Description, txn.Reference)

	suggestions := buildMatchSuggestions(txn.Type, txn.Unallocated, txnDate, txnSearchText)

	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Confidence > suggestions[j].Confidence
	})

	if suggestions == nil {
		suggestions = []MatchSuggestion{}
	}
	writeJSON(w, http.StatusOK, suggestions)
}

// AutoMatch automatically links a transaction to the best matching document.
// @Summary      Auto-match a transaction to a document
// @Description  Finds the best matching bill, invoice, or payout for a bank statement entry and automatically creates a transaction link when confidence is at least 0.7. Returns the created link on success, or the top suggestion without linking when confidence is below the threshold.
// @Tags         transactions
// @Produce      json
// @Param        id   path      int  true  "Transaction ID"
// @Success      200  {object}  Response{data=AutoMatchResult}
// @Failure      404  {object}  Response{error=string}
// @Router       /transactions/{id}/auto-match [post]
// @Security     BasicAuth
func AutoMatch(w http.ResponseWriter, r *http.Request) {
	txnID, _ := strconv.Atoi(chi.URLParam(r, "id"))
	txn, err := getTransactionByID(txnID)
	if err != nil {
		writeError(w, http.StatusNotFound, "transaction not found")
		return
	}

	if txn.Unallocated <= 0 {
		writeJSON(w, http.StatusOK, AutoMatchResult{Matched: false})
		return
	}

	var txnDate time.Time
	if txn.TransactionDate != nil && *txn.TransactionDate != "" {
		txnDate, _ = time.Parse("2006-01-02", *txn.TransactionDate)
	}
	txnSearchText := buildMatchSearchText(txn.Description, txn.Reference)

	suggestions := buildMatchSuggestions(txn.Type, txn.Unallocated, txnDate, txnSearchText)

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
		// No linkable candidate — return the best informational suggestion so callers can prompt manually.
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
	var linkID int
	err = DB.QueryRow(`INSERT INTO transaction_documents (transaction_id, document_type, document_id, amount)
		VALUES (?, ?, ?, ?) RETURNING id`, txnID, bestLinkable.DocumentType, bestLinkable.DocumentID, linkAmount).Scan(&linkID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	updateDocumentStatus(bestLinkable.DocumentType, bestLinkable.DocumentID)

	var td models.TransactionDocument
	err = DB.QueryRow("SELECT id, transaction_id, document_type, document_id, amount, created_at FROM transaction_documents WHERE id = ?", linkID).
		Scan(&td.ID, &td.TransactionID, &td.DocumentType, &td.DocumentID, &td.Amount, &td.CreatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

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

// matchDateScore returns a score (0–0.3) based on how close docDateStr is to txnDate.
// windowDays is the maximum number of days for any score to be returned.
func matchDateScore(txnDate time.Time, docDateStr string, windowDays int) (float64, string) {
	if txnDate.IsZero() || docDateStr == "" {
		return 0, ""
	}
	docDate, err := time.Parse("2006-01-02", docDateStr)
	if err != nil {
		return 0, ""
	}
	diff := math.Abs(txnDate.Sub(docDate).Hours() / 24)
	if diff >= float64(windowDays) {
		return 0, ""
	}
	score := 0.3 * (1.0 - diff/float64(windowDays))
	return score, "date_proximity"
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
func buildMatchSuggestions(txnType string, amount models.Money, txnDate time.Time, txnSearchText string) []MatchSuggestion {
	var suggestions []MatchSuggestion
	switch txnType {
	case "expense":
		suggestions = append(suggestions, suggestBills(amount, txnDate, txnSearchText)...)
	case "income":
		suggestions = append(suggestions, suggestInvoices(amount, txnDate, txnSearchText)...)
		suggestions = append(suggestions, suggestPayouts(amount, txnDate, txnSearchText)...)
	}
	// Recurring payments are suggested for both income and expense as informational matches
	suggestions = append(suggestions, suggestRecurringPayments(txnType, amount, txnDate, txnSearchText)...)
	return suggestions
}

// suggestBills returns match suggestions from unallocated bills (for expense transactions).
func suggestBills(amount models.Money, txnDate time.Time, txnSearchText string) []MatchSuggestion {
	if DB == nil {
		return nil
	}
	rows, err := DB.Query(`
		SELECT b.id, COALESCE(b.bill_number, ''), b.due_date, b.issue_date,
			b.amount, COALESCE(b.notes, ''), COALESCE(c.name, ''),
			COALESCE(a.total_allocated, 0)
		FROM bills b
		LEFT JOIN contacts c ON b.contact_id = c.id
		LEFT JOIN (
			SELECT document_id, SUM(amount) AS total_allocated
			FROM transaction_documents
			WHERE document_type = 'bill'
			GROUP BY document_id
		) a ON a.document_id = b.id
		WHERE b.status NOT IN ('paid', 'cancelled')
		  AND b.amount > COALESCE(a.total_allocated, 0)
	`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var suggestions []MatchSuggestion
	for rows.Next() {
		var id int
		var billNum, notes, contactName string
		var dueDate, issueDate *string
		var totalAmt, allocated models.Money
		if err := rows.Scan(&id, &billNum, &dueDate, &issueDate, &totalAmt, &notes, &contactName, &allocated); err != nil {
			continue
		}
		unallocated := models.Money(int64(totalAmt) - int64(allocated))
		if unallocated <= 0 {
			continue
		}

		// Determine amount match quality and linkability
		// A suggestion is linkable only when the transaction can be fully allocated to this document.
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
			// amount > unallocated: transaction exceeds document's remaining balance;
			// still a candidate but needs manual partial allocation.
			confidence += 0.2
			reasons = append(reasons, "amount_exceeds_unallocated")
		}

		docDate := derefDate(dueDate, issueDate)
		if ds, reason := matchDateScore(txnDate, docDate, 30); ds > 0 {
			confidence += ds
			reasons = append(reasons, reason)
		}

		if ds, reason := matchDescScore(txnSearchText, billNum, contactName+" "+notes); ds > 0 {
			confidence += ds
			reasons = append(reasons, reason)
		}

		suggestions = append(suggestions, MatchSuggestion{
			DocumentType: "bill",
			DocumentID:   id,
			DocumentRef:  billNum,
			DocumentDate: docDate,
			Amount:       totalAmt,
			Unallocated:  unallocated,
			Confidence:   math.Round(confidence*100) / 100,
			MatchReasons: reasons,
			Linkable:     linkable,
		})
	}
	return suggestions
}

// suggestInvoices returns match suggestions from unallocated invoices (for income transactions).
func suggestInvoices(amount models.Money, txnDate time.Time, txnSearchText string) []MatchSuggestion {
	if DB == nil {
		return nil
	}
	rows, err := DB.Query(`
		SELECT i.id, COALESCE(i.invoice_number, ''), i.due_date, i.issue_date,
			i.amount, COALESCE(i.notes, ''), COALESCE(c.name, ''),
			COALESCE(a.total_allocated, 0)
		FROM invoices i
		LEFT JOIN contacts c ON i.contact_id = c.id
		LEFT JOIN (
			SELECT document_id, SUM(amount) AS total_allocated
			FROM transaction_documents
			WHERE document_type = 'invoice'
			GROUP BY document_id
		) a ON a.document_id = i.id
		WHERE i.status NOT IN ('received', 'cancelled')
		  AND i.amount > COALESCE(a.total_allocated, 0)
	`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var suggestions []MatchSuggestion
	for rows.Next() {
		var id int
		var invoiceNum, notes, contactName string
		var dueDate, issueDate *string
		var totalAmt, allocated models.Money
		if err := rows.Scan(&id, &invoiceNum, &dueDate, &issueDate, &totalAmt, &notes, &contactName, &allocated); err != nil {
			continue
		}
		unallocated := models.Money(int64(totalAmt) - int64(allocated))
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

		docDate := derefDate(dueDate, issueDate)
		if ds, reason := matchDateScore(txnDate, docDate, 30); ds > 0 {
			confidence += ds
			reasons = append(reasons, reason)
		}

		if ds, reason := matchDescScore(txnSearchText, invoiceNum, contactName+" "+notes); ds > 0 {
			confidence += ds
			reasons = append(reasons, reason)
		}

		suggestions = append(suggestions, MatchSuggestion{
			DocumentType: "invoice",
			DocumentID:   id,
			DocumentRef:  invoiceNum,
			DocumentDate: docDate,
			Amount:       totalAmt,
			Unallocated:  unallocated,
			Confidence:   math.Round(confidence*100) / 100,
			MatchReasons: reasons,
			Linkable:     linkable,
		})
	}
	return suggestions
}

// suggestPayouts returns match suggestions from unallocated payouts (for income transactions).
func suggestPayouts(amount models.Money, txnDate time.Time, txnSearchText string) []MatchSuggestion {
	if DB == nil {
		return nil
	}
	rows, err := DB.Query(`
		SELECT p.id, COALESCE(p.utr_number, ''), p.settlement_date,
			p.final_payout_amt, COALESCE(p.outlet_name, ''),
			COALESCE(a.total_allocated, 0)
		FROM payouts p
		LEFT JOIN (
			SELECT document_id, SUM(amount) AS total_allocated
			FROM transaction_documents
			WHERE document_type = 'payout'
			GROUP BY document_id
		) a ON a.document_id = p.id
		WHERE p.final_payout_amt > COALESCE(a.total_allocated, 0)
	`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var suggestions []MatchSuggestion
	for rows.Next() {
		var id int
		var utrNumber, outletName string
		var settlementDate *string
		var totalAmt, allocated models.Money
		if err := rows.Scan(&id, &utrNumber, &settlementDate, &totalAmt, &outletName, &allocated); err != nil {
			continue
		}
		unallocated := models.Money(int64(totalAmt) - int64(allocated))
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

		docDate := ""
		if settlementDate != nil {
			docDate = *settlementDate
		}
		// Payouts are expected to settle within 7 days of the bank entry
		if ds, reason := matchDateScore(txnDate, docDate, 7); ds > 0 {
			confidence += ds
			reasons = append(reasons, reason)
		}

		// UTR number is a strong identifier for payouts
		if ds, reason := matchDescScore(txnSearchText, utrNumber, outletName); ds > 0 {
			confidence += ds
			reasons = append(reasons, reason)
		}

		suggestions = append(suggestions, MatchSuggestion{
			DocumentType: "payout",
			DocumentID:   id,
			DocumentRef:  utrNumber,
			DocumentDate: docDate,
			Amount:       totalAmt,
			Unallocated:  unallocated,
			Confidence:   math.Round(confidence*100) / 100,
			MatchReasons: reasons,
			Linkable:     linkable,
		})
	}
	return suggestions
}

// suggestRecurringPayments returns informational match suggestions from active recurring payments.
// These are not directly linkable via transaction_documents and serve as hints only.
func suggestRecurringPayments(txnType string, amount models.Money, txnDate time.Time, txnSearchText string) []MatchSuggestion {
	if DB == nil {
		return nil
	}
	rows, err := DB.Query(`
		SELECT r.id, r.name, r.next_due_date, r.amount, COALESCE(r.description, ''), COALESCE(r.reference, '')
		FROM recurring_payments r
		WHERE r.status = 'active' AND r.type = ?
	`, txnType)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var suggestions []MatchSuggestion
	for rows.Next() {
		var id int
		var name, desc, ref string
		var nextDueDate *string
		var rpAmount models.Money
		if err := rows.Scan(&id, &name, &nextDueDate, &rpAmount, &desc, &ref); err != nil {
			continue
		}

		// Allow a small tolerance of ±2% to accommodate minor variations (taxes, fees, rounding)
		tolerance := models.Money(int64(rpAmount) * 2 / 100)
		diff := int64(amount) - int64(rpAmount)
		if diff < 0 {
			diff = -diff
		}
		if diff > int64(tolerance) {
			continue
		}

		var confidence float64
		var reasons []string

		if amount == rpAmount {
			confidence += 0.5
			reasons = append(reasons, "exact_amount_match")
		} else {
			confidence += 0.3
			reasons = append(reasons, "approximate_amount_match")
		}

		docDate := ""
		if nextDueDate != nil {
			docDate = *nextDueDate
		}
		if ds, reason := matchDateScore(txnDate, docDate, 7); ds > 0 {
			confidence += ds
			reasons = append(reasons, reason)
		}

		if ds, reason := matchDescScore(txnSearchText, ref, name+" "+desc); ds > 0 {
			confidence += ds
			reasons = append(reasons, reason)
		}

		suggestions = append(suggestions, MatchSuggestion{
			DocumentType: "recurring_payment",
			DocumentID:   id,
			DocumentRef:  name,
			DocumentDate: docDate,
			Amount:       rpAmount,
			Unallocated:  0,
			Confidence:   math.Round(confidence*100) / 100,
			MatchReasons: reasons,
			Linkable:     false,
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
func getDocumentAllocated(docType string, docID int) models.Money {
	var allocated models.Money
	DB.QueryRow("SELECT COALESCE(SUM(amount), 0) FROM transaction_documents WHERE document_type = ? AND document_id = ?", docType, docID).Scan(&allocated)
	return allocated
}

// SuggestTransactionsForBill returns ranked bank transaction suggestions for a specific bill.
// @Summary      Suggest transactions for a bill
// @Description  Returns a ranked list of unallocated bank transactions that could match a bill, scored by amount, date, and description similarity. Already-linked transactions are excluded.
// @Tags         bills
// @Produce      json
// @Param        id   path      int  true  "Bill ID"
// @Success      200  {object}  Response{data=[]TransactionSuggestion}
// @Failure      404  {object}  Response{error=string}
// @Router       /bills/{id}/match-suggestions [get]
// @Security     BasicAuth
func SuggestTransactionsForBill(w http.ResponseWriter, r *http.Request) {
	billID, _ := strconv.Atoi(chi.URLParam(r, "id"))

	var totalAmt models.Money
	var billNum, notes string
	var dueDate, issueDate *string
	var contactName string
	err := DB.QueryRow(`
		SELECT b.amount, COALESCE(b.bill_number, ''), b.due_date, b.issue_date,
			COALESCE(b.notes, ''), COALESCE(c.name, '')
		FROM bills b
		LEFT JOIN contacts c ON b.contact_id = c.id
		WHERE b.id = ?`, billID).Scan(&totalAmt, &billNum, &dueDate, &issueDate, &notes, &contactName)
	if err != nil {
		writeError(w, http.StatusNotFound, "bill not found")
		return
	}

	unallocated := models.Money(int64(totalAmt) - int64(getDocumentAllocated("bill", billID)))
	if unallocated <= 0 {
		writeJSON(w, http.StatusOK, []TransactionSuggestion{})
		return
	}

	docDate := derefDate(dueDate, issueDate)
	docRef := billNum
	docContext := contactName + " " + notes

	suggestions := suggestTransactionsForDocument("expense", "bill", billID, unallocated, docDate, 30, docRef, docContext)
	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Confidence > suggestions[j].Confidence
	})
	if suggestions == nil {
		suggestions = []TransactionSuggestion{}
	}
	writeJSON(w, http.StatusOK, suggestions)
}

// SuggestTransactionsForInvoice returns ranked bank transaction suggestions for a specific invoice.
// @Summary      Suggest transactions for an invoice
// @Description  Returns a ranked list of unallocated bank transactions that could match an invoice, scored by amount, date, and description similarity. Already-linked transactions are excluded.
// @Tags         invoices
// @Produce      json
// @Param        id   path      int  true  "Invoice ID"
// @Success      200  {object}  Response{data=[]TransactionSuggestion}
// @Failure      404  {object}  Response{error=string}
// @Router       /invoices/{id}/match-suggestions [get]
// @Security     BasicAuth
func SuggestTransactionsForInvoice(w http.ResponseWriter, r *http.Request) {
	invoiceID, _ := strconv.Atoi(chi.URLParam(r, "id"))

	var totalAmt models.Money
	var invoiceNum, notes string
	var dueDate, issueDate *string
	var contactName string
	err := DB.QueryRow(`
		SELECT i.amount, COALESCE(i.invoice_number, ''), i.due_date, i.issue_date,
			COALESCE(i.notes, ''), COALESCE(c.name, '')
		FROM invoices i
		LEFT JOIN contacts c ON i.contact_id = c.id
		WHERE i.id = ?`, invoiceID).Scan(&totalAmt, &invoiceNum, &dueDate, &issueDate, &notes, &contactName)
	if err != nil {
		writeError(w, http.StatusNotFound, "invoice not found")
		return
	}

	unallocated := models.Money(int64(totalAmt) - int64(getDocumentAllocated("invoice", invoiceID)))
	if unallocated <= 0 {
		writeJSON(w, http.StatusOK, []TransactionSuggestion{})
		return
	}

	docDate := derefDate(dueDate, issueDate)
	docRef := invoiceNum
	docContext := contactName + " " + notes

	suggestions := suggestTransactionsForDocument("income", "invoice", invoiceID, unallocated, docDate, 30, docRef, docContext)
	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Confidence > suggestions[j].Confidence
	})
	if suggestions == nil {
		suggestions = []TransactionSuggestion{}
	}
	writeJSON(w, http.StatusOK, suggestions)
}

// SuggestTransactionsForPayout returns ranked bank transaction suggestions for a specific payout.
// @Summary      Suggest transactions for a payout
// @Description  Returns a ranked list of unallocated bank transactions that could match a payout, scored by amount, date, and UTR/description similarity. Already-linked transactions are excluded.
// @Tags         payouts
// @Produce      json
// @Param        id   path      int  true  "Payout ID"
// @Success      200  {object}  Response{data=[]TransactionSuggestion}
// @Failure      404  {object}  Response{error=string}
// @Router       /payouts/{id}/match-suggestions [get]
// @Security     BasicAuth
func SuggestTransactionsForPayout(w http.ResponseWriter, r *http.Request) {
	payoutID, _ := strconv.Atoi(chi.URLParam(r, "id"))

	var totalAmt models.Money
	var utrNumber, outletName string
	var settlementDate *string
	err := DB.QueryRow(`
		SELECT p.final_payout_amt, COALESCE(p.utr_number, ''), p.settlement_date, COALESCE(p.outlet_name, '')
		FROM payouts p
		WHERE p.id = ?`, payoutID).Scan(&totalAmt, &utrNumber, &settlementDate, &outletName)
	if err != nil {
		writeError(w, http.StatusNotFound, "payout not found")
		return
	}

	unallocated := models.Money(int64(totalAmt) - int64(getDocumentAllocated("payout", payoutID)))
	if unallocated <= 0 {
		writeJSON(w, http.StatusOK, []TransactionSuggestion{})
		return
	}

	docDate := ""
	if settlementDate != nil {
		docDate = *settlementDate
	}

	suggestions := suggestTransactionsForDocument("income", "payout", payoutID, unallocated, docDate, 7, utrNumber, outletName)
	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Confidence > suggestions[j].Confidence
	})
	if suggestions == nil {
		suggestions = []TransactionSuggestion{}
	}
	writeJSON(w, http.StatusOK, suggestions)
}

// suggestTransactionsForDocument queries expense/income transactions that could match the given document,
// excluding transactions already linked to this document.
func suggestTransactionsForDocument(txnType, docType string, docID int, docUnallocated models.Money,
	docDate string, windowDays int, docRef, docContext string) []TransactionSuggestion {
	if DB == nil {
		return nil
	}

	// Only consider transactions with remaining unallocated balance and not already linked to this document
	rows, err := DB.Query(`
		SELECT t.id, t.amount, t.transaction_date, COALESCE(t.description, ''), COALESCE(t.reference, ''),
			COALESCE((SELECT SUM(td.amount) FROM transaction_documents td WHERE td.transaction_id = t.id), 0)
		FROM transactions t
		WHERE t.type = ?
		AND NOT EXISTS (
			SELECT 1 FROM transaction_documents td2
			WHERE td2.transaction_id = t.id
			AND td2.document_type = ?
			AND td2.document_id = ?
		)
	`, txnType, docType, docID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var docDate_ time.Time
	if docDate != "" {
		docDate_, _ = time.Parse("2006-01-02", docDate)
	}

	var suggestions []TransactionSuggestion
	for rows.Next() {
		var id int
		var txnAmt, txnAllocated models.Money
		var txnDatePtr *string
		var desc, ref string
		if err := rows.Scan(&id, &txnAmt, &txnDatePtr, &desc, &ref, &txnAllocated); err != nil {
			continue
		}
		txnDateStr := ""
		if txnDatePtr != nil {
			txnDateStr = *txnDatePtr
		}
		txnUnallocated := models.Money(int64(txnAmt) - int64(txnAllocated))
		if txnUnallocated <= 0 {
			continue
		}

		var confidence float64
		var reasons []string
		// A link can always be created for min(txnUnallocated, docUnallocated) paise,
		// so linkable is always true as long as both sides have remaining balance.
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

		// Date proximity: compare transaction date to document date
		if !docDate_.IsZero() && txnDateStr != "" {
			txnDate, err := time.Parse("2006-01-02", txnDateStr)
			if err == nil {
				diff := math.Abs(txnDate.Sub(docDate_).Hours() / 24)
				if diff < float64(windowDays) {
					score := 0.3 * (1.0 - diff/float64(windowDays))
					confidence += score
					reasons = append(reasons, "date_proximity")
				}
			}
		}

		// Description / reference similarity
		txnSearchText := strings.ToLower(desc + " " + ref)
		if ds, reason := matchDescScore(txnSearchText, docRef, docContext); ds > 0 {
			confidence += ds
			reasons = append(reasons, reason)
		}

		suggestions = append(suggestions, TransactionSuggestion{
			TransactionID:   id,
			TransactionDate: txnDateStr,
			Description:     desc,
			Reference:       ref,
			Amount:          txnAmt,
			Unallocated:     txnUnallocated,
			Confidence:      math.Round(confidence*100) / 100,
			MatchReasons:    reasons,
			Linkable:        linkable,
		})
	}
	return suggestions
}

// SuggestTransactionsForRecurringPayment returns ranked bank transaction suggestions for a recurring payment.
// @Summary      Suggest transactions for a recurring payment
// @Description  Returns a ranked list of bank transactions that could correspond to a recurring payment occurrence, scored by amount (±2% tolerance), date proximity to next_due_date, and description/reference similarity. These are informational hints only (linkable=false) since recurring payments are not formally linked via transaction_documents.
// @Tags         recurring_payments
// @Produce      json
// @Param        id   path      int  true  "Recurring Payment ID"
// @Success      200  {object}  Response{data=[]TransactionSuggestion}
// @Failure      404  {object}  Response{error=string}
// @Router       /recurring-payments/{id}/match-suggestions [get]
// @Security     BasicAuth
func SuggestTransactionsForRecurringPayment(w http.ResponseWriter, r *http.Request) {
	rpID, _ := strconv.Atoi(chi.URLParam(r, "id"))

	if DB == nil {
		writeJSON(w, http.StatusOK, []TransactionSuggestion{})
		return
	}

	var rpAmount models.Money
	var rpType, name, desc, ref string
	var nextDueDate *string
	err := DB.QueryRow(`
		SELECT r.amount, r.type, r.name, COALESCE(r.description, ''), COALESCE(r.reference, ''), r.next_due_date
		FROM recurring_payments r
		WHERE r.id = ?`, rpID).Scan(&rpAmount, &rpType, &name, &desc, &ref, &nextDueDate)
	if err != nil {
		writeError(w, http.StatusNotFound, "recurring payment not found")
		return
	}

	docDate := ""
	if nextDueDate != nil {
		docDate = *nextDueDate
	}

	// Query matching transactions by type; no exclusion via transaction_documents
	// since recurring payments are not formally linked to transactions.
	rows, err := DB.Query(`
		SELECT t.id, t.amount, t.transaction_date, COALESCE(t.description, ''), COALESCE(t.reference, ''),
			COALESCE((SELECT SUM(td.amount) FROM transaction_documents td WHERE td.transaction_id = t.id), 0)
		FROM transactions t
		WHERE t.type = ?
	`, rpType)
	if err != nil {
		writeJSON(w, http.StatusOK, []TransactionSuggestion{})
		return
	}
	defer rows.Close()

	var docDate_ time.Time
	if docDate != "" {
		docDate_, _ = time.Parse("2006-01-02", docDate)
	}

	// ±2% tolerance on the recurring payment amount
	tolerance := models.Money(int64(rpAmount) * 2 / 100)

	var suggestions []TransactionSuggestion
	for rows.Next() {
		var id int
		var txnAmt, txnAllocated models.Money
		var txnDatePtr *string
		var txnDesc, txnRef string
		if err := rows.Scan(&id, &txnAmt, &txnDatePtr, &txnDesc, &txnRef, &txnAllocated); err != nil {
			continue
		}

		// Apply ±2% amount tolerance (same as forward direction for recurring payments)
		diff := int64(txnAmt) - int64(rpAmount)
		if diff < 0 {
			diff = -diff
		}
		if diff > int64(tolerance) {
			continue
		}

		txnDateStr := ""
		if txnDatePtr != nil {
			txnDateStr = *txnDatePtr
		}

		var confidence float64
		var reasons []string

		if txnAmt == rpAmount {
			confidence += 0.5
			reasons = append(reasons, "exact_amount_match")
		} else {
			confidence += 0.3
			reasons = append(reasons, "approximate_amount_match")
		}

		// Date proximity to next_due_date (7-day window, same as forward direction)
		if !docDate_.IsZero() && txnDateStr != "" {
			txnDate, err := time.Parse("2006-01-02", txnDateStr)
			if err == nil {
				diff := math.Abs(txnDate.Sub(docDate_).Hours() / 24)
				if diff < 7 {
					score := 0.3 * (1.0 - diff/7)
					confidence += score
					reasons = append(reasons, "date_proximity")
				}
			}
		}

		txnSearchText := strings.ToLower(txnDesc + " " + txnRef)
		if ds, reason := matchDescScore(txnSearchText, ref, name+" "+desc); ds > 0 {
			confidence += ds
			reasons = append(reasons, reason)
		}

		txnUnallocated := models.Money(int64(txnAmt) - int64(txnAllocated))

		suggestions = append(suggestions, TransactionSuggestion{
			TransactionID:   id,
			TransactionDate: txnDateStr,
			Description:     txnDesc,
			Reference:       txnRef,
			Amount:          txnAmt,
			Unallocated:     txnUnallocated,
			Confidence:      math.Round(confidence*100) / 100,
			MatchReasons:    reasons,
			Linkable:        false, // recurring payments are not formally linked via transaction_documents
		})
	}

	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Confidence > suggestions[j].Confidence
	})
	if suggestions == nil {
		suggestions = []TransactionSuggestion{}
	}
	writeJSON(w, http.StatusOK, suggestions)
}

// derefDate returns the first non-empty date from the provided pointers.
func derefDate(dates ...*string) string {
	for _, d := range dates {
		if d != nil && *d != "" {
			return *d
		}
	}
	return ""
}
