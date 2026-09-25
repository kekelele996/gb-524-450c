package model

import (
	"time"

	"spectrum-interference-triangulation/backend/internal/constants"
)

type InterferenceCase struct {
	ID                uint                 `json:"id" gorm:"primaryKey"`
	CaseCode          string               `json:"case_code" gorm:"size:40;not null;uniqueIndex"`
	Title             string               `json:"title" gorm:"size:160;not null"`
	FrequencyCenterHz float64              `json:"frequency_center_hz" gorm:"not null;check:frequency_center_hz > 0"`
	CaseStatus        constants.CaseStatus `json:"case_status" gorm:"type:varchar(24);not null;default:draft;check:case_status IN ('draft','collecting','analyzing','pending_review','confirmed','closed')"`
	Priority          string               `json:"priority" gorm:"size:16;not null;default:normal;check:priority IN ('low','normal','high')"`
	OpenedBy          uint                 `json:"opened_by" gorm:"not null"`
	ReviewerID        *uint                `json:"reviewer_id"`
	Conclusion        string               `json:"conclusion" gorm:"size:2000"`
	ReviewReason      string               `json:"review_reason" gorm:"size:1000"`
	// ReviewBasisEstimateID 是分析员选定的唯一复核依据（定位结果）。
	ReviewBasisEstimateID *uint `json:"review_basis_estimate_id" gorm:"index"`
	// ReviewBasisValid 表示当前依据是否仍可用于复核；观测增删或退回后变为 false。
	ReviewBasisValid bool `json:"review_basis_valid" gorm:"not null;default:false"`
	// ReviewBasisStaleReason 记录依据失效原因，便于定位页提示重跑原因。
	ReviewBasisStaleReason string `json:"review_basis_stale_reason" gorm:"size:40"`
	// ReviewBasisSelectedAt 记录依据被选定的时间；重新运行后新结果必须晚于该时间。
	ReviewBasisSelectedAt *time.Time `json:"review_basis_selected_at"`
	// ReviewBasisStaleAt 记录依据最近一次失效的时间。
	ReviewBasisStaleAt *time.Time `json:"review_basis_stale_at"`
	// PreviousReviewBasisEstimateID 退回后保留上次依据，供与新依据对照。
	PreviousReviewBasisEstimateID *uint      `json:"previous_review_basis_estimate_id"`
	Version                       uint       `json:"version" gorm:"not null;default:1"`
	ClosedAt                      *time.Time `json:"closed_at"`
	CreatedAt                     time.Time  `json:"created_at"`
	UpdatedAt                     time.Time  `json:"updated_at"`
}

func (InterferenceCase) TableName() string { return "interference_cases" }
