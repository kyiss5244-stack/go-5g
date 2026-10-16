import { useEffect, useRef, useState } from 'react';
import { ApiError, apiOAIDatasets, apiOAIRecords, apiOAISeries, type OAIDataset, type OAIRecord, type OAIStats } from '../api';

function number(value: number, digits = 0): string {
  return value.toLocaleString('zh-CN', { minimumFractionDigits: digits, maximumFractionDigits: digits });
}

function SINRChart({ points, cursor }: { points: OAIRecord[]; cursor: number }) {
  const width = 920;
  const height = 250;
  const padding = { left: 48, right: 22, top: 18, bottom: 32 };
  const values = points.map((point) => point.sinrDb);
  const rawMin = values.length ? Math.min(...values) : 0;
  const rawMax = values.length ? Math.max(...values) : 1;
  const min = Math.floor(rawMin / 5) * 5 - 2;
  const max = Math.ceil(rawMax / 5) * 5 + 2;
  const innerWidth = width - padding.left - padding.right;
  const innerHeight = height - padding.top - padding.bottom;
  const x = (index: number) => padding.left + (points.length <= 1 ? 0 : (index / (points.length - 1)) * innerWidth);
  const y = (value: number) => padding.top + ((max - value) / Math.max(1, max - min)) * innerHeight;
  const line = points.map((point, index) => `${index === 0 ? 'M' : 'L'} ${x(index).toFixed(2)} ${y(point.sinrDb).toFixed(2)}`).join(' ');
  const area = points.length ? `${line} L ${x(points.length - 1)} ${padding.top + innerHeight} L ${padding.left} ${padding.top + innerHeight} Z` : '';
  const ticks = [max, (max + min) / 2, min];
  const current = points[Math.min(cursor, Math.max(0, points.length - 1))];

  return (
    <div className="oai-chart-wrap">
      <svg className="oai-chart" viewBox={`0 0 ${width} ${height}`} role="img" aria-label="所选终端的 SINR 变化曲线">
        <defs>
          <linearGradient id="oai-sinr-fill" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="var(--primary)" stopOpacity="0.16" />
            <stop offset="100%" stopColor="var(--primary)" stopOpacity="0" />
          </linearGradient>
        </defs>
        {ticks.map((tick, index) => (
          <g key={`${index}-${tick}`}>
            <line x1={padding.left} x2={width - padding.right} y1={y(tick)} y2={y(tick)} className="oai-chart-grid" />
            <text x={padding.left - 10} y={y(tick) + 4} textAnchor="end" className="oai-chart-axis">{number(tick, 1)}</text>
          </g>
        ))}
        {area && <path d={area} fill="url(#oai-sinr-fill)" />}
        {line && <path d={line} className="oai-chart-line" />}
        {current && (
          <g>
            <line x1={x(cursor)} x2={x(cursor)} y1={padding.top} y2={padding.top + innerHeight} className="oai-chart-cursor" />
            <circle cx={x(cursor)} cy={y(current.sinrDb)} r="4.5" className="oai-chart-point" />
          </g>
        )}
        <text x={padding.left} y={height - 7} className="oai-chart-axis">记录起点</text>
        <text x={width - padding.right} y={height - 7} textAnchor="end" className="oai-chart-axis">记录终点</text>
      </svg>
      <span className="oai-chart-label">SINR · dB</span>
    </div>
  );
}

const OAI_FLOW_NODES = [
  { id: 'UE', name: '实验终端', detail: 'RNTI' },
  { id: 'gNB', name: 'OAI 基站', detail: '调度记录' },
  { id: 'AMF', name: '接入管理', detail: '流程映射' },
  { id: 'SMF', name: '会话管理', detail: '流程映射' },
  { id: 'UPF', name: '用户面', detail: '流程映射' },
] as const;

function OAIFlow({ record, cursor, playing }: { record?: OAIRecord; cursor: number; playing: boolean }) {
  const activeStep = playing ? cursor % OAI_FLOW_NODES.length : cursor > 0 ? OAI_FLOW_NODES.length - 1 : 0;
  return (
    <section className="flow-surface oai-flow-surface" aria-labelledby="oai-flow-title">
      <div className="section-heading flow-heading">
        <div><h2 id="oai-flow-title">核心网实验路径</h2><p>用当前 OAI 记录驱动原实验台的流程效果</p></div>
        <span className="protocol-label">历史数据驱动</span>
      </div>
      <div className={`flow-summary ${playing ? 'working' : cursor > 0 ? 'ok' : ''}`}>
        <span className="state-dot" aria-hidden="true" />
        <strong>{playing ? `正在演示第 ${cursor + 1} 条记录的核心网路径` : cursor > 0 ? '回放已暂停，流程停在当前记录' : '记录已就绪，开始回放后沿核心网路径推进'}</strong>
      </div>
      <ol className="network-path oai-network-path" aria-label="OAI 数据驱动的核心网流程">
        {OAI_FLOW_NODES.map((node, index) => {
          const reached = index <= activeStep;
          const completed = playing ? index < activeStep : cursor > 0 && reached;
          const currentNode = playing && index === activeStep;
          return (
            <li key={node.id} className={[reached ? 'reached' : '', completed ? 'completed' : '', currentNode ? 'current' : ''].filter(Boolean).join(' ')}>
              <div className="node-symbol" aria-hidden="true"><span>{node.id}</span>{completed && <i className="node-result" />}</div>
              <strong>{node.id}</strong><span>{node.name}</span><small>{node.detail}</small>
              {index < OAI_FLOW_NODES.length - 1 && <span className={`path-arrow ${index < activeStep ? 'passed' : currentNode ? 'active' : ''}`} aria-hidden="true" />}
            </li>
          );
        })}
      </ol>
      <div className="measurement-grid oai-measurement-grid" aria-label="当前 OAI 调度测量">
        <div className={`measurement ${record && record.sinrDb >= 0 ? 'ok' : ''}`}><div className="measurement-label"><span>SINR</span><small>OAI 原始记录</small></div><strong className="mono">{record ? `${number(record.sinrDb, 1)} dB` : '等待记录'}</strong></div>
        <div className="measurement"><div className="measurement-label"><span>MCS</span><small>上行调制编码</small></div><strong className="mono">{record ? record.mcs : '—'}</strong></div>
        <div className="measurement"><div className="measurement-label"><span>PRB</span><small>上行资源块</small></div><strong className="mono">{record ? record.prbs : '—'}</strong></div>
      </div>
      <p className="flow-footnote">此路径复用核心网实验的视觉效果。QUEST 历史记录只驱动显示和回放，不表示该记录真实经过当前 LiteCore 的 AMF、SMF 或 UPF。</p>
    </section>
  );
}

export function OAIDataPanel() {
  const [datasets, setDatasets] = useState<OAIDataset[]>([]);
  const [datasetId, setDatasetId] = useState('');
  const [rnti, setRNTI] = useState('');
  const [points, setPoints] = useState<OAIRecord[]>([]);
  const [records, setRecords] = useState<OAIRecord[]>([]);
  const [sourceCount, setSourceCount] = useState(0);
  const [downsampled, setDownsampled] = useState(false);
  const [stats, setStats] = useState<OAIStats>({ count: 0, avgSinrDb: 0, avgMcs: 0, avgPrbs: 0 });
  const [cursor, setCursor] = useState(0);
  const [playing, setPlaying] = useState(false);
  const [speed, setSpeed] = useState(1);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const interval = useRef<number | null>(null);
  const dataset = datasets.find((item) => item.id === datasetId) ?? datasets[0];
  const current = points[Math.min(cursor, Math.max(0, points.length - 1))];

  useEffect(() => {
    let active = true;
    apiOAIDatasets()
      .then(({ datasets: available }) => {
        if (!active) return;
        setDatasets(available);
        const first = available.find((item) => !item.loadError);
        if (first) {
          setDatasetId(first.id);
          setRNTI(first.ues[0] ?? '');
        }
      })
      .catch((cause) => active && setError(cause instanceof ApiError ? cause.message : '无法读取 OAI 数据目录'))
      .finally(() => active && setLoading(false));
    return () => { active = false; };
  }, []);

  useEffect(() => {
    if (!datasetId || !rnti) return;
    let active = true;
    setLoading(true);
    setError('');
    setPlaying(false);
    setCursor(0);
    setStats({ count: 0, avgSinrDb: 0, avgMcs: 0, avgPrbs: 0 });
    Promise.all([apiOAISeries(datasetId, rnti), apiOAIRecords(datasetId, rnti, 100)])
      .then(([series, table]) => {
        if (!active) return;
        setPoints(series.points);
        setSourceCount(series.sourceCount);
        setDownsampled(series.downsampled);
        setStats(series.stats);
        setRecords(table.records);
      })
      .catch((cause) => active && setError(cause instanceof ApiError ? cause.message : '无法读取 OAI 实验记录'))
      .finally(() => active && setLoading(false));
    return () => { active = false; };
  }, [datasetId, rnti]);

  useEffect(() => {
    if (!playing || points.length < 2) return;
    interval.current = window.setInterval(() => {
      setCursor((value) => {
        if (value >= points.length - 1) {
          setPlaying(false);
          return value;
        }
        return value + 1;
      });
    }, 240 / speed);
    return () => {
      if (interval.current !== null) window.clearInterval(interval.current);
    };
  }, [playing, points.length, speed]);

  if (loading && datasets.length === 0) {
    return <section className="panel oai-state" aria-busy="true"><h2>正在读取 OAI 数据</h2><p>校验数据来源和实验记录……</p></section>;
  }
  if (error && datasets.length === 0) {
    return <section className="panel oai-state error" role="alert"><h2>OAI 数据暂不可用</h2><p>{error}</p></section>;
  }
  if (!dataset) {
    return <section className="panel oai-state"><h2>没有可用数据集</h2><p>请先在网关的数据目录中放入经过登记的公开 OAI 数据。</p></section>;
  }

  return (
    <div className="oai-workspace" aria-busy={loading}>
      <div className="page-heading oai-page-heading">
        <div>
          <h1>公开 OAI 实验数据</h1>
          <p>离线读取 QUEST 的上行调度记录，按终端回放已核实的 SINR、MCS 与资源分配。</p>
        </div>
        <span className="oai-mode-note"><i aria-hidden="true" />离线回放</span>
      </div>

      <section className="oai-source-strip" aria-label="数据来源">
        <div>
          <span className="section-tag">来源可追溯</span>
          <strong>{dataset.name}</strong>
          <p>{dataset.environment}</p>
        </div>
        <dl>
          <div><dt>版本</dt><dd className="mono">{dataset.commit.slice(0, 8)}</dd></div>
          <div><dt>许可</dt><dd>{dataset.license}</dd></div>
          <div><dt>时间轴</dt><dd>{dataset.timeAxis}</dd></div>
        </dl>
        <a className="ghost-button oai-source-link" href={dataset.sourceUrl} target="_blank" rel="noreferrer">查看原始数据</a>
      </section>

      {error && <div className="inline-error" role="alert">{error}</div>}

      <section className="oai-control-row" aria-label="OAI 数据筛选">
        <label>数据集<select value={datasetId} onChange={(event) => setDatasetId(event.target.value)}>{datasets.filter((item) => !item.loadError).map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label>
        <label>终端 RNTI<select value={rnti} onChange={(event) => setRNTI(event.target.value)}>{dataset.ues.map((ue) => <option key={ue} value={ue}>0x{ue}</option>)}</select></label>
        <div className="oai-filter-context"><span>{number(sourceCount)} 条终端记录</span><small>{downsampled ? `图表抽取 ${number(points.length)} 点，统计使用全部记录` : '图表和统计使用全部终端记录'}</small></div>
        <a className="ghost-button oai-export-button" href={`/api/oai/datasets/${encodeURIComponent(datasetId)}/export?rnti=${encodeURIComponent(rnti)}`} download={`quest-${rnti}-records.csv`}>导出当前终端 CSV</a>
      </section>

      <OAIFlow record={current} cursor={cursor} playing={playing} />

      <div className="oai-metrics">
        <article className="oai-hero-metric"><span>当前 SINR</span><strong className="mono">{current ? number(current.sinrDb, 1) : '—'} <small>dB</small></strong><p>{current ? `帧 ${current.frame} · 时隙 ${current.slot} · 源文件第 ${current.sourceLine} 行` : '等待记录'}</p></article>
        <article><span>平均 SINR</span><strong className="mono">{number(stats.avgSinrDb, 1)} <small>dB</small></strong><p>{number(stats.count)} 条完整记录</p></article>
        <article><span>平均 MCS</span><strong className="mono">{number(stats.avgMcs, 1)}</strong><p>上行调制编码索引</p></article>
        <article><span>平均 PRB</span><strong className="mono">{number(stats.avgPrbs, 1)}</strong><p>每条调度记录</p></article>
      </div>

      <section className="panel oai-chart-panel" aria-labelledby="oai-chart-title">
        <div className="section-heading">
          <div><h2 id="oai-chart-title">SINR 记录序列</h2><p>单终端上行调度记录 · 横轴为采样后的记录顺序</p></div>
          <span className="section-tag">RNTI 0x{rnti}</span>
        </div>
        <SINRChart points={points} cursor={cursor} />
        <div className="oai-playback">
          <button className="primary-button oai-play-button" type="button" disabled={points.length < 2 || cursor >= points.length - 1} onClick={() => setPlaying((value) => !value)}>{playing ? '暂停回放' : cursor >= points.length - 1 ? '回放结束' : '开始回放'}</button>
          <button className="ghost-button" type="button" onClick={() => { setPlaying(false); setCursor(0); }}>回到起点</button>
          <label className="oai-range-label"><span>进度</span><input type="range" min="0" max={Math.max(0, points.length - 1)} value={cursor} onChange={(event) => { setPlaying(false); setCursor(Number(event.target.value)); }} /></label>
          <label className="oai-speed-label">速度<select value={speed} onChange={(event) => setSpeed(Number(event.target.value))}><option value={0.5}>0.5×</option><option value={1}>1×</option><option value={2}>2×</option></select></label>
        </div>
        <p className="flow-footnote">回放速度只改变界面推进速度，不改变原始数值。当前数据没有经过 LiteCore 的模拟注册流程。</p>
      </section>

      <section className="panel oai-table-panel" aria-labelledby="oai-table-title">
        <div className="section-heading"><div><h2 id="oai-table-title">原始记录对照</h2><p>显示所选终端前 100 条，保留原文件行号</p></div><span className="section-tag">10 个原始字段</span></div>
        <div className="table-wrap oai-table-wrap">
          <table>
            <thead><tr><th>源行</th><th>帧.时隙</th><th>SINR</th><th>MCS</th><th>PRB</th><th>传输块</th><th>待调度</th><th>HOL 时延</th><th>上行时延</th></tr></thead>
            <tbody>{records.map((record) => <tr key={record.index}><td className="mono">{record.sourceLine}</td><td className="mono">{record.frame}.{record.slot}</td><td className="mono">{number(record.sinrDb, 1)} dB</td><td className="mono">{record.mcs}</td><td className="mono">{record.prbs}</td><td className="mono">{number(record.transportBlockBytes)} B</td><td className="mono">{number(record.pendingUlBytes)} B</td><td className="mono">{number(record.holDelayMs, 3)} ms</td><td className="mono">{number(record.ulDelayMs, 3)} ms</td></tr>)}</tbody>
          </table>
        </div>
      </section>

      <p className="oai-boundary-note"><strong>数据边界：</strong>这是公开 OAI 历史实验记录。数据源没有明确证明该文件经过 USRP 空口，因此页面不标注“空口实测”；它也不代表当前 LiteCore 正在与 OAI 实时连接。</p>
    </div>
  );
}
