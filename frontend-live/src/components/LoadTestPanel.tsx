// 网页版小压测面板：表单 + 结果仪表区。
// 编排（工作池、清理、汇总）在 App.tsx 的 startLoadTest()，这里只做展示。
import { LatencyEvolution, type LoadLatencyPoint } from './LatencyEvolution';
import { LatencyHistogram, pct } from './LatencyHistogram';
import { CompareBars, SuccessBar } from './SuccessBar';
import { SCENARIOS, type Scenario } from '../api';

export type LoadRunState = {
  runId: string;
  scenario: Scenario;
  concurrency: number;
  running: boolean;
  total: number;
  done: number;
  ok: number;
  fail: number;
  latencies: number[]; // 成功请求的耗时（ms），直方图与分位数的口径
  points: LoadLatencyPoint[]; // 完成顺序上的每一个请求（成败都记）
  startedAt: number;
  finishedAt?: number;
  cleanupFail: number;
  samples: LoadRunSample[];
};

export type LoadRunSample = {
  index: number;
  ueId: string;
  success: boolean;
  retries: number;
  clientMs: number;
  gatewayMs: number;
  processingMs: number;
  signalPower: number | null;
  sinr: number | null;
  amfUeId?: string;
  sessionId?: string;
  ueIp?: string;
  message: string;
};

export type LoadRunSummary = {
  seq: number;
  total: number;
  ok: number;
  fail: number;
  rate: number; // 成功率 %
  median: number;
  p95: number;
  max: number;
  throughput: number; // 个/秒
};

type Props = {
  state: LoadRunState | null;
  runs: LoadRunSummary[];
  inputs: { concurrency: string; total: string };
  scenario: Scenario;
  onInput: (field: 'concurrency' | 'total', value: string) => void;
  onScenario: (scenario: Scenario) => void;
  onStart: () => void;
  disabled: boolean;
  demo: boolean;
};

// 从当前状态实时推导 6 个统计瓦片的数值（运行时用墙钟，结束后用 finishedAt）。
function liveStat(state: LoadRunState) {
  const elapsed = ((state.finishedAt ?? Date.now()) - state.startedAt) / 1000;
  const sorted = [...state.latencies].sort((a, b) => a - b);
  return {
    rate: state.total > 0 ? (state.ok * 100) / state.total : 0,
    median: pct(sorted, 0.5),
    p95: pct(sorted, 0.95),
    max: sorted.length > 0 ? sorted[sorted.length - 1] : 0,
    throughput: elapsed > 0 ? state.done / elapsed : 0,
  };
}

function csvCell(value: string | number | boolean | null | undefined): string {
  if (value === null || value === undefined) return '';
  let text = String(value);
  // 防止用户输入的 UE ID 被 Excel 当作公式执行。
  if (typeof value === 'string' && /^[=+\-@]/.test(text)) text = `'${text}`;
  return `"${text.replaceAll('"', '""')}"`;
}

function exportRunCsv(state: LoadRunState): void {
  const stats = liveStat(state);
  const headers = [
    'run_id',
    'scenario',
    'concurrency',
    'total',
    'started_at',
    'finished_at',
    'success_count',
    'failure_count',
    'success_rate_pct',
    'median_ms',
    'p95_ms',
    'max_ms',
    'throughput_ue_per_s',
    'cleanup_fail',
    'request_index',
    'ue_id',
    'success',
    'retry_count',
    'client_ms',
    'gateway_ms',
    'processing_ms',
    'signal_power_dbm',
    'sinr_db',
    'amf_ue_id',
    'session_id',
    'ue_ip',
    'message',
  ];
  const common = [
    state.runId,
    state.scenario,
    state.concurrency,
    state.total,
    new Date(state.startedAt).toISOString(),
    state.finishedAt ? new Date(state.finishedAt).toISOString() : '',
    state.ok,
    state.fail,
    stats.rate.toFixed(2),
    stats.median.toFixed(2),
    stats.p95.toFixed(2),
    stats.max.toFixed(2),
    stats.throughput.toFixed(2),
    state.cleanupFail,
  ];
  const rows = [...state.samples]
    .sort((left, right) => left.index - right.index)
    .map((sample) => [
      ...common,
      sample.index,
      sample.ueId,
      sample.success,
      sample.retries,
      sample.clientMs.toFixed(2),
      sample.gatewayMs.toFixed(2),
      sample.processingMs.toFixed(2),
      sample.signalPower,
      sample.sinr,
      sample.amfUeId,
      sample.sessionId,
      sample.ueIp,
      sample.message,
    ]);
  const csv = [headers, ...rows].map((row) => row.map(csvCell).join(',')).join('\r\n');
  const url = URL.createObjectURL(new Blob([`\uFEFF${csv}`], { type: 'text/csv;charset=utf-8' }));
  const link = document.createElement('a');
  link.href = url;
  link.download = `${state.runId}.csv`;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}

export function LoadTestPanel({ state, runs, inputs, scenario, onInput, onScenario, onStart, disabled, demo }: Props) {
  const controlsDisabled = disabled || !!state?.running;

  return (
    <div className="load-panel">
      <div className="load-form">
        <label htmlFor="load-scenario">压测信道模型</label>
        <select
          id="load-scenario"
          className="field load-scenario"
          value={scenario}
          aria-describedby="load-scenario-help"
          disabled={controlsDisabled}
          onChange={(event) => {
            const value = event.target.value as Scenario;
            if (Object.hasOwn(SCENARIOS, value)) onScenario(value);
          }}
        >
          {Object.entries(SCENARIOS).map(([key, value]) => (
            <option key={key} value={key}>
              {value.label}
            </option>
          ))}
        </select>
        <label htmlFor="load-concurrency">并发数（1–50）</label>
        <input
          id="load-concurrency"
          className="field load-input"
          inputMode="numeric"
          value={inputs.concurrency}
          disabled={controlsDisabled}
          onChange={(event) => onInput('concurrency', event.target.value)}
        />
        <label htmlFor="load-total">总数（1–200）</label>
        <input
          id="load-total"
          className="field load-input"
          inputMode="numeric"
          value={inputs.total}
          disabled={controlsDisabled}
          onChange={(event) => onInput('total', event.target.value)}
        />
        <button className="run-button load-button" type="button" disabled={controlsDisabled} onClick={onStart}>
          {state?.running ? `压测中 ${state.done}/${state.total}…` : '开始压测'}
        </button>
        {state && !state.running && state.samples.length > 0 && (
          <button className="ghost-button load-export-button" type="button" onClick={() => exportRunCsv(state)}>
            导出本轮 CSV
          </button>
        )}
        <p className="field-help" id="load-scenario-help">
          {SCENARIOS[scenario].description} 此选择仅用于压测，不会改变上方单次注册的信道模型。结束后自动逐个注销清理。
        </p>
      </div>
      {demo && <p className="hint">演示模式下不发真实请求——压测需要连上后端，等页面自动切回实时模式再试。</p>}
      {state ? (
        <LoadDashboard state={state} runs={runs} />
      ) : (
        <p className="hint">还没压测过。输入并发与总数，点「开始压测」——结果仪表区会实时成形，结束后保留最近一轮。</p>
      )}
    </div>
  );
}

// 仪表区单独成组件：state 一定非空，统计推导不需要判空。
function LoadDashboard({ state, runs }: { state: LoadRunState; runs: LoadRunSummary[] }) {
  const stats = liveStat(state);
  return (
    <div className="load-dashboard">
      <div className="load-stats">
        <dl className="load-stat">
          <dt>总请求</dt>
          <dd>
            {state.done}/{state.total}
          </dd>
        </dl>
        <dl className="load-stat">
          <dt>成功率</dt>
          <dd>{stats.rate.toFixed(1)}%</dd>
        </dl>
        <dl className="load-stat">
          <dt>中位数</dt>
          <dd>{state.latencies.length > 0 ? `${stats.median.toFixed(1)} ms` : '—'}</dd>
        </dl>
        <dl className="load-stat">
          <dt>P95</dt>
          <dd>{state.latencies.length > 0 ? `${stats.p95.toFixed(1)} ms` : '—'}</dd>
        </dl>
        <dl className="load-stat">
          <dt>最大</dt>
          <dd>{state.latencies.length > 0 ? `${stats.max.toFixed(1)} ms` : '—'}</dd>
        </dl>
        <dl className="load-stat">
          <dt>吞吐</dt>
          <dd>{stats.throughput.toFixed(1)}/s</dd>
        </dl>
      </div>
      {state.cleanupFail > 0 && (
        <p className="load-cleanup-warn">注销清理失败 {state.cleanupFail} 个——可到「在线终端」面板手动注销。</p>
      )}
      <LatencyEvolution points={state.points} total={state.total} running={state.running} />
      <div className="load-charts">
        <div className="load-chart-block">
          <h3>延迟分布（成功注册）</h3>
          <LatencyHistogram latencies={state.latencies} />
        </div>
        <div className="load-chart-block">
          <h3>成败占比</h3>
          <SuccessBar ok={state.ok} fail={state.fail} />
          <p className="hint">失败多数是门控拒绝；网关断连时在飞请求会全部计失败。</p>
          {runs.length > 0 && (
            <>
              <h3>最近 {runs.length} 轮对比</h3>
              <CompareBars runs={runs} />
            </>
          )}
        </div>
      </div>
    </div>
  );
}
