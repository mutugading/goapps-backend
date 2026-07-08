package costproductrequest

import (
	"strings"
	"time"
)

// Allowed enum values.
const (
	RawMatPOYBoughtout = "POY_BOUGHTOUT"
	RawMatChipsSD      = "CHIPS_SD"
	RawMatChipsBRT     = "CHIPS_BRT"
	RawMatChipsRecycle = "CHIPS_RECYCLE"

	BoxTypeJumbo  = "JUMBO"
	BoxTypeNormal = "NORMAL"
	BoxTypePallet = "PALLET"

	TubeTypePaper   = "PAPER"
	TubeTypePlastic = "PLASTIC"
)

var (
	allowedRawMat   = map[string]struct{}{RawMatPOYBoughtout: {}, RawMatChipsSD: {}, RawMatChipsBRT: {}, RawMatChipsRecycle: {}}
	allowedBoxType  = map[string]struct{}{BoxTypeJumbo: {}, BoxTypeNormal: {}, BoxTypePallet: {}}
	allowedTubeType = map[string]struct{}{TubeTypePaper: {}, TubeTypePlastic: {}}
)

// Spec is the embedded product specification for a request with classification=new.
type Spec struct {
	SpecID             int64
	RawMaterialType    string
	ProductDescription string
	ShadeID            *int32
	ShadeCode          string
	ShadeName          string
	// PaperTubeTypeID is deprecated for new writes (product-request-workflow-revamp
	// D3) — retained for historical rows / legacy display only. New rows use
	// TubeType instead. Nil means not collected (matches ShadeID's nullability).
	PaperTubeTypeID   *int32
	TubeType          string // "PAPER" / "PLASTIC" / empty (not collected)
	WeightPerBobbinKg string // decimal stringified
	BoxType           string
	CreatedAt         time.Time
	CreatedBy         string
}

// SpecInput is the user-facing shape passed to create/update.
type SpecInput struct {
	RawMaterialType    string
	ProductDescription string
	ShadeID            int32 // 0 = unset
	ShadeCode          string
	ShadeName          string
	// PaperTubeTypeID is deprecated for new writes (product-request-workflow-revamp
	// D3) — optional, retained only for legacy display of historical rows.
	PaperTubeTypeID   int32
	TubeType          string // "PAPER" / "PLASTIC" / empty (not collected)
	WeightPerBobbinKg string
	BoxType           string
}

// Validate enforces the FR-1 spec rules + DB CHECK constraints (chk_cps_*).
//
// RawMaterialType, BoxType, and WeightPerBobbinKg are optional (migration
// 000435 relaxed their DB constraints to "NULL OR valid") — an empty value is
// accepted, but a non-empty RawMaterialType/BoxType must still match the
// allowed enum set.
//
// TubeType (migration 000437) replaces PaperTubeTypeID for new writes: it is
// optional, but a non-empty value must be "PAPER" or "PLASTIC". PaperTubeTypeID
// is no longer required — it is retained only for legacy display of rows
// created before this change.
func (in SpecInput) Validate() error {
	if in.RawMaterialType != "" {
		if _, ok := allowedRawMat[in.RawMaterialType]; !ok {
			return ErrInvalidSpec
		}
	}
	if strings.TrimSpace(in.ProductDescription) == "" {
		return ErrInvalidSpec
	}
	if in.TubeType != "" {
		if _, ok := allowedTubeType[in.TubeType]; !ok {
			return ErrInvalidSpec
		}
	}
	if in.BoxType != "" {
		if _, ok := allowedBoxType[in.BoxType]; !ok {
			return ErrInvalidSpec
		}
	}
	// shade_id OR shade_code must be present.
	if in.ShadeID <= 0 && strings.TrimSpace(in.ShadeCode) == "" {
		return ErrInvalidSpec
	}
	return nil
}

// ToSpec materializes a Spec (sans SpecID / CreatedAt — set by repo).
func (in SpecInput) ToSpec(actor string) Spec {
	s := Spec{
		RawMaterialType:    in.RawMaterialType,
		ProductDescription: strings.TrimSpace(in.ProductDescription),
		ShadeCode:          strings.TrimSpace(in.ShadeCode),
		ShadeName:          strings.TrimSpace(in.ShadeName),
		TubeType:           in.TubeType,
		WeightPerBobbinKg:  strings.TrimSpace(in.WeightPerBobbinKg),
		BoxType:            in.BoxType,
		CreatedAt:          time.Now().UTC(),
		CreatedBy:          actor,
	}
	if in.ShadeID > 0 {
		v := in.ShadeID
		s.ShadeID = &v
	}
	if in.PaperTubeTypeID > 0 {
		v := in.PaperTubeTypeID
		s.PaperTubeTypeID = &v
	}
	return s
}
