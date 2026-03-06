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

	// Only consider linkable suggestions for auto-matching
	var linkable []MatchSuggestion
	for _, s := range suggestions {
		if s.Linkable {
			linkable = append(linkable, s)
		}
	}

	if len(linkable) == 0 {
		writeJSON(w, http.StatusOK, AutoMatchResult{Matched: false})
		return
	}

	sort.Slice(linkable, func(i, j int) bool {
		return linkable[i].Confidence > linkable[j].Confidence
	})

	best := linkable[0]
	const autoMatchThreshold = 0.7
	if best.Confidence < autoMatchThreshold {
		writeJSON(w, http.StatusOK, AutoMatchResult{Matched: false, Suggestion: &best})
		return
	}

	// Create the transaction link using the full unallocated amount of the transaction
	linkAmount := txn.Unallocated
	var linkID int
	err = DB.QueryRow(`INSERT INTO transaction_documents (transaction_id, document_type, document_id, amount)
		VALUES (?, ?, ?, ?) RETURNING id`, txnID, best.DocumentType, best.DocumentID, linkAmount).Scan(&linkID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	updateDocumentStatus(best.DocumentType, best.DocumentID)

	var td models.TransactionDocument
	DB.QueryRow("SELECT id, transaction_id, document_type, document_id, amount, created_at FROM transaction_documents WHERE id = ?", linkID).
		Scan(&td.ID, &td.TransactionID, &td.DocumentType, &td.DocumentID, &td.Amount, &td.CreatedAt)

	writeJSON(w, http.StatusOK, AutoMatchResult{Matched: true, Link: &td, Suggestion: &best})
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
			COALESCE((SELECT SUM(td.amount) FROM transaction_documents td WHERE td.document_type = 'bill' AND td.document_id = b.id), 0)
		FROM bills b
		LEFT JOIN contacts c ON b.contact_id = c.id
		WHERE b.status NOT IN ('paid', 'cancelled')
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
			COALESCE((SELECT SUM(td.amount) FROM transaction_documents td WHERE td.document_type = 'invoice' AND td.document_id = i.id), 0)
		FROM invoices i
		LEFT JOIN contacts c ON i.contact_id = c.id
		WHERE i.status NOT IN ('received', 'cancelled')
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
			COALESCE((SELECT SUM(td.amount) FROM transaction_documents td WHERE td.document_type = 'payout' AND td.document_id = p.id), 0)
		FROM payouts p
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

// derefDate returns the first non-empty date from the provided pointers.
func derefDate(dates ...*string) string {
	for _, d := range dates {
		if d != nil && *d != "" {
			return *d
		}
	}
	return ""
}
