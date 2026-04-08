// Package formula provides domain logic for Formula management.
package formula

import (
	"regexp"
	"strings"
)

// =============================================================================
// Code Value Object
// =============================================================================

var codePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// Code represents a formula code value object.
type Code struct {
	value string
}

// NewCode creates a new validated Code.
func NewCode(s string) (Code, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Code{}, ErrEmptyCode
	}
	if len(s) > 50 {
		return Code{}, ErrCodeTooLong
	}
	if !codePattern.MatchString(s) {
		return Code{}, ErrInvalidCodeFormat
	}
	return Code{value: s}, nil
}

// String returns the code string.
func (c Code) String() string { return c.value }

// =============================================================================
// FormulaType Value Object
// =============================================================================

// FormulaType represents the type of a formula.
type FormulaType struct {
	value string
}

// Valid formula types.
var (
	FormulaTypeCalculation = FormulaType{value: "CALCULATION"}
	FormulaTypeSQLQuery    = FormulaType{value: "SQL_QUERY"}
	FormulaTypeConstant    = FormulaType{value: "CONSTANT"}
)

// NewFormulaType creates a FormulaType from a string.
func NewFormulaType(s string) (FormulaType, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	switch s {
	case "CALCULATION":
		return FormulaTypeCalculation, nil
	case "SQL_QUERY":
		return FormulaTypeSQLQuery, nil
	case "CONSTANT":
		return FormulaTypeConstant, nil
	default:
		return FormulaType{}, ErrInvalidFormulaType
	}
}

// String returns the formula type string.
func (f FormulaType) String() string { return f.value }

// IsValid returns true if the formula type is valid.
func (f FormulaType) IsValid() bool {
	return f.value == "CALCULATION" || f.value == "SQL_QUERY" || f.value == "CONSTANT"
}
