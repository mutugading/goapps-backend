// Package product contains the Product aggregate and its supporting types.
package product

import "errors"

// Sentinel errors returned by the product domain.
var (
	// ErrNotFound is returned when a product is not found.
	ErrNotFound = errors.New("product not found")

	// ErrAlreadyExists is returned when a product with this code or item code already exists.
	ErrAlreadyExists = errors.New("product with this code or item code already exists")

	// ErrInvalidCode is returned when the product code is invalid.
	ErrInvalidCode = errors.New("invalid product code")

	// ErrInvalidName is returned when the product name is invalid.
	ErrInvalidName = errors.New("invalid product name")

	// ErrInvalidItemCode is returned when the product item code is invalid.
	ErrInvalidItemCode = errors.New("invalid product item code")

	// ErrInvalidShadeCode is returned when the product shade code is invalid.
	ErrInvalidShadeCode = errors.New("invalid product shade code")

	// ErrInvalidShadeName is returned when the product shade name is invalid.
	ErrInvalidShadeName = errors.New("invalid product shade name")

	// ErrInvalidProductStatus is returned when the product status value is invalid.
	ErrInvalidProductStatus = errors.New("invalid product status")

	// ErrInvalidWorkflowStatus is returned when the workflow status value is invalid.
	ErrInvalidWorkflowStatus = errors.New("invalid workflow status")

	// ErrInvalidPurpose is returned when the purpose value is invalid.
	ErrInvalidPurpose = errors.New("invalid purpose")

	// ErrInvalidDeptID is returned when the department ID is invalid.
	ErrInvalidDeptID = errors.New("invalid department id")

	// ErrLocked is returned when the product is locked and cannot be modified.
	ErrLocked = errors.New("product is locked and cannot be modified")

	// ErrSourceDeleted is returned when attempting to duplicate from a deleted product.
	ErrSourceDeleted = errors.New("cannot duplicate from a deleted product")

	// ErrSelfDuplication is returned when the source product id matches the new product code.
	ErrSelfDuplication = errors.New("source product id must differ from new product")

	// ErrInvalidDuplicationNote is returned when the duplication note exceeds the maximum length.
	ErrInvalidDuplicationNote = errors.New("duplication note must not exceed 500 characters")
)
