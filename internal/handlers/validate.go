package handlers

import (
	"regexp"
	"strings"
)

var (
	emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	phoneRegex = regexp.MustCompile(`^[+]?[\d\s\-().]{7,20}$`)
)

func validateEmail(email string) bool {
	if email == "" {
		return true // email is optional
	}
	return emailRegex.MatchString(email)
}

func validatePhone(phone string) bool {
	if phone == "" {
		return true // phone is optional
	}
	return phoneRegex.MatchString(phone)
}

func validateNotes(notes string) bool {
	return len(notes) <= 1000
}

func sanitizeLogInput(s string) string {
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\r", "")
	return s
}
