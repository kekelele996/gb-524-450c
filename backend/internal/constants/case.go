package constants

type CaseStatus string

const (
	CaseDraft         CaseStatus = "draft"
	CaseCollecting    CaseStatus = "collecting"
	CaseAnalyzing     CaseStatus = "analyzing"
	CasePendingReview CaseStatus = "pending_review"
	CaseConfirmed     CaseStatus = "confirmed"
	CaseClosed        CaseStatus = "closed"
)

var CaseStatuses = []CaseStatus{
	CaseDraft, CaseCollecting, CaseAnalyzing,
	CasePendingReview, CaseConfirmed, CaseClosed,
}

func ValidCaseStatus(value CaseStatus) bool {
	for _, status := range CaseStatuses {
		if status == value {
			return true
		}
	}
	return false
}

func CanTransitionCase(from, to CaseStatus) bool {
	switch from {
	case CaseDraft:
		return to == CaseCollecting
	case CaseCollecting:
		return to == CaseAnalyzing
	case CaseAnalyzing:
		return to == CasePendingReview
	case CasePendingReview:
		return to == CaseConfirmed || to == CaseAnalyzing
	case CaseConfirmed:
		return to == CaseClosed
	default:
		return false
	}
}

func CaseStatusValues() []string {
	values := make([]string, 0, len(CaseStatuses))
	for _, status := range CaseStatuses {
		values = append(values, string(status))
	}
	return values
}

// 复核依据（定位结果）失效原因。依据一旦因观测证据变化或案例退回而失效，
// 案例必须重新运行定位并重新选定结果后才能再次提交复核。
const (
	ReviewBasisStaleObservationAdded    = "observation_added"
	ReviewBasisStaleObservationExcluded = "observation_excluded"
	ReviewBasisStaleCaseRejected        = "case_rejected"
)

// ValidReviewBasisStaleReason 判断依据失效原因是否为系统已知值。
func ValidReviewBasisStaleReason(reason string) bool {
	switch reason {
	case ReviewBasisStaleObservationAdded, ReviewBasisStaleObservationExcluded, ReviewBasisStaleCaseRejected:
		return true
	default:
		return false
	}
}
