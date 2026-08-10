// Package contracts exposes deterministic JSON schemas and validates authored
// contract files before they enter durable run state.
package contracts

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/bocacorazon/hermoso/internal/domain"
)

type Kind string

const (
	FeatureDesign               Kind = "feature-design"
	FeatureVerificationContract Kind = "feature-verification-contract"
	WorkGraph                   Kind = "work-graph"
	PhaseResult                 Kind = "phase-result"
)

var ErrUnknownKind = errors.New("unknown contract kind")

func ParseKind(value string) (Kind, error) {
	kind := Kind(value)
	switch kind {
	case FeatureDesign, FeatureVerificationContract, WorkGraph, PhaseResult:
		return kind, nil
	default:
		return "", fmt.Errorf("%w %q", ErrUnknownKind, value)
	}
}

func Schema(kind Kind) ([]byte, error) {
	var schema map[string]any
	switch kind {
	case FeatureDesign:
		schema = featureDesignSchema()
	case FeatureVerificationContract:
		schema = featureVerificationContractSchema()
	case WorkGraph:
		schema = workGraphSchema()
	case PhaseResult:
		schema = phaseResultSchema()
	default:
		return nil, fmt.Errorf("%w %q", ErrUnknownKind, kind)
	}
	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode %s schema: %w", kind, err)
	}
	return append(data, '\n'), nil
}

func Validate(kind Kind, data []byte) error {
	_, err := decode(kind, data)
	return err
}

func ValidateForContext(kind Kind, data []byte, expected domain.ContextRef) error {
	contract, err := decode(kind, data)
	if err != nil {
		return err
	}
	var actual domain.ContextRef
	switch value := contract.(type) {
	case *domain.FeatureDesign:
		actual = value.Context
	case *domain.FeatureVerificationContract:
		actual = value.Context
	case *domain.WorkGraph:
		actual = value.Context
	case *domain.PhaseResult:
		actual = value.Context
	}
	if !actual.Equal(expected) {
		return fmt.Errorf("validate %s: context does not match persisted project/run state", kind)
	}
	return nil
}

func decode(kind Kind, data []byte) (interface{ Validate() error }, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var contract interface{ Validate() error }
	switch kind {
	case FeatureDesign:
		contract = &domain.FeatureDesign{}
	case FeatureVerificationContract:
		contract = &domain.FeatureVerificationContract{}
	case WorkGraph:
		contract = &domain.WorkGraph{}
	case PhaseResult:
		contract = &domain.PhaseResult{}
	default:
		return nil, fmt.Errorf("%w %q", ErrUnknownKind, kind)
	}
	if err := decoder.Decode(contract); err != nil {
		return nil, fmt.Errorf("decode %s: %w", kind, err)
	}
	if err := ensureEOF(decoder); err != nil {
		return nil, fmt.Errorf("decode %s: %w", kind, err)
	}
	if err := contract.Validate(); err != nil {
		return nil, fmt.Errorf("validate %s: %w", kind, err)
	}
	return contract, nil
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}
