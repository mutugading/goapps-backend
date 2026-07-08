package costproductrequest

import (
	"errors"
	"testing"
)

func TestSpecInput_Validate(t *testing.T) {
	validShadeID := int32(1)

	base := func() SpecInput {
		return SpecInput{
			RawMaterialType:    RawMatPOYBoughtout,
			ProductDescription: "PET bottle grade resin",
			ShadeID:            validShadeID,
			PaperTubeTypeID:    1,
			WeightPerBobbinKg:  "1.500",
			BoxType:            BoxTypeJumbo,
		}
	}

	tests := []struct {
		name    string
		mutate  func(in *SpecInput)
		wantErr error
	}{
		{
			name:    "valid full input",
			mutate:  func(_ *SpecInput) {},
			wantErr: nil,
		},
		{
			name: "empty raw material type now valid (D1)",
			mutate: func(in *SpecInput) {
				in.RawMaterialType = ""
			},
			wantErr: nil,
		},
		{
			name: "empty box type now valid (D1)",
			mutate: func(in *SpecInput) {
				in.BoxType = ""
			},
			wantErr: nil,
		},
		{
			name: "empty weight per bobbin now valid (D1)",
			mutate: func(in *SpecInput) {
				in.WeightPerBobbinKg = ""
			},
			wantErr: nil,
		},
		{
			name: "all three D1 fields empty simultaneously now valid",
			mutate: func(in *SpecInput) {
				in.RawMaterialType = ""
				in.BoxType = ""
				in.WeightPerBobbinKg = ""
			},
			wantErr: nil,
		},
		{
			name: "non-empty invalid raw material type still rejected",
			mutate: func(in *SpecInput) {
				in.RawMaterialType = "BOGUS"
			},
			wantErr: ErrInvalidSpec,
		},
		{
			name: "non-empty invalid box type still rejected",
			mutate: func(in *SpecInput) {
				in.BoxType = "BOGUS"
			},
			wantErr: ErrInvalidSpec,
		},
		{
			name: "empty product description still rejected",
			mutate: func(in *SpecInput) {
				in.ProductDescription = "   "
			},
			wantErr: ErrInvalidSpec,
		},
		{
			name: "missing paper tube type id now valid (D3, legacy-only field)",
			mutate: func(in *SpecInput) {
				in.PaperTubeTypeID = 0
			},
			wantErr: nil,
		},
		{
			name: "old-row-read path: PaperTubeTypeID populated, TubeType empty, still valid",
			mutate: func(in *SpecInput) {
				in.PaperTubeTypeID = 1
				in.TubeType = ""
			},
			wantErr: nil,
		},
		{
			name: "new-row-write path: TubeType PAPER populated, PaperTubeTypeID unset",
			mutate: func(in *SpecInput) {
				in.PaperTubeTypeID = 0
				in.TubeType = TubeTypePaper
			},
			wantErr: nil,
		},
		{
			name: "new-row-write path: TubeType PLASTIC populated, PaperTubeTypeID unset",
			mutate: func(in *SpecInput) {
				in.PaperTubeTypeID = 0
				in.TubeType = TubeTypePlastic
			},
			wantErr: nil,
		},
		{
			name: "non-empty invalid tube type rejected",
			mutate: func(in *SpecInput) {
				in.TubeType = "BOGUS"
			},
			wantErr: ErrInvalidSpec,
		},
		{
			name: "shade id and shade custom text both absent still rejected",
			mutate: func(in *SpecInput) {
				in.ShadeID = 0
				in.ShadeCode = ""
			},
			wantErr: ErrInvalidSpec,
		},
		{
			name: "shade custom text alone satisfies shade presence",
			mutate: func(in *SpecInput) {
				in.ShadeID = 0
				in.ShadeCode = "Natural White"
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := base()
			tt.mutate(&in)

			err := in.Validate()

			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("Validate() error = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Validate() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
