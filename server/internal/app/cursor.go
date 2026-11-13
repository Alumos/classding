package app

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"strings"
)

// pageKey orders records newest first, with the ID breaking ties so that
// records created in the same instant are never skipped or repeated.
type pageKey struct {
	CreatedAt string `json:"created_at"`
	ID        string `json:"id"`
}

// newestFirst compares parsed times, not RFC 3339 strings: a whole second and
// a fractional second do not sort lexically at the boundary.
func newestFirst(x, y pageKey) int {
	left, _ := parseTime(x.CreatedAt)
	right, _ := parseTime(y.CreatedAt)
	if order := right.Compare(left); order != 0 {
		return order
	}
	return strings.Compare(y.ID, x.ID)
}

func (k pageKey) encode() *string {
	data, _ := json.Marshal(k)
	encoded := base64.RawURLEncoding.EncodeToString(data)
	return &encoded
}

// decodeCursor reads an opaque page cursor issued by pageKey.encode.
func decodeCursor(raw string) (*pageKey, bool) {
	if raw == "" {
		return nil, true
	}
	if len(raw) > 1024 {
		return nil, false
	}
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, false
	}
	var key pageKey
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&key) != nil || key.ID == "" || len(key.ID) > 128 {
		return nil, false
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, false
	}
	if _, valid := parseTime(key.CreatedAt); !valid {
		return nil, false
	}
	return &key, true
}

// after reports whether the record belongs on a page that follows cursor.
func (k pageKey) after(cursor *pageKey) bool { return cursor == nil || newestFirst(k, *cursor) > 0 }
