package app

import (
	"encoding/json"
	"fmt"
	"io"
)

type response struct {
	OK      bool           `json:"ok"`
	Command string         `json:"command,omitempty"`
	Data    map[string]any `json:"data,omitempty"`
	Error   *responseError `json:"error,omitempty"`
}

type responseError struct {
	Code    string              `json:"code"`
	Message string              `json:"message"`
	Details []map[string]string `json:"details,omitempty"`
}

func (o output) validationFailure(message string, details domainValidationErrors) int {
	if !o.json {
		fmt.Fprintf(o.stderr, "hermoso: %s\n", message)
		return ExitFailure
	}
	if err := json.NewEncoder(o.stdout).Encode(response{
		OK: false,
		Error: &responseError{
			Code:    ErrorValidation,
			Message: message,
			Details: details.ValidationDetails(),
		},
	}); err != nil {
		fmt.Fprintf(o.stderr, "hermoso: encode error response: %v\n", err)
		return ExitFailure
	}
	return ExitFailure
}

type output struct {
	json   bool
	stdout io.Writer
	stderr io.Writer
}

func (o output) success(command string, data map[string]any, text string) int {
	if o.json {
		if err := json.NewEncoder(o.stdout).Encode(response{
			OK:      true,
			Command: command,
			Data:    data,
		}); err != nil {
			fmt.Fprintf(o.stderr, "hermoso: encode response: %v\n", err)
			return ExitFailure
		}
		return ExitOK
	}

	if _, err := io.WriteString(o.stdout, text); err != nil {
		fmt.Fprintf(o.stderr, "hermoso: write response: %v\n", err)
		return ExitFailure
	}
	return ExitOK
}

func (o output) usageError(message string) int {
	if o.json {
		return o.failure(ExitUsage, ErrorUsage, message)
	}
	fmt.Fprintf(o.stderr, "hermoso: %s\n\n%s", message, usage)
	return ExitUsage
}

func (o output) failure(exitCode int, code, message string) int {
	if o.json {
		if err := json.NewEncoder(o.stdout).Encode(response{
			OK: false,
			Error: &responseError{
				Code:    code,
				Message: message,
			},
		}); err != nil {
			fmt.Fprintf(o.stderr, "hermoso: encode error response: %v\n", err)
			return ExitFailure
		}
		return exitCode
	}

	fmt.Fprintf(o.stderr, "hermoso: %s\n", message)
	return exitCode
}
