import { useEffect, useMemo, useState } from 'react'
import HistoryEduRounded from '@mui/icons-material/HistoryEduRounded'
import PlayArrowRounded from '@mui/icons-material/PlayArrowRounded'
import ReplayRounded from '@mui/icons-material/ReplayRounded'
import ScienceRounded from '@mui/icons-material/ScienceRounded'
import TaskAltRounded from '@mui/icons-material/TaskAltRounded'
import { Alert, Box, Button, Chip, FormControlLabel, MenuItem, Stack, Switch, Table, TableBody, TableCell, TableHead, TableRow, TextField, Typography } from '@mui/material'
import { BearingPlot } from '../components/common/BearingPlot'
import { PageHeader } from '../components/common/PageHeader'
import { QualityBadge } from '../components/common/QualityBadge'
import { useAuth } from '../hooks/useAuth'
import { useLocalizationRun } from '../hooks/useLocalizationRun'
import { useCaseStore } from '../stores/caseStore'
import { useLocalizationStore } from '../stores/localizationStore'
import { useObservationStore } from '../stores/observationStore'
import { useStationStore } from '../stores/stationStore'
import type { LocalizationEstimate } from '../types/localization'
import { formatCoordinate, formatDateTime, formatDecimal, formatFrequency } from '../utils/format'
import { isRerunEstimate, reviewBasisBlock, staleReasonText } from '../utils/reviewBasis'

export function LocalizationPage() {
  const { hasRole } = useAuth()
  const canAnalyze = hasRole('analyst', 'admin')
  const cases = useCaseStore((state) => state.cases)
  const loadCases = useCaseStore((state) => state.load)
  const selectReviewBasis = useCaseStore((state) => state.selectReviewBasis)
  const stations = useStationStore((state) => state.stations)
  const loadStations = useStationStore((state) => state.load)
  const observations = useObservationStore((state) => state.observations)
  const loadObservations = useObservationStore((state) => state.load)
  const estimates = useLocalizationStore((state) => state.estimates)
  const selected = useLocalizationStore((state) => state.selected)
  const select = useLocalizationStore((state) => state.select)
  const loadEstimates = useLocalizationStore((state) => state.load)
  const { execute, busy, lastResult } = useLocalizationRun()
  const [caseId, setCaseId] = useState(0)
  const [allowOutlier, setAllowOutlier] = useState(true)
  const [basisBusyId, setBasisBusyId] = useState(0)

  useEffect(() => {
    void Promise.all([loadCases(), loadStations()])
  }, [loadCases, loadStations])

  useEffect(() => {
    if (!caseId) {
      const analyzing = cases.find((item) => item.case_status === 'analyzing')
      if (analyzing) setCaseId(analyzing.id)
    }
  }, [caseId, cases])

  useEffect(() => {
    if (caseId) void Promise.all([loadObservations(caseId), loadEstimates(caseId)])
  }, [caseId, loadEstimates, loadObservations])

  const selectedCase = cases.find((item) => item.id === caseId)
  const residuals = selected?.residuals_json ?? []
  const observationById = useMemo(() => new Map(observations.map((item) => [item.id, item])), [observations])
  const basisBlock = selectedCase ? reviewBasisBlock(selectedCase) : null
  const previousBasis = useMemo(
    () => (selectedCase?.previous_review_basis_estimate_id ? estimates.find((item) => item.id === selectedCase.previous_review_basis_estimate_id) : undefined),
    [estimates, selectedCase]
  )

  const run = async () => {
    if (!caseId) return
    await execute(caseId, allowOutlier)
    await loadCases()
  }

  const chooseBasis = async (estimate: LocalizationEstimate) => {
    if (!selectedCase) return
    setBasisBusyId(estimate.id)
    try {
      await selectReviewBasis(selectedCase.id, estimate.id, selectedCase.version)
    } finally {
      setBasisBusyId(0)
    }
  }

  return (
    <>
      <PageHeader
        eyebrow="WEIGHTED BEARING INTERSECTION / WLS V1"
        title="三角定位证据台"
        summary={selectedCase ? `${selectedCase.case_code} · ${formatFrequency(selectedCase.frequency_center_hz)} · ${selectedCase.active_observation_count} 条有效观测` : '选择 analyzing 案例后运行离线定位'}
        actions={canAnalyze ? <Button variant="contained" startIcon={<PlayArrowRounded />} disabled={!caseId || selectedCase?.case_status !== 'analyzing' || busy} onClick={() => void run()}>{busy ? '正在计算' : '运行加权定位'}</Button> : undefined}
      />

      <section className="control-strip localization-controls">
        <TextField select size="small" label="分析案例" value={caseId || ''} onChange={(event) => setCaseId(Number(event.target.value))} sx={{ minWidth: 330 }}>
          {cases.filter((item) => item.case_status === 'analyzing' || item.case_status === 'pending_review' || item.id === caseId).map((item) => <MenuItem key={item.id} value={item.id}>{item.case_code} · {item.title}</MenuItem>)}
        </TextField>
        <FormControlLabel control={<Switch checked={allowOutlier} onChange={(event) => setAllowOutlier(event.target.checked)} />} label="生成可解释离群候选" />
        {lastResult?.candidate && <Alert severity="warning">已保留原估计，并生成剔除观测 #{lastResult.candidate.outlier_ids_json[0]} 的候选重算。</Alert>}
      </section>

      {basisBlock?.kind === 'none' && <Alert severity="info" sx={{ mb: 2 }}>{basisBlock.message}</Alert>}
      {basisBlock?.kind === 'stale' && (
        <Alert severity="warning" icon={<ReplayRounded />} sx={{ mb: 2 }}>
          <Typography component="span" fontWeight={700}>{basisBlock.message}</Typography>
          {selectedCase?.review_basis_stale_at && <Typography component="div" variant="caption">失效时间 {formatDateTime(selectedCase.review_basis_stale_at)} · {selectedCase.review_basis_stale_reason ? staleReasonText[selectedCase.review_basis_stale_reason] : ''}</Typography>}
        </Alert>
      )}
      {basisBlock?.kind === 'ready' && (
        <Alert severity="success" icon={<TaskAltRounded />} sx={{ mb: 2 }}>
          {basisBlock.message}
          {selectedCase?.review_basis_selected_at && <Typography component="div" variant="caption">选定时间 {formatDateTime(selectedCase.review_basis_selected_at)}</Typography>}
          {previousBasis && <Typography component="div" variant="caption">退回前的上次依据为运行 #{previousBasis.id}（{formatCoordinate(previousBasis.latitude)}, {formatCoordinate(previousBasis.longitude)}），可与新依据对照。</Typography>}
        </Alert>
      )}
      {selectedCase?.case_status === 'pending_review' && (
        <Alert severity="info" icon={<HistoryEduRounded />} sx={{ mb: 2 }}>
          案例待复核，证据锁定：复核员以运行 #{selectedCase.review_basis_estimate_id ?? '-'} 作为唯一复核依据；退回后才能新增/排除观测并重跑。
        </Alert>
      )}

      <Alert severity="info" icon={<ScienceRounded />} className="safety-alert">估计坐标、不确定半径和离群候选均为离线模型证据，必须与原始方位线和残差共同复核。</Alert>

      <section className="localization-grid">
        <div className="plot-section plot-primary">
          <BearingPlot stations={stations} observations={observations} estimate={selected} height={520} />
        </div>
        <aside className="estimate-rail" aria-label="定位结果历史">
          <Typography component="h2" variant="h6">不可覆盖的运行历史</Typography>
          <Typography variant="caption" color="text.secondary">分析员必须选定且仅选定一条作为复核依据；新增/排除观测后原依据失效，需重跑并重新选定。</Typography>
          <Stack gap={1.5} mt={2}>
            {estimates.map((estimate) => (
              <EstimateCard
                key={estimate.id}
                estimate={estimate}
                viewed={selected?.id === estimate.id}
                isBasis={selectedCase?.review_basis_estimate_id === estimate.id}
                basisValid={selectedCase?.review_basis_valid ?? false}
                isPreviousBasis={selectedCase?.previous_review_basis_estimate_id === estimate.id}
                canSelect={canAnalyze && selectedCase?.case_status === 'analyzing'}
                rerunAllowed={isRerunEstimate(estimate, selectedCase?.review_basis_stale_at)}
                busy={basisBusyId === estimate.id}
                onView={() => select(estimate)}
                onChooseBasis={() => void chooseBasis(estimate)}
              />
            ))}
            {estimates.length === 0 && <Typography color="text.secondary">尚无运行结果。有效观测满足几何条件后可运行定位。</Typography>}
          </Stack>
        </aside>
      </section>

      <section className="data-section" aria-labelledby="residual-title">
        <Stack direction={{ xs: 'column', 'md': 'row' }} justifyContent="space-between" gap={1} mb={2}>
          <Typography id="residual-title" component="h2" variant="h6">逐站角度残差</Typography>
          {selected && <Typography variant="body2" color="text.secondary">算法 {selected.algorithm_version} · 创建于 {formatDateTime(selected.created_at)}{selectedCase?.review_basis_estimate_id === selected.id ? ' · 当前复核依据' : ''}</Typography>}
        </Stack>
        <Box className="table-scroll">
          <Table size="small" aria-label="定位残差证据">
            <TableHead><TableRow><TableCell>观测 / 测向站</TableCell><TableCell>观测方位</TableCell><TableCell>预测方位</TableCell><TableCell>角度残差</TableCell><TableCell>标准化残差</TableCell><TableCell>质量</TableCell></TableRow></TableHead>
            <TableBody>
              {residuals.map((residual) => {
                const observation = observationById.get(residual.observation_id)
                return <TableRow key={residual.observation_id} className={selected?.outlier_ids_json.includes(residual.observation_id) ? 'row-warning' : ''}>
                  <TableCell><strong>#{residual.observation_id}</strong> · {residual.station_code}</TableCell>
                  <TableCell className="numeric">{formatDecimal(residual.observed_deg, 2)}°</TableCell>
                  <TableCell className="numeric">{formatDecimal(residual.predicted_deg, 2)}°</TableCell>
                  <TableCell className="numeric">{residual.residual_deg >= 0 ? '+' : ''}{formatDecimal(residual.residual_deg, 2)}°</TableCell>
                  <TableCell className="numeric">{formatDecimal(residual.standardized, 2)} σ {selected?.outlier_ids_json.includes(residual.observation_id) && <strong> · 离群证据</strong>}</TableCell>
                  <TableCell>{observation ? <QualityBadge quality={observation.quality} /> : '历史快照'}</TableCell>
                </TableRow>
              })}
              {!selected && <TableRow><TableCell colSpan={6}>选择或运行一条定位结果后显示逐站残差。</TableCell></TableRow>}
            </TableBody>
          </Table>
        </Box>
      </section>
    </>
  )
}

interface EstimateCardProps {
  estimate: LocalizationEstimate
  viewed: boolean
  isBasis: boolean
  basisValid: boolean
  isPreviousBasis: boolean
  canSelect: boolean
  rerunAllowed: boolean
  busy: boolean
  onView: () => void
  onChooseBasis: () => void
}

function EstimateCard({ estimate, viewed, isBasis, basisValid, isPreviousBasis, canSelect, rerunAllowed, busy, onView, onChooseBasis }: EstimateCardProps) {
  return (
    <div className={`estimate-item ${viewed ? 'is-selected' : ''}`} role="group" aria-label={`定位运行 ${estimate.id}`}>
      <button type="button" className="estimate-view" onClick={onView}>
        <span className="estimate-item-top">
          <strong>运行 #{estimate.id}</strong>
          <span>{estimate.estimate_status === 'outlier_candidate' ? '△ 离群候选' : '◆ 原始估计'}</span>
        </span>
        <span className="estimate-coordinate">{formatCoordinate(estimate.latitude)}, {formatCoordinate(estimate.longitude)}</span>
        <span className="estimate-metrics">残差 {formatDecimal(estimate.residual_deg)}° · 半径 {formatDecimal(estimate.uncertainty_radius_m, 0)} m · {formatDateTime(estimate.created_at)}</span>
      </button>
      <Stack direction="row" gap={1} alignItems="center" flexWrap="wrap">
        {isBasis && (basisValid
          ? <Chip size="small" color="success" icon={<TaskAltRounded />} label="复核依据" />
          : <Chip size="small" color="warning" icon={<ReplayRounded />} label="依据已失效 · 需重跑重选" />)}
        {isPreviousBasis && <Chip size="small" variant="outlined" icon={<HistoryEduRounded />} label="退回前上次依据" />}
        {canSelect && !isBasis && (
          rerunAllowed
            ? <Button size="small" variant={viewed ? 'contained' : 'outlined'} disabled={busy} onClick={onChooseBasis}>{busy ? '正在设定' : '设为复核依据'}</Button>
            : <TooltipHint />
        )}
        {canSelect && isBasis && !basisValid && (
          rerunAllowed ? <Button size="small" variant="outlined" disabled={busy} onClick={onChooseBasis}>重新设为依据</Button> : <TooltipHint />
        )}
      </Stack>
    </div>
  )
}

function TooltipHint() {
  return <Typography variant="caption" color="warning.main">失效前的历史结果不能作为新依据，请先重跑定位。</Typography>
}
