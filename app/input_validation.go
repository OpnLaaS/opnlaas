package app

import (
	"errors"
	"fmt"
	"strings"

	goaway "github.com/TwiN/go-away"
)

type inputValidationError struct {
	message string
}

func (e *inputValidationError) Error() string {
	if e == nil {
		return "invalid input"
	}
	return e.message
}

func newInputValidationError(message string) error {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		trimmed = "invalid input"
	}
	return &inputValidationError{message: trimmed}
}

func inputValidationMessage(err error) (message string, ok bool) {
	var validationErr *inputValidationError
	if errors.As(err, &validationErr) {
		return validationErr.Error(), true
	}
	return "", false
}

var profanityDetector = goaway.NewProfanityDetector().
	WithExactWord(true).
	WithSanitizeLeetSpeak(true).
	WithSanitizeSpecialCharacters(true).
	WithSanitizeAccents(true)

func validateNoProfanity(field string, raw string) error {
	text := strings.TrimSpace(raw)
	if text == "" {
		return nil
	}
	if !profanityDetector.IsProfane(text) {
		return nil
	}

	field = strings.TrimSpace(field)
	if field == "" {
		field = "text"
	}
	return newInputValidationError(fmt.Sprintf("%s contains inappropriate language", field))
}
