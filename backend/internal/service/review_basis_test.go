package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"spectrum-interference-triangulation/backend/internal/constants"
	"spectrum-interference-triangulation/backend/internal/dto"
	"spectrum-interference-triangulation/backend/internal/localization"
	"spectrum-interference-triangulation/backend/internal/model"
	"spectrum-interference-triangulation/backend/internal/repository"
	"spectrum-interference-triangulation/backend/pkg/api"
)

// setupReviewBasisFixture 在 SQLite 中建立一个 analyzing 案例和一条历史定位结果。
func setupReviewBasisFixture(t *testing.T, db *gorm.DB, seq int) (caseID uint, estimateID uint, baseTime time.Time) {
	t.Helper()
	baseTime = time.Now().UTC().Add(-2 * time.Hour)
	c := model.InterferenceCase{
		CaseCode: fmt.Sprintf("RF-BASIS-%d", seq), Title: "复核依据门限测试案例", FrequencyCenterHz: 433920000,
		CaseStatus: constants.CaseAnalyzing, Priority: "normal", OpenedBy: 1, Version: 1,
	}
	if err := db.Create(&c).Error; err != nil {
		t.Fatalf("create case: %v", err)
	}
	e := model.LocalizationEstimate{
		CaseID: c.ID, AlgorithmVersion: localization.AlgorithmVersion,
		Latitude: 31.23, Longitude: 121.47, UncertaintyRadiusM: 300, ResidualDeg: 1.2,
		ConditionNumber: 4, GeometryDegenerate: false,
		UsedObservationIDsJSON: datatypes.JSON([]byte(`[1,2,3]`)),
		OutlierIDsJSON:         datatypes.JSON([]byte(`[]`)),
		ResidualsJSON:          datatypes.JSON([]byte(`[]`)),
		InputSnapshotJSON:      datatypes.JSON([]byte(`[]`)),
		EstimateStatus:         constants.EstimateComplete, CreatedBy: 2,
	}
	if err := db.Create(&e).Error; err != nil {
		t.Fatalf("create estimate: %v", err)
	}
	if err := db.Exec("UPDATE localization_estimates SET created_at = ? WHERE id = ?",
		baseTime.Format(time.RFC3339Nano), e.ID).Error; err != nil {
		t.Fatalf("set estimate created_at: %v", err)
	}
	return c.ID, e.ID, baseTime
}

func newEstimate(t *testing.T, db *gorm.DB, caseID uint, createdAt time.Time) model.LocalizationEstimate {
	t.Helper()
	e := model.LocalizationEstimate{
		CaseID: caseID, AlgorithmVersion: localization.AlgorithmVersion,
		Latitude: 31.24, Longitude: 121.48, UncertaintyRadiusM: 280, ResidualDeg: 0.9,
		ConditionNumber: 3.5, GeometryDegenerate: false,
		UsedObservationIDsJSON: datatypes.JSON([]byte(`[1,2,3]`)),
		OutlierIDsJSON:         datatypes.JSON([]byte(`[]`)),
		ResidualsJSON:          datatypes.JSON([]byte(`[]`)),
		InputSnapshotJSON:      datatypes.JSON([]byte(`[]`)),
		EstimateStatus:         constants.EstimateComplete, CreatedBy: 2,
	}
	if err := db.Create(&e).Error; err != nil {
		t.Fatalf("create estimate: %v", err)
	}
	if err := db.Exec("UPDATE localization_estimates SET created_at = ? WHERE id = ?",
		createdAt.Format(time.RFC3339Nano), e.ID).Error; err != nil {
		t.Fatalf("set estimate created_at: %v", err)
	}
	return e
}

func TestValidateReviewBasisForSubmission(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name      string
		configure func(t *testing.T, db *gorm.DB, caseID, estimateID uint, baseTime time.Time)
		wantCode  string
	}{
		{
			name:      "missing basis is rejected",
			configure: func(t *testing.T, db *gorm.DB, caseID, estimateID uint, baseTime time.Time) {},
			wantCode:  "REVIEW_BASIS_REQUIRED",
		},
		{
			name: "stale basis after observation change is rejected",
			configure: func(t *testing.T, db *gorm.DB, caseID, estimateID uint, baseTime time.Time) {
				if err := db.Model(&model.InterferenceCase{}).Where("id = ?", caseID).Updates(map[string]any{
					"review_basis_estimate_id":  estimateID,
					"review_basis_valid":        false,
					"review_basis_stale_reason": constants.ReviewBasisStaleObservationExcluded,
					"review_basis_selected_at":  baseTime.Add(30 * time.Minute),
					"review_basis_stale_at":     baseTime.Add(45 * time.Minute),
				}).Error; err != nil {
					t.Fatalf("stale basis fixture: %v", err)
				}
			},
			wantCode: "REVIEW_BASIS_STALE",
		},
		{
			name: "historical estimate older than stale time is rejected",
			configure: func(t *testing.T, db *gorm.DB, caseID, estimateID uint, baseTime time.Time) {
				staleAt := baseTime.Add(45 * time.Minute)
				if err := db.Model(&model.InterferenceCase{}).Where("id = ?", caseID).Updates(map[string]any{
					"review_basis_estimate_id": estimateID, "review_basis_valid": true,
					"review_basis_selected_at": staleAt.Add(time.Minute),
					"review_basis_stale_at":    staleAt,
				}).Error; err != nil {
					t.Fatalf("historical basis fixture: %v", err)
				}
			},
			wantCode: "REVIEW_BASIS_STALE",
		},
		{
			name: "rerun basis created after stale time is accepted",
			configure: func(t *testing.T, db *gorm.DB, caseID, estimateID uint, baseTime time.Time) {
				staleAt := baseTime.Add(45 * time.Minute)
				fresh := newEstimate(t, db, caseID, staleAt.Add(5*time.Minute))
				if err := db.Model(&model.InterferenceCase{}).Where("id = ?", caseID).Updates(map[string]any{
					"review_basis_estimate_id": fresh.ID, "review_basis_valid": true,
					"review_basis_selected_at": staleAt.Add(10 * time.Minute),
					"review_basis_stale_at":    staleAt,
				}).Error; err != nil {
					t.Fatalf("rerun basis fixture: %v", err)
				}
			},
			wantCode: "",
		},
		{
			name: "valid fresh basis without stale marker is accepted",
			configure: func(t *testing.T, db *gorm.DB, caseID, estimateID uint, baseTime time.Time) {
				if err := db.Model(&model.InterferenceCase{}).Where("id = ?", caseID).Updates(map[string]any{
					"review_basis_estimate_id": estimateID, "review_basis_valid": true,
					"review_basis_selected_at": baseTime.Add(10 * time.Minute),
				}).Error; err != nil {
					t.Fatalf("valid basis fixture: %v", err)
				}
			},
			wantCode: "",
		},
	}

	for index, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := newTestDB(t)
			caseService := NewCaseService(repository.NewCaseRepository(db), repository.NewEstimateRepository(db))
			caseID, estimateID, baseTime := setupReviewBasisFixture(t, db, index+1)
			tc.configure(t, db, caseID, estimateID, baseTime)
			current, err := caseService.Get(ctx, caseID)
			if err != nil {
				t.Fatalf("load case: %v", err)
			}
			err = caseService.validateReviewBasisForSubmission(ctx, current, 3)
			if tc.wantCode == "" {
				if err != nil {
					t.Fatalf("expected basis accepted, got %v", err)
				}
				return
			}
			assertErrorCode(t, err, tc.wantCode)
		})
	}
}

func TestSelectReviewBasisRequiresRerunAfterStale(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	caseService := NewCaseService(repository.NewCaseRepository(db), repository.NewEstimateRepository(db))
	actor := repository.Actor{UserID: 2, Email: "analyst@spectrum.local", Role: constants.RoleAnalyst, RequestID: "req-test"}
	caseID, estimateID, baseTime := setupReviewBasisFixture(t, db, 99)
	staleAt := baseTime.Add(45 * time.Minute)
	if err := db.Model(&model.InterferenceCase{}).Where("id = ?", caseID).Updates(map[string]any{
		"review_basis_estimate_id":  estimateID,
		"review_basis_valid":        false,
		"review_basis_stale_reason": constants.ReviewBasisStaleCaseRejected,
		"review_basis_selected_at":  baseTime.Add(30 * time.Minute),
		"review_basis_stale_at":     staleAt,
		"version":                   2,
	}).Error; err != nil {
		t.Fatalf("stale case fixture: %v", err)
	}

	// 退回后直接选老结果：拒绝。
	_, err := caseService.SelectReviewBasis(ctx, caseID, dto.SelectReviewBasisRequest{EstimateID: estimateID, Version: 2}, actor)
	assertErrorCode(t, err, "REVIEW_BASIS_STALE")

	// 重跑产生的新结果：允许选定，并保留上次依据供对照。
	fresh := newEstimate(t, db, caseID, staleAt.Add(5*time.Minute))
	updated, err := caseService.SelectReviewBasis(ctx, caseID, dto.SelectReviewBasisRequest{EstimateID: fresh.ID, Version: 2}, actor)
	if err != nil {
		t.Fatalf("expected rerun basis selection accepted, got %v", err)
	}
	if !updated.ReviewBasisValid || updated.ReviewBasisEstimateID == nil || *updated.ReviewBasisEstimateID != fresh.ID {
		t.Fatalf("unexpected updated basis: %+v", updated)
	}
	if updated.PreviousReviewBasisEstimateID == nil || *updated.PreviousReviewBasisEstimateID != estimateID {
		t.Fatalf("previous basis should be retained for comparison, got %+v", updated.PreviousReviewBasisEstimateID)
	}
}

func TestReviewBasisStaleReasons(t *testing.T) {
	for _, reason := range []string{
		constants.ReviewBasisStaleObservationAdded,
		constants.ReviewBasisStaleObservationExcluded,
		constants.ReviewBasisStaleCaseRejected,
	} {
		if !constants.ValidReviewBasisStaleReason(reason) {
			t.Fatalf("reason %s should be valid", reason)
		}
	}
	if constants.ValidReviewBasisStaleReason("unexpected") {
		t.Fatal("unexpected reason should be invalid")
	}
}

func assertErrorCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with code %s, got nil", code)
	}
	appErr, ok := err.(*api.Error)
	if !ok {
		t.Fatalf("expected *api.Error, got %T: %v", err, err)
	}
	if appErr.Code != code {
		t.Fatalf("expected error code %s, got %s", code, appErr.Code)
	}
}
