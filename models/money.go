package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
)

// Money represents a monetary value in paise (the smallest currency unit).
// It can be unmarshaled from JSON as a number (integer or float) or a string.
type Money int64

// UnmarshalJSON implements the json.Unmarshaler interface.
func (m *Money) UnmarshalJSON(data []byte) error {
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}

	switch val := v.(type) {
	case float64:
		// Convert decimal to paise (e.g., 12.34 -> 1234)
		*m = Money(math.Round(val * 100))
	case string:
		// Attempt to parse string as float
		f, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return fmt.Errorf("invalid money string: %s", val)
		}
		*m = Money(math.Round(f * 100))
	case nil:
		*m = 0
	default:
		return fmt.Errorf("invalid money type: %T", val)
	}

	return nil
}

// MarshalJSON implements the json.Marshaler interface.
func (m Money) MarshalJSON() ([]byte, error) {
	return json.Marshal(int64(m))
}

// Scan implements the sql.Scanner interface.
func (m *Money) Scan(value interface{}) error {
	if value == nil {
		*m = 0
		return nil
	}

	switch v := value.(type) {
	case int64:
		*m = Money(v)
	case int32:
		*m = Money(v)
	case int:
		*m = Money(v)
	case float64:
		*m = Money(math.Round(v))
	case []byte:
		i, err := strconv.ParseInt(string(v), 10, 64)
		if err != nil {
			return err
		}
		*m = Money(i)
	case string:
		i, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return fmt.Errorf("cannot scan string %q into Money: %w", v, err)
		}
		*m = Money(i)
	default:
		// Attempt to handle *big.Int via String() method for max compatibility
		if stringer, ok := value.(fmt.Stringer); ok {
			strVal := stringer.String()
			i, err := strconv.ParseInt(strVal, 10, 64)
			if err == nil {
				*m = Money(i)
				return nil
			}
		}
		return fmt.Errorf("cannot scan %T into Money", value)
	}

	return nil
}

// Value implements the driver.Valuer interface.
func (m Money) Value() (driver.Value, error) {
	return int64(m), nil
}

// ToFloat returns the value as a float (e.g., 1234 -> 12.34).
func (m Money) ToFloat() float64 {
	return float64(m) / 100.0
}
