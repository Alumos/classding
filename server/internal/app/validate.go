package app

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Field limits shared by every endpoint that accepts them.
const (
	maxDisplayName = 24
	maxClassName   = 40
	maxDeviceName  = 60
	maxMessageText = 200
	maxPhraseText  = 200
	maxPhrases     = 30
	maxTeachers    = 100
	maxDevices     = 20
)

// runesBetween reports whether s holds between min and max characters.
func runesBetween(s string, min, max int) bool {
	n := utf8.RuneCountInString(s)
	return n >= min && n <= max
}

func validDisplayName(name string) bool { return runesBetween(name, 1, maxDisplayName) }

// validUsername accepts 3–32 lowercase letters, digits, dots, underscores and hyphens.
func validUsername(username string) bool {
	if len(username) < 3 || len(username) > 32 {
		return false
	}
	for _, ch := range username {
		if !(ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || ch == '_' || ch == '-' || ch == '.') {
			return false
		}
	}
	return true
}

// validPassword requires 10–128 bytes with at least one letter and one digit.
func validPassword(s string) bool {
	return len(s) >= 10 && len(s) <= 128 &&
		strings.IndexFunc(s, unicode.IsLetter) >= 0 &&
		strings.IndexFunc(s, unicode.IsDigit) >= 0
}

func normalizeUsername(username string) string { return strings.ToLower(strings.TrimSpace(username)) }

func inRange(n, min, max int) bool { return n >= min && n <= max }
