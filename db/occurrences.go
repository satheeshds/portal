package db

import (
	"fmt"
	"log/slog"
	"time"
)

// GenerateOccurrences creates pending recurring_payment_occurrences for all active recurring
// payments whose next_due_date is on or before today. It is idempotent — each (recurring_payment_id,
// due_date) pair has a UNIQUE constraint, so re-running on an already-generated date is a no-op.
//
// Gap recovery: if the server was offline for months the loop runs until next_due_date > today,
// creating one row per missed period. All missed occurrences appear as "pending" and can be matched
// to the corresponding bank transactions in statement history.
func GenerateOccurrences(database *PortalDB) error {
	today := time.Now().Format("2006-01-02")
	slog.Info("generating recurring payment occurrences", "up_to", today)

	rows, err := database.Query(`
		SELECT id, frequency, interval, next_due_date, end_date, amount
		FROM recurring_payments
		WHERE status = 'active' AND next_due_date IS NOT NULL AND next_due_date <= ?
	`, today)
	if err != nil {
		return fmt.Errorf("query recurring payments: %w", err)
	}
	defer rows.Close()

	type rpRow struct {
		id          int
		frequency   string
		interval    int
		nextDueDate string
		endDate     *string
		amount      int64
	}

	var payments []rpRow
	for rows.Next() {
		var rp rpRow
		if err := rows.Scan(&rp.id, &rp.frequency, &rp.interval, &rp.nextDueDate, &rp.endDate, &rp.amount); err != nil {
			return fmt.Errorf("scan recurring payment: %w", err)
		}
		payments = append(payments, rp)
	}
	rows.Close()

	todayTime, _ := time.Parse("2006-01-02", today)

	for _, rp := range payments {
		nextDue, err := time.Parse("2006-01-02", rp.nextDueDate)
		if err != nil {
			slog.Warn("invalid next_due_date", "recurring_payment_id", rp.id, "date", rp.nextDueDate)
			continue
		}

		completed := false
		for !nextDue.After(todayTime) {
			// Insert occurrence — ON CONFLICT DO NOTHING makes this idempotent
			if _, err := database.Exec(`
				INSERT INTO recurring_payment_occurrences (recurring_payment_id, due_date, amount, status)
				VALUES (?, ?, ?, 'pending')
				ON CONFLICT DO NOTHING
			`, rp.id, nextDue.Format("2006-01-02"), rp.amount); err != nil {
				slog.Error("failed to insert occurrence", "recurring_payment_id", rp.id,
					"due_date", nextDue.Format("2006-01-02"), "error", err)
				return fmt.Errorf("insert recurring_payment_occurrence (recurring_payment_id=%d, due_date=%s): %w",
					rp.id, nextDue.Format("2006-01-02"), err)
			}

			nextDue = AdvanceDate(nextDue, rp.frequency, rp.interval)

			// If a hard end_date is set and we've passed it, mark the schedule completed
			if rp.endDate != nil && *rp.endDate != "" {
				endDate, err := time.Parse("2006-01-02", *rp.endDate)
				if err == nil && nextDue.After(endDate) {
					completed = true
					break
				}
			}
		}

		if completed {
			if _, err := database.Exec(`UPDATE recurring_payments SET status = 'completed', next_due_date = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
				nextDue.Format("2006-01-02"), rp.id); err != nil {
				slog.Error("failed to mark recurring payment completed", "recurring_payment_id", rp.id, "error", err)
			} else {
				slog.Info("recurring payment completed", "recurring_payment_id", rp.id)
			}
		} else {
			if _, err := database.Exec(`UPDATE recurring_payments SET next_due_date = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
				nextDue.Format("2006-01-02"), rp.id); err != nil {
				slog.Error("failed to update next_due_date", "recurring_payment_id", rp.id, "error", err)
			}
		}
	}

	slog.Info("occurrence generation complete", "payments_processed", len(payments))
	return nil
}

// AdvanceDate adds interval * frequency to d and returns the new date.
func AdvanceDate(d time.Time, frequency string, interval int) time.Time {
	switch frequency {
	case "daily":
		return d.AddDate(0, 0, interval)
	case "weekly":
		return d.AddDate(0, 0, interval*7)
	case "monthly":
		return d.AddDate(0, interval, 0)
	case "quarterly":
		return d.AddDate(0, interval*3, 0)
	case "yearly":
		return d.AddDate(interval, 0, 0)
	default:
		return d.AddDate(0, 1, 0)
	}
}
