package starlings

import (
	"database/sql/driver"
	"encoding/json/jsontext"
	"fmt"
	"strconv"
	"time"
)

// discordEpoch is the first millisecond of 2015, the origin Discord counts
// snowflake timestamps from.
const discordEpoch = 1420070400000

// Snowflake is a Discord object ID. Discord generates these as 64-bit integers
// but sends them over the wire as decimal strings, because JavaScript cannot
// represent them exactly as numbers. Snowflake keeps the integer form in memory
// and handles the string conversion at the JSON boundary.
type Snowflake uint64

// ParseSnowflake converts a decimal ID string into a Snowflake.
func ParseSnowflake(s string) (Snowflake, error) {
	n, err := strconv.ParseUint(s, 10, 64)
	return Snowflake(n), err
}

// String returns the decimal form Discord's API expects in URLs and payloads.
func (s Snowflake) String() string { return strconv.FormatUint(uint64(s), 10) }

// IsZero reports whether the snowflake is unset. Discord never issues an ID of
// zero, so it doubles as the "absent" value.
func (s Snowflake) IsZero() bool { return s == 0 }

// Time returns when Discord created the object. The top 42 bits of a snowflake
// are a millisecond timestamp, so this needs no API call.
func (s Snowflake) Time() time.Time {
	ms := int64(s>>22) + discordEpoch
	return time.UnixMilli(ms).UTC()
}

// MarshalJSON encodes the snowflake as a JSON string, matching Discord's wire
// format. A zero snowflake encodes as null so that omitted IDs round-trip.
func (s Snowflake) MarshalJSON() ([]byte, error) {
	if s == 0 {
		return []byte("null"), nil
	}
	return strconv.AppendQuote(nil, s.String()), nil
}

// UnmarshalJSON accepts the string form Discord sends, and tolerates both null
// and a bare JSON number.
func (s *Snowflake) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*s = 0
		return nil
	}
	str := string(b)
	if len(str) >= 2 && str[0] == '"' {
		unquoted, err := strconv.Unquote(str)
		if err != nil {
			return err
		}
		str = unquoted
	}
	if str == "" {
		*s = 0
		return nil
	}
	n, err := strconv.ParseUint(str, 10, 64)
	if err != nil {
		return err
	}
	*s = Snowflake(n)
	return nil
}

// MarshalJSONTo is the encoding/json/v2 fast path: it writes straight into the
// encoder's buffer instead of allocating an intermediate []byte the way
// MarshalJSON must.
func (s Snowflake) MarshalJSONTo(enc *jsontext.Encoder) error {
	if s == 0 {
		return enc.WriteToken(jsontext.Null)
	}
	// 20 digits is the most a uint64 can need, plus two quotes.
	var buf [22]byte
	b := append(buf[:0], '"')
	b = strconv.AppendUint(b, uint64(s), 10)
	b = append(b, '"')
	return enc.WriteValue(b)
}

// UnmarshalJSONFrom is the encoding/json/v2 fast path. It reads one token
// rather than being handed a copied []byte, so decoding an ID allocates
// nothing.
func (s *Snowflake) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	tok, err := dec.ReadToken()
	if err != nil {
		return err
	}
	switch tok.Kind() {
	case 'n':
		*s = 0
	case '"':
		str := tok.String()
		if str == "" {
			*s = 0
			return nil
		}
		n, err := strconv.ParseUint(str, 10, 64)
		if err != nil {
			return err
		}
		*s = Snowflake(n)
	case '0':
		n, err := tok.Uint()
		if err != nil {
			return err
		}
		*s = Snowflake(n)
	default:
		return fmt.Errorf("starlings: cannot decode %s into a Snowflake", tok.Kind())
	}
	return nil
}

// MustID converts a Discord ID literal into a Snowflake, panicking if it is
// not a valid ID.
//
// It is meant for IDs written into the source - a channel you always post to,
// your own guild - where a typo is a bug to fix rather than a condition to
// handle:
//
//	var logChannel = starlings.MustID("123456789012345678")
//
// For an ID that arrives at runtime, from a config file or a command argument,
// use ParseSnowflake and check the error.
func MustID(s string) Snowflake {
	id, err := ParseSnowflake(s)
	if err != nil {
		panic("starlings: " + s + " is not a valid Discord ID: " + err.Error())
	}
	return id
}

// Value stores a snowflake in a database as its decimal text, which every SQL
// engine can hold without losing precision. Zero is stored as NULL.
func (s Snowflake) Value() (driver.Value, error) {
	if s == 0 {
		return nil, nil
	}
	return s.String(), nil
}

// Scan reads a snowflake stored as text, an integer, or NULL.
func (s *Snowflake) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*s = 0
	case string:
		return s.UnmarshalText([]byte(v))
	case []byte:
		return s.UnmarshalText(v)
	case int64:
		if v < 0 {
			return fmt.Errorf("starlings: negative snowflake %d", v)
		}
		*s = Snowflake(v)
	default:
		return fmt.Errorf("starlings: cannot scan %T into a Snowflake", src)
	}
	return nil
}

// MarshalText returns the decimal form, so snowflakes work as map keys in
// JSON and in text formats such as CSV.
func (s Snowflake) MarshalText() ([]byte, error) { return []byte(s.String()), nil }

// UnmarshalText parses the decimal form. Empty text is zero.
func (s *Snowflake) UnmarshalText(b []byte) error {
	if len(b) == 0 {
		*s = 0
		return nil
	}
	v, err := strconv.ParseUint(string(b), 10, 64)
	if err != nil {
		return fmt.Errorf("starlings: invalid snowflake %q", b)
	}
	*s = Snowflake(v)
	return nil
}
