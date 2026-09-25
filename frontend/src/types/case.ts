export type CaseStatus = 'draft' | 'collecting' | 'analyzing' | 'pending_review' | 'confirmed' | 'closed'
export type CasePriority = 'low' | 'normal' | 'high'

// 复核依据失效原因，与后端 constants/review_basis.go 保持一致。
export type ReviewBasisStaleReason = 'observation_added' | 'observation_excluded' | 'case_rejected' | ''

export interface InterferenceCase {
  id: number
  case_code: string
  title: string
  frequency_center_hz: number
  case_status: CaseStatus
  priority: CasePriority
  opened_by: number
  reviewer_id: number | null
  conclusion: string
  review_reason: string
  review_basis_estimate_id: number | null
  review_basis_valid: boolean
  review_basis_stale_reason: ReviewBasisStaleReason
  review_basis_selected_at: string | null
  review_basis_stale_at: string | null
  previous_review_basis_estimate_id: number | null
  version: number
  closed_at: string | null
  created_at: string
  updated_at: string
}

export interface CaseSummary {
  id: number
  case_code: string
  title: string
  frequency_center_hz: number
  case_status: CaseStatus
  priority: CasePriority
  version: number
  observation_count: number
  active_observation_count: number
  estimate_count: number
  conclusion: string
  review_basis_estimate_id: number | null
  review_basis_valid: boolean
  review_basis_stale_reason: ReviewBasisStaleReason
  review_basis_selected_at: string | null
  review_basis_stale_at: string | null
  previous_review_basis_estimate_id: number | null
}

export interface CaseInput {
  case_code: string
  title: string
  frequency_center_hz: number
  priority: CasePriority
}

export interface CaseTransition {
  target_status: CaseStatus
  version: number
  conclusion?: string
  reason?: string
}

