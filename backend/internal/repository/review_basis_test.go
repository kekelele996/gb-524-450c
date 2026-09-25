package repository

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"spectrum-interference-triangulation/backend/internal/constants"
	"spectrum-interference-triangulation/backend/internal/model"
)

var repoDBSeq atomic.Uint64

func newRepoTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:repo-basis-%d?mode=memory&cache=shared", repoDBSeq.Add(1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite test database: %v", err)
	}
	if err := db.AutoMigrate(
		&model.User{},
		&model.ReceiverStation{},
		&model.InterferenceCase{},
		&model.BearingObservation{},
		&model.LocalizationEstimate{},
		&model.AuditEvent{},
	); err != nil {
		t.Fatalf("migrate sqlite test database: %v", err)
	}
	return db
}

func testActor() Actor {
	return Actor{UserID: 2, Email: "analyst@spectrum.local", Role: constants.RoleAnalyst, RequestID: "req-repo-test"}
}

func TestMarkReviewBasisStaleTx(t *testing.T) {
	db := newRepoTestDB(t)
	basisID := uint(77)
	selectedAt := time.Now().UTC().Add(-30 * time.Minute)
	cs := model.InterferenceCase{
		CaseCode: "RF-REPO-1", Title: "仓储依据失效测试", FrequencyCenterHz: 433920000,
		CaseStatus: constants.CaseAnalyzing, Priority: "normal", OpenedBy: 1, Version: 3,
		ReviewBasisEstimateID: &basisID, ReviewBasisValid: true, ReviewBasisSelectedAt: &selectedAt,
	}
	if err := db.Create(&cs).Error; err != nil {
		t.Fatalf("create case: %v", err)
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		changed, err := MarkReviewBasisStaleTx(tx, cs.ID, constants.ReviewBasisStaleObservationExcluded, testActor())
		if err != nil {
			return err
		}
		if !changed {
			t.Fatal("expected review basis to be invalidated")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("mark stale transaction: %v", err)
	}

	var updated model.InterferenceCase
	if err := db.First(&updated, cs.ID).Error; err != nil {
		t.Fatalf("reload case: %v", err)
	}
	if updated.ReviewBasisValid || updated.ReviewBasisStaleReason != constants.ReviewBasisStaleObservationExcluded || updated.ReviewBasisStaleAt == nil {
		t.Fatalf("basis should be stale with reason, got %+v", updated)
	}
	if updated.Version != 4 {
		t.Fatalf("expected optimistic version 4, got %d", updated.Version)
	}
	var auditCount int64
	if err := db.Model(&model.AuditEvent{}).
		Where("entity_type = ? AND entity_id = ? AND action = ?", "interference_case", cs.ID, "review_basis.invalidated").
		Count(&auditCount).Error; err != nil {
		t.Fatalf("count audit: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("expected exactly one invalidation audit, got %d", auditCount)
	}

	// 已失效的依据重复标记应为空操作，不再制造审计或版本跳动。
	err = db.Transaction(func(tx *gorm.DB) error {
		changed, err := MarkReviewBasisStaleTx(tx, cs.ID, constants.ReviewBasisStaleObservationAdded, testActor())
		if err != nil {
			return err
		}
		if changed {
			t.Fatal("already-stale basis must not be invalidated twice")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("second mark stale transaction: %v", err)
	}
	var reloaded model.InterferenceCase
	if err := db.First(&reloaded, cs.ID).Error; err != nil {
		t.Fatalf("reload case: %v", err)
	}
	if reloaded.Version != 4 {
		t.Fatalf("expected version unchanged at 4, got %d", reloaded.Version)
	}
}

func TestMarkReviewBasisStaleTxEvidenceLockedCaseSkipped(t *testing.T) {
	db := newRepoTestDB(t)
	basisID := uint(88)
	selectedAt := time.Now().UTC().Add(-20 * time.Minute)
	cs := model.InterferenceCase{
		CaseCode: "RF-REPO-2", Title: "复核中证据锁定测试", FrequencyCenterHz: 433920000,
		CaseStatus: constants.CasePendingReview, Priority: "normal", OpenedBy: 1, Version: 5,
		ReviewBasisEstimateID: &basisID, ReviewBasisValid: true, ReviewBasisSelectedAt: &selectedAt,
	}
	if err := db.Create(&cs).Error; err != nil {
		t.Fatalf("create case: %v", err)
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		changed, err := MarkReviewBasisStaleTx(tx, cs.ID, constants.ReviewBasisStaleObservationAdded, testActor())
		if err != nil {
			return err
		}
		if changed {
			t.Fatal("pending_review case basis must not be invalidated by the stale hook")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("mark stale on locked case: %v", err)
	}
}
