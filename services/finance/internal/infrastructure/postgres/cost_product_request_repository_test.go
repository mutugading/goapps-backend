// Package postgres_test provides integration tests for CostProductRequestRepository,
// covering the D1 nullable-field round-trip (product-request-workflow-revamp P2-T3):
// cps_raw_material_type/cps_box_type/cps_weight_per_bobbin_kg must map empty
// string <-> SQL NULL cleanly on both INSERT and SELECT.
package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductrequest"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

const cprTestCustomerPrefix = "ZZTCPR"

// CostProductRequestRepoSuite exercises Create/GetByID/Save round-trips
// against a real DB, focused on the D1 nullable-spec-field mapping.
type CostProductRequestRepoSuite struct {
	suite.Suite
	db            *postgres.DB
	repo          *postgres.CostProductRequestRepository
	ctx           context.Context
	requestTypeID int32
	tubeTypeID    int32
}

func TestCostProductRequestRepoSuite(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	suite.Run(t, new(CostProductRequestRepoSuite))
}

func (s *CostProductRequestRepoSuite) SetupSuite() {
	s.ctx = context.Background()

	host := getEnvOrDefault("TEST_DB_HOST", "localhost")
	port := getEnvOrDefault("TEST_DB_PORT", "5434")
	user := getEnvOrDefault("TEST_DB_USER", "finance")
	password := getEnvOrDefault("TEST_DB_PASSWORD", "finance123")
	dbname := getEnvOrDefault("TEST_DB_NAME", "finance_db")

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	raw, err := sql.Open("pgx", dsn)
	require.NoError(s.T(), err)
	require.NoError(s.T(), waitForDB(raw, 10*time.Second))

	s.db = postgres.NewDBFromSQL(raw)
	s.repo = postgres.NewCostProductRequestRepository(s.db)

	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT crt_type_id FROM cost_request_type ORDER BY crt_type_id LIMIT 1`).Scan(&s.requestTypeID))
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT cptt_paper_tube_type_id FROM cost_paper_tube_type ORDER BY cptt_paper_tube_type_id LIMIT 1`).Scan(&s.tubeTypeID))
}

func (s *CostProductRequestRepoSuite) TearDownSuite() {
	if s.db == nil {
		return
	}
	_, err := s.db.ExecContext(s.ctx,
		`DELETE FROM cost_product_request WHERE cpr_customer_code LIKE $1`, cprTestCustomerPrefix+"%")
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.db.Close())
}

// TestCreateAndGet_EmptyD1Fields_RoundTripAsEmptyString verifies that
// submitting a spec with raw material type, box type, and weight per bobbin
// all empty (D1's removed-from-form fields) round-trips through INSERT (as
// SQL NULL, satisfying migration 000435's relaxed NOT NULL) and back out as
// empty Go strings, matching the pre-D1 "unset" representation the domain
// layer expects.
func (s *CostProductRequestRepoSuite) TestCreateAndGet_EmptyD1Fields_RoundTripAsEmptyString() {
	t := s.T()

	req, err := costproductrequest.New(costproductrequest.NewInput{
		RequestTypeID:         s.requestTypeID,
		Title:                 "D1 nullable fields round-trip",
		CustomerName:          "ZZT Customer",
		CustomerCode:          cprTestCustomerPrefix + "-001",
		ProductClassification: costproductrequest.ClassNew,
		RequesterUserID:       "itest",
		Spec: &costproductrequest.SpecInput{
			// D1: these 3 fields deliberately left empty.
			RawMaterialType:    "",
			BoxType:            "",
			WeightPerBobbinKg:  "",
			ProductDescription: "PET preform, natural",
			ShadeCode:          "Natural",
			PaperTubeTypeID:    s.tubeTypeID,
		},
	})
	require.NoError(t, err)

	require.NoError(t, s.repo.Create(s.ctx, req))
	require.Positive(t, req.RequestID())

	got, err := s.repo.GetByID(s.ctx, req.RequestID())
	require.NoError(t, err)
	require.NotNil(t, got.Spec())

	spec := got.Spec()
	require.Empty(t, spec.RawMaterialType, "cps_raw_material_type should round-trip as empty string")
	require.Empty(t, spec.BoxType, "cps_box_type should round-trip as empty string")
	require.Empty(t, spec.WeightPerBobbinKg, "cps_weight_per_bobbin_kg should round-trip as empty string")
	require.Equal(t, "PET preform, natural", spec.ProductDescription)
	require.Equal(t, "Natural", spec.ShadeCode)

	// Verify the underlying columns are actually SQL NULL, not empty-string sentinels.
	var rawMat, boxType, weight sql.NullString
	require.NoError(t, s.db.QueryRowContext(s.ctx,
		`SELECT cps_raw_material_type, cps_box_type, cps_weight_per_bobbin_kg::text FROM cost_product_spec WHERE cps_request_id=$1`,
		req.RequestID()).Scan(&rawMat, &boxType, &weight))
	require.False(t, rawMat.Valid, "cps_raw_material_type should be SQL NULL")
	require.False(t, boxType.Valid, "cps_box_type should be SQL NULL")
	require.False(t, weight.Valid, "cps_weight_per_bobbin_kg should be SQL NULL")
}

// TestCreateAndGet_TubeType_RoundTrips verifies that D3's cps_tube_type
// column round-trips a non-empty value ("PAPER"/"PLASTIC") through Create+Get,
// and that an empty TubeType round-trips as empty string (SQL NULL), not some
// sentinel value.
func (s *CostProductRequestRepoSuite) TestCreateAndGet_TubeType_RoundTrips() {
	t := s.T()

	cases := []struct {
		name     string
		tubeType string
	}{
		{name: "paper", tubeType: costproductrequest.TubeTypePaper},
		{name: "plastic", tubeType: costproductrequest.TubeTypePlastic},
		{name: "empty", tubeType: ""},
	}

	for i, tc := range cases {
		req, err := costproductrequest.New(costproductrequest.NewInput{
			RequestTypeID:         s.requestTypeID,
			Title:                 "D3 tube type round-trip " + tc.name,
			CustomerName:          "ZZT Customer",
			CustomerCode:          fmt.Sprintf("%s-TT-%03d", cprTestCustomerPrefix, i),
			ProductClassification: costproductrequest.ClassNew,
			RequesterUserID:       "itest",
			Spec: &costproductrequest.SpecInput{
				ProductDescription: "Tube type round-trip test",
				ShadeCode:          "Natural",
				TubeType:           tc.tubeType,
			},
		})
		require.NoError(t, err)
		require.NoError(t, s.repo.Create(s.ctx, req))

		got, err := s.repo.GetByID(s.ctx, req.RequestID())
		require.NoError(t, err)
		require.NotNil(t, got.Spec())
		require.Equal(t, tc.tubeType, got.Spec().TubeType, "cps_tube_type should round-trip exactly")

		var tubeType sql.NullString
		require.NoError(t, s.db.QueryRowContext(s.ctx,
			`SELECT cps_tube_type FROM cost_product_spec WHERE cps_request_id=$1`,
			req.RequestID()).Scan(&tubeType))
		if tc.tubeType == "" {
			require.False(t, tubeType.Valid, "cps_tube_type should be SQL NULL when unset")
		} else {
			require.True(t, tubeType.Valid)
			require.Equal(t, tc.tubeType, tubeType.String)
		}
	}
}

// TestSave_EmptyD1Fields_RoundTripAsEmptyString verifies the same NULL
// mapping on the Save (replace-spec) path used for updates.
func (s *CostProductRequestRepoSuite) TestSave_EmptyD1Fields_RoundTripAsEmptyString() {
	t := s.T()

	req, err := costproductrequest.New(costproductrequest.NewInput{
		RequestTypeID:         s.requestTypeID,
		Title:                 "D1 nullable fields save round-trip",
		CustomerName:          "ZZT Customer",
		CustomerCode:          cprTestCustomerPrefix + "-002",
		ProductClassification: costproductrequest.ClassNew,
		RequesterUserID:       "itest",
		Spec: &costproductrequest.SpecInput{
			RawMaterialType:    costproductrequest.RawMatPOYBoughtout,
			BoxType:            costproductrequest.BoxTypeJumbo,
			WeightPerBobbinKg:  "1.500",
			ProductDescription: "Initial spec, fully populated",
			ShadeCode:          "Natural",
			PaperTubeTypeID:    s.tubeTypeID,
		},
	})
	require.NoError(t, err)
	require.NoError(t, s.repo.Create(s.ctx, req))

	// Reload, then re-save with a spec that empties the 3 D1 fields.
	loaded, err := s.repo.GetByID(s.ctx, req.RequestID())
	require.NoError(t, err)

	require.NoError(t, loaded.Update(costproductrequest.UpdateInput{
		Title:                 loaded.Title(),
		CustomerName:          loaded.CustomerName(),
		CustomerCode:          loaded.CustomerCode(),
		ProductClassification: loaded.ProductClassification(),
		Spec: &costproductrequest.SpecInput{
			RawMaterialType:    "",
			BoxType:            "",
			WeightPerBobbinKg:  "",
			ProductDescription: "Updated spec, D1 fields cleared",
			ShadeCode:          "Natural",
			PaperTubeTypeID:    s.tubeTypeID,
		},
	}))
	require.NoError(t, s.repo.Save(s.ctx, loaded))

	got, err := s.repo.GetByID(s.ctx, req.RequestID())
	require.NoError(t, err)
	require.NotNil(t, got.Spec())
	require.Empty(t, got.Spec().RawMaterialType)
	require.Empty(t, got.Spec().BoxType)
	require.Empty(t, got.Spec().WeightPerBobbinKg)
	require.Equal(t, "Updated spec, D1 fields cleared", got.Spec().ProductDescription)
}
