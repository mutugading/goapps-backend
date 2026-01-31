// Package uom provides domain logic for Unit of Measure management.
package uom

import (
	"regexp"
	"strings"
)

// =============================================================================
// Code Value Object
// =============================================================================

// Code represents a validated UOM code value object.
type Code struct {
	value string
}

// codePattern defines the valid format for UOM codes.
// Must start with uppercase letter, followed by uppercase letters, digits, or underscores.
var codePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// NewCode creates a new validated Code value object.
func NewCode(code string) (Code, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return Code{}, ErrEmptyCode
	}
	if len(code) > 20 {
		return Code{}, ErrCodeTooLong
	}
	if !codePattern.MatchString(code) {
		return Code{}, ErrInvalidCodeFormat
	}
	return Code{value: code}, nil
}

// String returns the string representation of the code.
func (c Code) String() string {
	return c.value
}

// IsEmpty returns true if the code is empty.
func (c Code) IsEmpty() bool {
	return c.value == ""
}

// Equals checks if two codes are equal.
func (c Code) Equals(other Code) bool {
	return c.value == other.value
}

// =============================================================================
// Category Value Object
// =============================================================================

// Category represents a validated UOM category value object.
type Category string

// Category constants.
const (
	CategoryWeight   Category = "WEIGHT"
	CategoryLength   Category = "LENGTH"
	CategoryVolume   Category = "VOLUME"
	CategoryQuantity Category = "QUANTITY"
)

// validCategories is a set of valid category values.
var validCategories = map[Category]bool{
	CategoryWeight:   true,
	CategoryLength:   true,
	CategoryVolume:   true,
	CategoryQuantity: true,
}

// NewCategory creates a new validated Category value object.
func NewCategory(category string) (Category, error) {
	cat := Category(strings.ToUpper(strings.TrimSpace(category)))
	if !validCategories[cat] {
		return "", ErrInvalidCategory
	}
	return cat, nil
}

// String returns the string representation of the category.
func (c Category) String() string {
	return string(c)
}

// IsValid returns true if the category is valid.
func (c Category) IsValid() bool {
	return validCategories[c]
}

// AllCategories returns a slice of all valid categories.
func AllCategories() []Category {
	return []Category{CategoryWeight, CategoryLength, CategoryVolume, CategoryQuantity}
}
