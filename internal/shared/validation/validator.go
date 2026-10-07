package validation

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var uuid = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

var phonePattern = regexp.MustCompile(`^[+()0-9 -]{7,32}$`)

func Text(value string, max int) bool {
	if !utf8.ValidString(value) || strings.TrimSpace(value) == "" || utf8.RuneCountInString(value) > max {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func Phone(value string) bool {
	if !phonePattern.MatchString(value) {
		return false
	}
	n := 0
	for _, r := range value {
		if r >= '0' && r <= '9' {
			n++
		}
	}
	return n >= 7
}

func UUID(value string) bool { return uuid.MatchString(value) }
