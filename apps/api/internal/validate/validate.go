// Package validate collects per-field input problems so a request can report
// everything that is wrong at once instead of one error per round trip.
package validate

import (
	"fmt"
	"net/mail"
	"strings"
	"unicode/utf8"

	"github.com/dark/x-mail/api/internal/httpx"
)

type Validator struct {
	Errors map[string]string
}

func New() *Validator { return &Validator{Errors: map[string]string{}} }

// Check records message against field when ok is false. The first problem
// recorded for a field wins, so the most specific check should run first.
func (v *Validator) Check(ok bool, field, message string) {
	if ok {
		return
	}
	if _, exists := v.Errors[field]; !exists {
		v.Errors[field] = message
	}
}

func (v *Validator) Required(field, value string) {
	v.Check(strings.TrimSpace(value) != "", field, "This field is required.")
}

func (v *Validator) MinLen(field, value string, n int) {
	v.Check(utf8.RuneCountInString(value) >= n, field, fmt.Sprintf("Must be at least %d characters.", n))
}

func (v *Validator) MaxLen(field, value string, n int) {
	v.Check(utf8.RuneCountInString(value) <= n, field, fmt.Sprintf("Must be at most %d characters.", n))
}

// Email checks the address parses and looks like a single plain address.
func (v *Validator) Email(field, value string) {
	addr, err := mail.ParseAddress(strings.TrimSpace(value))
	v.Check(err == nil && addr != nil && strings.Contains(addr.Address, "@"), field, "Enter a valid email address.")
}

func (v *Validator) In(field, value string, allowed ...string) {
	for _, a := range allowed {
		if value == a {
			return
		}
	}
	v.Check(false, field, fmt.Sprintf("Must be one of: %s.", strings.Join(allowed, ", ")))
}

func (v *Validator) Valid() bool { return len(v.Errors) == 0 }

// Err returns a 422 carrying every field message, or nil when input is valid.
func (v *Validator) Err() error {
	if v.Valid() {
		return nil
	}
	return httpx.UnprocessableEntity("Please correct the highlighted fields.").WithFields(v.Errors)
}
