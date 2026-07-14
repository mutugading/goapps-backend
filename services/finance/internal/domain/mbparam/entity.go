// Package mbparam provides domain logic for MB (Master Batch) parameter master data.
package mbparam

// Parameter type constants.
const (
	TypeScalar   = "SCALAR"
	TypePicklist = "PICKLIST"
)

// Entity is a single MB parameter master-data row.
type Entity struct {
	id            string
	code          string
	name          string
	description   string
	paramType     string
	defaultValue  string
	defaultOption string
	unit          string
	displayOrder  int32
	isActive      bool
	options       []*Option
	createdAt     string
	createdBy     string
	updatedAt     string
	updatedBy     string
	deletedAt     string
	deletedBy     string
}

// Option is a single picklist option belonging to a PICKLIST-type parameter. mst_mb_param_option
// carries no audit-trail columns, so Option deliberately has none either.
type Option struct {
	id           string
	paramCode    string
	code         string
	numericValue string
	description  string
	displayOrder int32
	isActive     bool
}

// NewEntity constructs a new parameter row, validating code and createdBy are present and
// paramType is SCALAR or PICKLIST, and defaulting isActive to true.
//
//nolint:revive // Many parameters required for construction.
func NewEntity(code, name, paramType, description, defaultValue, defaultOption, unit string, displayOrder int32, createdBy string) (*Entity, error) {
	if code == "" {
		return nil, ErrCodeRequired
	}
	if paramType != TypeScalar && paramType != TypePicklist {
		return nil, ErrInvalidType
	}
	if createdBy == "" {
		return nil, ErrCreatedByRequired
	}
	return &Entity{
		code:          code,
		name:          name,
		paramType:     paramType,
		description:   description,
		defaultValue:  defaultValue,
		defaultOption: defaultOption,
		unit:          unit,
		displayOrder:  displayOrder,
		isActive:      true,
		createdBy:     createdBy,
	}, nil
}

// Reconstruct rebuilds an Entity from storage without re-running construction validation.
//
//nolint:revive // Many parameters required for hydration from storage.
func Reconstruct(id, code, name, description, paramType, defaultValue, defaultOption, unit string, displayOrder int32, isActive bool, createdAt, createdBy, updatedAt, updatedBy, deletedAt, deletedBy string) *Entity {
	return &Entity{
		id:            id,
		code:          code,
		name:          name,
		description:   description,
		paramType:     paramType,
		defaultValue:  defaultValue,
		defaultOption: defaultOption,
		unit:          unit,
		displayOrder:  displayOrder,
		isActive:      isActive,
		createdAt:     createdAt,
		createdBy:     createdBy,
		updatedAt:     updatedAt,
		updatedBy:     updatedBy,
		deletedAt:     deletedAt,
		deletedBy:     deletedBy,
	}
}

// ID returns the param's UUID.
func (e *Entity) ID() string { return e.id }

// Code returns the param's business code.
func (e *Entity) Code() string { return e.code }

// Name returns the param's display name.
func (e *Entity) Name() string { return e.name }

// Type returns SCALAR or PICKLIST.
func (e *Entity) Type() string { return e.paramType }

// Description returns the param's description.
func (e *Entity) Description() string { return e.description }

// DefaultValue returns the param's default numeric value (empty if unset).
func (e *Entity) DefaultValue() string { return e.defaultValue }

// DefaultOption returns the param's default picklist option code (empty if unset).
func (e *Entity) DefaultOption() string { return e.defaultOption }

// Unit returns the param's unit of measure.
func (e *Entity) Unit() string { return e.unit }

// DisplayOrder returns the param's display order.
func (e *Entity) DisplayOrder() int32 { return e.displayOrder }

// IsActive returns whether the param is active.
func (e *Entity) IsActive() bool { return e.isActive }

// Options returns the param's picklist options (empty for SCALAR params).
func (e *Entity) Options() []*Option { return e.options }

// SetOptions attaches the param's picklist options. Used by the repository layer to complete
// hydration after a batched eager-load query; not part of construction validation.
func (e *Entity) SetOptions(options []*Option) { e.options = options }

// CreatedAt returns the creation timestamp.
func (e *Entity) CreatedAt() string { return e.createdAt }

// CreatedBy returns the creator's identifier.
func (e *Entity) CreatedBy() string { return e.createdBy }

// UpdatedAt returns the last update timestamp.
func (e *Entity) UpdatedAt() string { return e.updatedAt }

// UpdatedBy returns the last updater's identifier.
func (e *Entity) UpdatedBy() string { return e.updatedBy }

// DeletedAt returns the soft-delete timestamp.
func (e *Entity) DeletedAt() string { return e.deletedAt }

// DeletedBy returns the soft-deleter's identifier.
func (e *Entity) DeletedBy() string { return e.deletedBy }

// IsDeleted returns whether the param row has been soft-deleted.
func (e *Entity) IsDeleted() bool { return e.deletedAt != "" }

// NewOption constructs a new picklist option, validating paramCode and code are present and
// defaulting isActive to true.
func NewOption(paramCode, code, numericValue, description string, displayOrder int32) (*Option, error) {
	if paramCode == "" {
		return nil, ErrParamCodeRequired
	}
	if code == "" {
		return nil, ErrOptionCodeRequired
	}
	return &Option{
		paramCode:    paramCode,
		code:         code,
		numericValue: numericValue,
		description:  description,
		displayOrder: displayOrder,
		isActive:     true,
	}, nil
}

// ReconstructOption rebuilds an Option from storage without re-running construction validation.
func ReconstructOption(id, paramCode, code, numericValue, description string, displayOrder int32, isActive bool) *Option {
	return &Option{
		id:           id,
		paramCode:    paramCode,
		code:         code,
		numericValue: numericValue,
		description:  description,
		displayOrder: displayOrder,
		isActive:     isActive,
	}
}

// ID returns the option's UUID.
func (o *Option) ID() string { return o.id }

// ParamCode returns the code of the parameter this option belongs to.
func (o *Option) ParamCode() string { return o.paramCode }

// Code returns the option's business code.
func (o *Option) Code() string { return o.code }

// NumericValue returns the option's numeric value.
func (o *Option) NumericValue() string { return o.numericValue }

// Description returns the option's description.
func (o *Option) Description() string { return o.description }

// DisplayOrder returns the option's display order.
func (o *Option) DisplayOrder() int32 { return o.displayOrder }

// IsActive returns whether the option is active.
func (o *Option) IsActive() bool { return o.isActive }
