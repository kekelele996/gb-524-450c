import type { CaseSummary, ReviewBasisStaleReason } from '../types/case'
import type { LocalizationEstimate } from '../types/localization'

export const staleReasonText: Record<Exclude<ReviewBasisStaleReason, ''>, string> = {
  observation_added: '选定依据后新增了观测，定位输入已变化',
  observation_excluded: '选定依据后排除了观测，定位输入已变化',
  case_rejected: '案例被复核员退回，需要重新分析'
}

// 定位页判定：某条结果在依据失效后是否仍然可以被重新选为依据。
export function isRerunEstimate(estimate: LocalizationEstimate, staleAt?: string | null): boolean {
  if (!staleAt) return true
  return new Date(estimate.created_at).getTime() > new Date(staleAt).getTime()
}

// 提交复核前的依据状态，用于定位页与案例页的统一提示。
export type BasisBlock =
  | { kind: 'none'; message: string }
  | { kind: 'stale'; message: string }
  | { kind: 'ready'; message: string }

export function reviewBasisBlock(item: Pick<CaseSummary, 'case_status' | 'review_basis_estimate_id' | 'review_basis_valid' | 'review_basis_stale_reason'>): BasisBlock | null {
  if (item.case_status !== 'analyzing') return null
  if (!item.review_basis_estimate_id) {
    return { kind: 'none', message: '尚未选定复核依据：请运行定位后，从右侧结果中选定一条作为复核员的唯一复核依据。' }
  }
  if (!item.review_basis_valid) {
    const reason = item.review_basis_stale_reason ? staleReasonText[item.review_basis_stale_reason] : undefined
    return {
      kind: 'stale',
      message: `原复核依据已失效${reason ? `（${reason}）` : ''}，当前结果不能提交复核。请重新运行定位，再从新产生的结果中选定复核依据。`
    }
  }
  return { kind: 'ready', message: '复核依据已选定且仍然有效，可以提交复核。' }
}
