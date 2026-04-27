package store

import (
	"fmt"
	"strings"
	"time"
)

type nullableDate struct {
	Value *string
}

func (d *nullableDate) Scan(value any) error {
	if value == nil {
		d.Value = nil
		return nil
	}

	var s string
	switch v := value.(type) {
	case time.Time:
		s = v.Format("2006-01-02")
	case string:
		s = v
	case []byte:
		s = string(v)
	case fmt.Stringer:
		s = v.String()
	default:
		return fmt.Errorf("cannot scan %T into nullableDate", value)
	}

	s = strings.TrimSpace(s)
	if s == "" {
		d.Value = nil
		return nil
	}
	if len(s) >= 10 && s[4] == '-' && s[7] == '-' {
		s = s[:10]
	}

	d.Value = &s
	return nil
}
