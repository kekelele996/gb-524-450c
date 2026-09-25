package repository

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"spectrum-interference-triangulation/backend/internal/constants"
	"spectrum-interference-triangulation/backend/internal/model"
	"spectrum-interference-triangulation/backend/pkg/api"
)

type EstimateRepository struct {
	db *gorm.DB
}

func NewEstimateRepository(db *gorm.DB) *EstimateRepository {
	return &EstimateRepository{db: db}
}

func (r *EstimateRepository) List(ctx context.Context, caseID uint) ([]model.LocalizationEstimate, error) {
	query := r.db.WithContext(ctx).Model(&model.LocalizationEstimate{})
	if caseID > 0 {
		query = query.Where("case_id = ?", caseID)
	}
	var estimates []model.LocalizationEstimate
	if err := query.Order("created_at DESC, id DESC").Find(&estimates).Error; err != nil {
		return nil, fmt.Errorf("list localization estimates: %w", err)
	}
	return estimates, nil
}

func (r *EstimateRepository) Get(ctx context.Context, id uint) (model.LocalizationEstimate, error) {
	var estimate model.LocalizationEstimate
	if err := r.db.WithContext(ctx).First(&estimate, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return model.LocalizationEstimate{}, api.NewError(404, "ESTIMATE_NOT_FOUND", "定位结果不存在")
		}
		return model.LocalizationEstimate{}, fmt.Errorf("get localization estimate: %w", err)
	}
	return estimate, nil
}

func (r *EstimateRepository) CreateRun(ctx context.Context, caseID, version uint, primary *model.LocalizationEstimate, candidate *model.LocalizationEstimate, allowOutlier bool, conditionLimit float64, actor Actor) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		claim := tx.Model(&model.InterferenceCase{}).
			Where("id = ? AND version = ? AND case_status = ?", caseID, version, constants.CaseAnalyzing).
			UpdateColumn("version", gorm.Expr("version + 1"))
		if claim.Error != nil {
			return fmt.Errorf("claim case localization run: %w", claim.Error)
		}
		if claim.RowsAffected != 1 {
			return api.NewError(409, "CASE_VERSION_CONFLICT", "案例状态或版本已变化，请刷新后重试")
		}
		if err := tx.Create(primary).Error; err != nil {
			return fmt.Errorf("save primary estimate: %w", err)
		}
		if candidate != nil {
			candidate.ParentEstimateID = &primary.ID
			if err := tx.Create(candidate).Error; err != nil {
				return fmt.Errorf("save outlier candidate estimate: %w", err)
			}
		}
		after := map[string]any{
			"primary_id":        primary.ID,
			"algorithm_version": primary.AlgorithmVersion,
			"allow_outlier":     allowOutlier,
			"condition_limit":   conditionLimit,
			"candidate_id": func() uint {
				if candidate == nil {
					return 0
				}
				return candidate.ID
			}(),
			"residual_deg":         primary.ResidualDeg,
			"condition_number":     primary.ConditionNumber,
			"geometry_degenerate":  primary.GeometryDegenerate,
			"used_observation_ids": primary.UsedObservationIDsJSON,
			"outlier_ids":          primary.OutlierIDsJSON,
		}
		audit := NewAudit(actor, "localization_estimate.created", "interference_case", caseID, map[string]any{"version": version}, after)
		if err := tx.Create(&audit).Error; err != nil {
			return fmt.Errorf("audit localization run: %w", err)
		}
		return nil
	})
}

// SelectReviewBasis 将一条定位结果选定为案例的复核依据。
// 依据失效（观测变化或案例退回）后，只允许选定失效时间之后重新运行的结果。
func (r *EstimateRepository) SelectReviewBasis(ctx context.Context, caseID, estimateID, version uint, actor Actor) (model.InterferenceCase, error) {
	var updated model.InterferenceCase
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var before model.InterferenceCase
		if err := tx.First(&before, caseID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return api.NewError(404, "CASE_NOT_FOUND", "干扰案例不存在")
			}
			return fmt.Errorf("load case for review basis selection: %w", err)
		}
		if before.CaseStatus != constants.CaseAnalyzing {
			return api.WithDetails(api.NewError(409, "CASE_NOT_ANALYZING", "只有 analyzing 状态的案例可以选定复核依据"), map[string]any{"current": before.CaseStatus})
		}
		if before.Version != version {
			return api.NewError(409, "CASE_VERSION_CONFLICT", "案例版本已更新，请刷新后重试")
		}
		var estimate model.LocalizationEstimate
		if err := tx.First(&estimate, estimateID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return api.NewError(404, "ESTIMATE_NOT_FOUND", "定位结果不存在")
			}
			return fmt.Errorf("load estimate for review basis: %w", err)
		}
		if estimate.CaseID != caseID {
			return api.NewError(422, "ESTIMATE_CASE_MISMATCH", "定位结果不属于该案例，不能作为复核依据")
		}
		if before.ReviewBasisInvalidatedAt != nil && !estimate.CreatedAt.After(*before.ReviewBasisInvalidatedAt) {
			return api.WithDetails(api.NewError(409, "REVIEW_BASIS_OUTDATED", "该定位结果早于依据失效时间，请重新运行定位后再选定"), map[string]any{
				"estimate_id": estimateID, "invalidated_at": before.ReviewBasisInvalidatedAt,
			})
		}
		result := tx.Model(&model.InterferenceCase{}).
			Where("id = ? AND version = ? AND case_status = ?", caseID, version, constants.CaseAnalyzing).
			Updates(map[string]any{
				"review_basis_estimate_id": estimateID, "review_basis_stale": false,
				"version": gorm.Expr("version + 1"),
			})
		if result.Error != nil {
			return fmt.Errorf("select review basis: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return api.NewError(409, "CASE_VERSION_CONFLICT", "案例被其他请求更新，请刷新后重试")
		}
		if err := tx.First(&updated, caseID).Error; err != nil {
			return fmt.Errorf("reload case after review basis selection: %w", err)
		}
		beforeBasis := map[string]any{
			"review_basis_estimate_id": before.ReviewBasisEstimateID,
			"review_basis_stale":       before.ReviewBasisStale,
			"version":                  before.Version,
		}
		afterBasis := map[string]any{
			"review_basis_estimate_id": estimateID,
			"review_basis_stale":       false,
			"estimate_residual_deg":    estimate.ResidualDeg,
			"estimate_created_at":      estimate.CreatedAt,
		}
		audit := NewAudit(actor, "interference_case.review_basis_selected", "interference_case", caseID, beforeBasis, afterBasis)
		if err := tx.Create(&audit).Error; err != nil {
			return fmt.Errorf("audit review basis selection: %w", err)
		}
		return nil
	})
	return updated, err
}
