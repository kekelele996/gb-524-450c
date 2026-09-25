package dto

import (
	"time"

	"spectrum-interference-triangulation/backend/internal/constants"
)

type CreateCaseRequest struct {
	CaseCode          string  `json:"case_code" binding:"required,min=4,max=40"`
	Title             string  `json:"title" binding:"required,min=4,max=160"`
	FrequencyCenterHz float64 `json:"frequency_center_hz" binding:"required,gt=0"`
	Priority          string  `json:"priority" binding:"required,oneof=low normal high"`
}

type TransitionCaseRequest struct {
	TargetStatus constants.CaseStatus `json:"target_status" binding:"required"`
	Version      uint                 `json:"version" binding:"required"`
	Conclusion   string               `json:"conclusion" binding:"max=2000"`
	Reason       string               `json:"reason" binding:"max=1000"`
}

// SelectReviewBasisRequest 由分析员从多条不可覆盖的定位结果中选定唯一复核依据。
type SelectReviewBasisRequest struct {
	EstimateID uint `json:"estimate_id" binding:"required"`
	Version    uint `json:"version" binding:"required"`
}

type CaseSummary struct {
	ID                            uint                 `json:"id"`
	CaseCode                      string               `json:"case_code"`
	Title                         string               `json:"title"`
	FrequencyCenterHz             float64              `json:"frequency_center_hz"`
	CaseStatus                    constants.CaseStatus `json:"case_status"`
	Priority                      string               `json:"priority"`
	Version                       uint                 `json:"version"`
	ObservationCount              int64                `json:"observation_count"`
	ActiveObservationCount        int64                `json:"active_observation_count"`
	EstimateCount                 int64                `json:"estimate_count"`
	Conclusion                    string               `json:"conclusion"`
	ReviewBasisEstimateID         *uint                `json:"review_basis_estimate_id"`
	ReviewBasisValid              bool                 `json:"review_basis_valid"`
	ReviewBasisStaleReason        string               `json:"review_basis_stale_reason"`
	ReviewBasisSelectedAt         *time.Time           `json:"review_basis_selected_at"`
	ReviewBasisStaleAt            *time.Time           `json:"review_basis_stale_at"`
	PreviousReviewBasisEstimateID *uint                `json:"previous_review_basis_estimate_id"`
}
