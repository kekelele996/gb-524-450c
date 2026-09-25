package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"spectrum-interference-triangulation/backend/internal/constants"
	"spectrum-interference-triangulation/backend/internal/dto"
	"spectrum-interference-triangulation/backend/internal/model"
	"spectrum-interference-triangulation/backend/internal/repository"
	"spectrum-interference-triangulation/backend/pkg/api"
)

type reviewBasisFixture struct {
	db           *gorm.DB
	estimates    *EstimateService
	cases        *CaseService
	observations *ObservationService
	caseID       uint
	analyst      repository.Actor
	reviewer     repository.Actor
}

func newReviewBasisFixture(t *testing.T) *reviewBasisFixture {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open in-memory database: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.ReceiverStation{}, &model.InterferenceCase{}, &model.BearingObservation{}, &model.LocalizationEstimate{}, &model.AuditEvent{}); err != nil {
		t.Fatalf("migrate schema: %v", err)
	}
	analystUser := model.User{Email: "analyst@spectrum.local", DisplayName: "分析员", PasswordHash: "x", Role: constants.RoleAnalyst, Active: true}
	reviewerUser := model.User{Email: "reviewer@spectrum.local", DisplayName: "复核员", PasswordHash: "x", Role: constants.RoleReviewer, Active: true}
	if err := db.Create(&analystUser).Error; err != nil {
		t.Fatalf("seed analyst: %v", err)
	}
	if err := db.Create(&reviewerUser).Error; err != nil {
		t.Fatalf("seed reviewer: %v", err)
	}
	calibrated := time.Now().UTC().Add(-24 * time.Hour)
	stations := []model.ReceiverStation{
		{StationCode: "RX-WEST", Name: "西", Latitude: 31.2304, Longitude: 121.4437, AccuracyDeg: 1.2, StationStatus: "active", CalibratedAt: &calibrated},
		{StationCode: "RX-SOUTH", Name: "南", Latitude: 31.2104, Longitude: 121.4737, AccuracyDeg: 1.5, StationStatus: "active", CalibratedAt: &calibrated},
		{StationCode: "RX-EAST", Name: "东", Latitude: 31.2304, Longitude: 121.5037, AccuracyDeg: 1.0, StationStatus: "active", CalibratedAt: &calibrated},
		{StationCode: "RX-NORTH", Name: "北", Latitude: 31.2504, Longitude: 121.4737, AccuracyDeg: 1.8, StationStatus: "active", CalibratedAt: &calibrated},
	}
	if err := db.Create(&stations).Error; err != nil {
		t.Fatalf("seed stations: %v", err)
	}
	caseRecord := model.InterferenceCase{
		CaseCode: "RF-TEST-001", Title: "复核依据测试案例", FrequencyCenterHz: 433920000,
		CaseStatus: constants.CaseAnalyzing, Priority: "normal", OpenedBy: analystUser.ID, Version: 1,
	}
	if err := db.Create(&caseRecord).Error; err != nil {
		t.Fatalf("seed case: %v", err)
	}
	now := time.Now().UTC()
	bearings := []float64{90.0, 0.2, 269.8, 205.0}
	observations := make([]model.BearingObservation, 0, len(bearings))
	for index, bearing := range bearings {
		observations = append(observations, model.BearingObservation{
			StationID: stations[index].ID, CaseID: caseRecord.ID,
			BearingDeg: bearing, CorrectedBearingDeg: bearing, SignalDBM: -70,
			FrequencyHz: 433920000, BandwidthHz: 12500, ObservedAt: now,
			Quality: constants.QualityGood, CreatedBy: analystUser.ID,
		})
	}
	if err := db.Create(&observations).Error; err != nil {
		t.Fatalf("seed observations: %v", err)
	}
	estimateRepo := repository.NewEstimateRepository(db)
	observationRepo := repository.NewObservationRepository(db)
	caseRepo := repository.NewCaseRepository(db)
	stationRepo := repository.NewStationRepository(db)
	return &reviewBasisFixture{
		db:           db,
		estimates:    NewEstimateService(estimateRepo, observationRepo, caseRepo, 1000),
		cases:        NewCaseService(caseRepo),
		observations: NewObservationService(observationRepo, stationRepo, caseRepo),
		caseID:       caseRecord.ID,
		analyst:      repository.Actor{UserID: analystUser.ID, Email: analystUser.Email, Role: constants.RoleAnalyst, RequestID: "test-analyst"},
		reviewer:     repository.Actor{UserID: reviewerUser.ID, Email: reviewerUser.Email, Role: constants.RoleReviewer, RequestID: "test-reviewer"},
	}
}

func (f *reviewBasisFixture) currentCase(t *testing.T) model.InterferenceCase {
	t.Helper()
	caseRecord, err := f.cases.Get(context.Background(), f.caseID)
	if err != nil {
		t.Fatalf("reload case: %v", err)
	}
	return caseRecord
}

func (f *reviewBasisFixture) runLocalization(t *testing.T) RunResult {
	t.Helper()
	result, err := f.estimates.Run(context.Background(), dto.RunLocalizationRequest{CaseID: f.caseID, AllowOutlier: false}, f.analyst)
	if err != nil {
		t.Fatalf("run localization: %v", err)
	}
	return result
}

func (f *reviewBasisFixture) selectBasis(t *testing.T, estimateID, version uint) (model.InterferenceCase, error) {
	t.Helper()
	return f.estimates.SelectReviewBasis(context.Background(), f.caseID, dto.SelectReviewBasisRequest{EstimateID: estimateID, Version: version}, f.analyst)
}

func (f *reviewBasisFixture) auditCount(action string) int64 {
	var count int64
	f.db.Model(&model.AuditEvent{}).Where("action = ?", action).Count(&count)
	return count
}

func expectAPIError(t *testing.T, err error, code string) {
	t.Helper()
	var appErr *api.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("expected API error %s, got %v", code, err)
	}
	if appErr.Code != code {
		t.Fatalf("expected error code %s, got %s (%s)", code, appErr.Code, appErr.Message)
	}
}

func TestReviewBasisLifecycle(t *testing.T) {
	f := newReviewBasisFixture(t)
	ctx := context.Background()

	// 有定位结果但未选定依据时不能提交复核。
	first := f.runLocalization(t)
	caseRecord := f.currentCase(t)
	_, err := f.cases.Transition(ctx, f.caseID, dto.TransitionCaseRequest{TargetStatus: constants.CasePendingReview, Version: caseRecord.Version}, f.analyst)
	expectAPIError(t, err, "REVIEW_BASIS_REQUIRED")

	// 非分析员不能选定依据。
	caseRecord = f.currentCase(t)
	if _, err := f.estimates.SelectReviewBasis(ctx, f.caseID, dto.SelectReviewBasisRequest{EstimateID: first.Primary.ID, Version: caseRecord.Version}, repository.Actor{UserID: 99, Email: "observer@spectrum.local", Role: constants.RoleObserver, RequestID: "test-observer"}); !errors.Is(err, api.ErrForbidden) {
		t.Fatalf("expected forbidden for observer, got %v", err)
	}

	// 分析员选定一条定位结果作为复核依据，产生审计。
	updated, err := f.selectBasis(t, first.Primary.ID, caseRecord.Version)
	if err != nil {
		t.Fatalf("select review basis: %v", err)
	}
	if updated.ReviewBasisEstimateID == nil || *updated.ReviewBasisEstimateID != first.Primary.ID || updated.ReviewBasisStale {
		t.Fatalf("basis not applied: %+v", updated)
	}
	if f.auditCount("interference_case.review_basis_selected") != 1 {
		t.Fatal("review basis selection must be audited")
	}

	// 版本过期时选定失败。
	if _, err := f.selectBasis(t, first.Primary.ID, caseRecord.Version); err != nil {
		expectAPIError(t, err, "CASE_VERSION_CONFLICT")
	}

	// 依据有效时可以提交复核。
	caseRecord = f.currentCase(t)
	if _, err := f.cases.Transition(ctx, f.caseID, dto.TransitionCaseRequest{TargetStatus: constants.CasePendingReview, Version: caseRecord.Version}, f.analyst); err != nil {
		t.Fatalf("submit review with valid basis: %v", err)
	}

	// 复核员退回：保留上次依据供对照，但标记失效。
	caseRecord = f.currentCase(t)
	returned, err := f.cases.Transition(ctx, f.caseID, dto.TransitionCaseRequest{TargetStatus: constants.CaseAnalyzing, Version: caseRecord.Version, Reason: "残差证据不足，请补充观测"}, f.reviewer)
	if err != nil {
		t.Fatalf("return case: %v", err)
	}
	if returned.ReviewBasisEstimateID == nil || *returned.ReviewBasisEstimateID != first.Primary.ID {
		t.Fatal("returned case must keep previous basis for comparison")
	}
	if !returned.ReviewBasisStale || returned.ReviewBasisInvalidatedAt == nil {
		t.Fatal("returned case basis must be marked stale with invalidation time")
	}

	// 依据失效后不能直接再次提交。
	caseRecord = f.currentCase(t)
	_, err = f.cases.Transition(ctx, f.caseID, dto.TransitionCaseRequest{TargetStatus: constants.CasePendingReview, Version: caseRecord.Version}, f.analyst)
	expectAPIError(t, err, "REVIEW_BASIS_STALE")

	// 失效前运行的结果不能重新选定，必须重新运行。
	caseRecord = f.currentCase(t)
	_, err = f.selectBasis(t, first.Primary.ID, caseRecord.Version)
	expectAPIError(t, err, "REVIEW_BASIS_OUTDATED")

	// 重新运行并选定新依据后可以再次提交。
	second := f.runLocalization(t)
	caseRecord = f.currentCase(t)
	if _, err := f.selectBasis(t, second.Primary.ID, caseRecord.Version); err != nil {
		t.Fatalf("reselect basis after rerun: %v", err)
	}
	caseRecord = f.currentCase(t)
	if _, err := f.cases.Transition(ctx, f.caseID, dto.TransitionCaseRequest{TargetStatus: constants.CasePendingReview, Version: caseRecord.Version}, f.analyst); err != nil {
		t.Fatalf("resubmit after rerun and reselect: %v", err)
	}
}

func TestReviewBasisInvalidatedByObservationChanges(t *testing.T) {
	f := newReviewBasisFixture(t)
	ctx := context.Background()

	run := f.runLocalization(t)
	caseRecord := f.currentCase(t)
	if _, err := f.selectBasis(t, run.Primary.ID, caseRecord.Version); err != nil {
		t.Fatalf("select review basis: %v", err)
	}

	// 新增观测让依据失效并进入审计。
	if _, err := f.observations.Create(ctx, dto.CreateObservationRequest{
		StationID: 1, CaseID: f.caseID, BearingDeg: 45, SignalDBM: -80,
		FrequencyHz: 433920000, BandwidthHz: 12500, Quality: string(constants.QualityFair),
	}, f.analyst); err != nil {
		t.Fatalf("create observation: %v", err)
	}
	caseRecord = f.currentCase(t)
	if !caseRecord.ReviewBasisStale || caseRecord.ReviewBasisInvalidatedAt == nil {
		t.Fatal("new observation must invalidate the selected basis")
	}
	if caseRecord.ReviewBasisEstimateID == nil || *caseRecord.ReviewBasisEstimateID != run.Primary.ID {
		t.Fatal("invalidated basis ID must be kept for comparison")
	}
	if f.auditCount("interference_case.review_basis_invalidated") != 1 {
		t.Fatal("basis invalidation must be audited")
	}

	// 失效后不能提交复核。
	_, err := f.cases.Transition(ctx, f.caseID, dto.TransitionCaseRequest{TargetStatus: constants.CasePendingReview, Version: caseRecord.Version}, f.analyst)
	expectAPIError(t, err, "REVIEW_BASIS_STALE")

	// 重新运行并选定后，排除观测同样让依据失效。
	second := f.runLocalization(t)
	caseRecord = f.currentCase(t)
	if _, err := f.selectBasis(t, second.Primary.ID, caseRecord.Version); err != nil {
		t.Fatalf("reselect after rerun: %v", err)
	}
	if _, err := f.observations.Exclude(ctx, 1, dto.ExcludeObservationRequest{Reason: "测向站校准漂移"}, f.analyst); err != nil {
		t.Fatalf("exclude observation: %v", err)
	}
	caseRecord = f.currentCase(t)
	if !caseRecord.ReviewBasisStale {
		t.Fatal("excluding an observation must invalidate the selected basis")
	}
	if f.auditCount("interference_case.review_basis_invalidated") != 2 {
		t.Fatal("each invalidation must be audited")
	}
}

func TestSelectReviewBasisRejectsCrossCaseEstimate(t *testing.T) {
	f := newReviewBasisFixture(t)
	ctx := context.Background()

	otherCase := model.InterferenceCase{
		CaseCode: "RF-TEST-002", Title: "另一个案例", FrequencyCenterHz: 433920000,
		CaseStatus: constants.CaseAnalyzing, Priority: "normal", OpenedBy: f.analyst.UserID, Version: 1,
	}
	if err := f.db.Create(&otherCase).Error; err != nil {
		t.Fatalf("create second case: %v", err)
	}
	run := f.runLocalization(t)
	if _, err := f.estimates.SelectReviewBasis(ctx, otherCase.ID, dto.SelectReviewBasisRequest{EstimateID: run.Primary.ID, Version: 1}, f.analyst); err != nil {
		expectAPIError(t, err, "ESTIMATE_CASE_MISMATCH")
	} else {
		t.Fatal("cross-case estimate must be rejected")
	}
}
