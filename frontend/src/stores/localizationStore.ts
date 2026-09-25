import { create } from 'zustand'
import { localizationApi } from '../api/localizations'
import type { InterferenceCase } from '../types/case'
import type { LocalizationEstimate, LocalizationRunResult } from '../types/localization'

interface LocalizationState {
  estimates: LocalizationEstimate[]
  selected: LocalizationEstimate | null
  busy: boolean
  basisBusy: boolean
  load: (caseId?: number) => Promise<void>
  run: (caseId: number, allowOutlier: boolean) => Promise<LocalizationRunResult>
  select: (estimate: LocalizationEstimate | null) => void
  selectBasis: (caseId: number, estimateId: number, version: number) => Promise<InterferenceCase>
}

export const useLocalizationStore = create<LocalizationState>((set, get) => ({
  estimates: [],
  selected: null,
  busy: false,
  basisBusy: false,
  load: async (caseId) => {
    const response = await localizationApi.list(caseId)
    set({ estimates: response.data, selected: response.data[0] ?? null })
  },
  run: async (caseId, allowOutlier) => {
    set({ busy: true })
    try {
      const response = await localizationApi.run(caseId, allowOutlier)
      const additions = response.data.candidate ? [response.data.candidate, response.data.primary] : [response.data.primary]
      set({ estimates: [...additions, ...get().estimates], selected: response.data.candidate ?? response.data.primary })
      return response.data
    } finally {
      set({ busy: false })
    }
  },
  select: (estimate) => set({ selected: estimate }),
  selectBasis: async (caseId, estimateId, version) => {
    set({ basisBusy: true })
    try {
      const response = await localizationApi.selectBasis(caseId, estimateId, version)
      return response.data
    } finally {
      set({ basisBusy: false })
    }
  }
}))

