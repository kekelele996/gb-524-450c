package service

import (
	"context"
	"strings"

	"spectrum-interference-triangulation/backend/internal/constants"
	"spectrum-interference-triangulation/backend/internal/dto"
	"spectrum-interference-triangulation/backend/internal/model"
	"spectrum-interference-triangulation/backend/internal/repository"
	"spectrum-interference-triangulation/backend/pkg/api"
)

type CaseService struct {
	repo         *repository.CaseRepository
	estimateRepo *repository.EstimateRepository
}

func NewCaseService(repo *repository.CaseRepository, estimateRepo *repository.EstimateRepository) *CaseService {
	return &CaseService{repo: repo, estimateRepo: estimateRepo}
}

func (s *CaseService) List(ctx context.Context, page, pageSize int, status string) ([]dto.CaseSummary, int64, error) {
	if status != "" && !constants.ValidCaseStatus(constants.CaseStatus(status)) {
		return nil, 0, api.NewError(400, "INVALID_CASE_STATUS", "案例状态筛选值无效")
	}
	cases, total, err := s.repo.List(ctx, page, pageSize, status)
	if err != nil {
		return nil, 0, err
	}
	result := make([]dto.CaseSummary, 0, len(cases))
	for _, item := range cases {
		observations, active, estimates, countErr := s.repo.Counts(ctx, item.ID)
		if countErr != nil {
			return nil, 0, countErr
		}
		result = append(result, dto.CaseSummary{
			ID: item.ID, CaseCode: item.CaseCode, Title: item.Title,
			FrequencyCenterHz: item.FrequencyCenterHz, CaseStatus: item.CaseStatus,
			Priority: item.Priority, Version: item.Version, Conclusion: item.Conclusion,
			ObservationCount: observations, ActiveObservationCount: active, EstimateCount: estimates,
			ReviewBasisEstimateID:         item.ReviewBasisEstimateID,
			ReviewBasisValid:              item.ReviewBasisValid,
			ReviewBasisStaleReason:        item.ReviewBasisStaleReason,
			ReviewBasisSelectedAt:         item.ReviewBasisSelectedAt,
			ReviewBasisStaleAt:            item.ReviewBasisStaleAt,
			PreviousReviewBasisEstimateID: item.PreviousReviewBasisEstimateID,
		})
	}
	return result, total, nil
}

func (s *CaseService) Get(ctx context.Context, id uint) (model.InterferenceCase, error) {
	return s.repo.Get(ctx, id)
}

func (s *CaseService) Create(ctx context.Context, request dto.CreateCaseRequest, actor repository.Actor) (model.InterferenceCase, error) {
	if !constants.CanObserve(actor.Role) {
		return model.InterferenceCase{}, api.ErrForbidden
	}
	item := model.InterferenceCase{
		CaseCode: strings.ToUpper(strings.TrimSpace(request.CaseCode)),
		Title:    strings.TrimSpace(request.Title), FrequencyCenterHz: request.FrequencyCenterHz,
		CaseStatus: constants.CaseDraft, Priority: request.Priority,
		OpenedBy: actor.UserID, Version: 1,
	}
	if err := s.repo.Create(ctx, &item, actor); err != nil {
		return model.InterferenceCase{}, err
	}
	return item, nil
}

// SelectReviewBasis 由分析员从多条不可覆盖的定位结果中选定唯一复核依据。
// 只允许在 analyzing 状态选择本案例的结果；失效后必须选择重跑产生的新结果。
func (s *CaseService) SelectReviewBasis(ctx context.Context, caseID uint, request dto.SelectReviewBasisRequest, actor repository.Actor) (model.InterferenceCase, error) {
	if !constants.CanAnalyze(actor.Role) {
		return model.InterferenceCase{}, api.ErrForbidden
	}
	current, err := s.repo.Get(ctx, caseID)
	if err != nil {
		return model.InterferenceCase{}, err
	}
	if current.CaseStatus != constants.CaseAnalyzing {
		return model.InterferenceCase{}, api.WithDetails(api.NewError(409, "CASE_NOT_ANALYZING", "只有 analyzing 状态的案例可以选定复核依据"), map[string]any{"current": current.CaseStatus})
	}
	estimate, err := s.estimateRepo.Get(ctx, request.EstimateID)
	if err != nil {
		return model.InterferenceCase{}, err
	}
	if estimate.CaseID != caseID {
		return model.InterferenceCase{}, api.WithDetails(api.NewError(422, "REVIEW_BASIS_CASE_MISMATCH", "只能选择本案例运行产生的定位结果作为复核依据"), map[string]any{
			"estimate_id": estimate.ID, "estimate_case_id": estimate.CaseID,
		})
	}
	if current.ReviewBasisStaleAt != nil && !estimate.CreatedAt.After(*current.ReviewBasisStaleAt) {
		return model.InterferenceCase{}, api.WithDetails(api.NewError(422, "REVIEW_BASIS_STALE", "原依据已失效，必须重新运行定位并选择新结果，不能沿用历史结果"), map[string]any{
			"stale_at": current.ReviewBasisStaleAt, "estimate_created_at": estimate.CreatedAt,
		})
	}
	return s.repo.SelectReviewBasis(ctx, caseID, request.Version, request.EstimateID, actor)
}

func (s *CaseService) Transition(ctx context.Context, id uint, request dto.TransitionCaseRequest, actor repository.Actor) (model.InterferenceCase, error) {
	current, err := s.repo.Get(ctx, id)
	if err != nil {
		return model.InterferenceCase{}, err
	}
	if !constants.ValidCaseStatus(request.TargetStatus) {
		return model.InterferenceCase{}, api.NewError(400, "INVALID_CASE_STATUS", "目标案例状态无效")
	}
	if err := authorizeTransition(current.CaseStatus, request.TargetStatus, actor.Role); err != nil {
		return model.InterferenceCase{}, err
	}
	observations, active, estimates, err := s.repo.Counts(ctx, id)
	if err != nil {
		return model.InterferenceCase{}, err
	}
	_ = observations
	if request.TargetStatus == constants.CaseAnalyzing && active < 2 {
		return model.InterferenceCase{}, api.NewError(409, "INSUFFICIENT_OBSERVATIONS", "进入分析前至少需要两条有效观测")
	}
	if request.TargetStatus == constants.CasePendingReview {
		if active < 3 || estimates < 1 {
			return model.InterferenceCase{}, api.WithDetails(api.NewError(409, "REVIEW_EVIDENCE_INCOMPLETE", "提交复核前至少需要三条有效观测和一条定位结果"), map[string]any{
				"active_observations": active, "estimates": estimates,
			})
		}
		if err := s.validateReviewBasisForSubmission(ctx, current, active); err != nil {
			return model.InterferenceCase{}, err
		}
	}
	conclusion := strings.TrimSpace(request.Conclusion)
	reason := strings.TrimSpace(request.Reason)
	if request.TargetStatus == constants.CaseConfirmed && conclusion == "" {
		return model.InterferenceCase{}, api.NewError(422, "CONCLUSION_REQUIRED", "确认案例时必须填写人工复核结论")
	}
	if current.CaseStatus == constants.CasePendingReview && request.TargetStatus == constants.CaseAnalyzing && len(reason) < 6 {
		return model.InterferenceCase{}, api.NewError(422, "REVIEW_REASON_REQUIRED", "驳回案例时必须填写不少于 6 个字符的原因")
	}
	var reviewerID *uint
	if constants.CanReview(actor.Role) {
		reviewerID = &actor.UserID
	}
	return s.repo.Transition(ctx, id, request.Version, request.TargetStatus, conclusion, reason, reviewerID, actor)
}

// validateReviewBasisForSubmission 强制执行：提交复核时必须存在一条分析员明确选定、
// 仍然有效、且自失效后重新运行产生的定位结果作为唯一复核依据。
func (s *CaseService) validateReviewBasisForSubmission(ctx context.Context, current model.InterferenceCase, active int64) error {
	if current.ReviewBasisEstimateID == nil {
		return api.WithDetails(api.NewError(409, "REVIEW_BASIS_REQUIRED", "提交复核前必须由分析员在定位页选定一条定位结果作为复核依据"), map[string]any{
			"active_observations": active,
		})
	}
	if !current.ReviewBasisValid {
		return api.WithDetails(api.NewError(409, "REVIEW_BASIS_STALE", "原复核依据已随观测变化或退回而失效，必须重新运行定位并重新选定依据"), map[string]any{
			"stale_reason": current.ReviewBasisStaleReason,
		})
	}
	estimate, err := s.estimateRepo.Get(ctx, *current.ReviewBasisEstimateID)
	if err != nil {
		return err
	}
	if estimate.CaseID != current.ID {
		return api.NewError(409, "REVIEW_BASIS_CASE_MISMATCH", "复核依据不属于当前案例，不能提交复核")
	}
	if current.ReviewBasisStaleAt != nil && !estimate.CreatedAt.After(*current.ReviewBasisStaleAt) {
		return api.WithDetails(api.NewError(409, "REVIEW_BASIS_STALE", "依据失效后未重新运行定位，不能提交复核"), map[string]any{
			"stale_at": current.ReviewBasisStaleAt,
		})
	}
	return nil
}

func authorizeTransition(from, to constants.CaseStatus, role string) error {
	if from == constants.CasePendingReview || to == constants.CaseConfirmed || to == constants.CaseClosed {
		if !constants.CanReview(role) {
			return api.ErrForbidden
		}
		return nil
	}
	if to == constants.CaseAnalyzing || to == constants.CasePendingReview {
		if !constants.CanAnalyze(role) {
			return api.ErrForbidden
		}
		return nil
	}
	if !constants.CanObserve(role) {
		return api.ErrForbidden
	}
	return nil
}
