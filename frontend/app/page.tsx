'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import { ArrowRight, Check, CircleHelp, LoaderCircle, Radio, RotateCcw, Smartphone, X } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { scenarios, simulateRegistration, type Registration, type Scenario } from '@/lib/registration';

const nodes = [
  { id: 'UE', name: '模拟终端', task: '发起注册', port: '客户端' },
  { id: 'AMF', name: '接入管理', task: '校验信道', port: ':50051' },
  { id: 'SMF', name: '会话管理', task: '分配会话', port: ':50052' },
  { id: 'UPF', name: '用户面', task: '模拟转发', port: ':50053' },
];
const bins = [{ range: '0–1', count: 12 }, { range: '1–3', count: 49 }, { range: '3–8', count: 19 }, { range: '8–13', count: 2 }, { range: '13–18', count: 18 }];
const samples = [
  { id: 'UE-000100', accepted: true, signal: '-80.70', sinr: '18.45', latency: '1.595' },
  { id: 'UE-000099', accepted: true, signal: '-83.27', sinr: '16.57', latency: '1.595' },
  { id: 'UE-000098', accepted: true, signal: '-78.03', sinr: '17.44', latency: '1.595' },
  { id: 'UE-000097', accepted: false, signal: '-115.80', sinr: '1.77', latency: '0.516' },
];

export default function Home() {
  const [scenario, setScenario] = useState<Scenario>('stable');
  const [ueId, setUeId] = useState('UE-DEMO-01');
  const [running, setRunning] = useState(false);
  const [phase, setPhase] = useState(-1);
  const [result, setResult] = useState<Registration | null>(null);
  const [records, setRecords] = useState<Registration[]>([]);
  const [error, setError] = useState('');
  const busy = useRef(false);
  const sequence = useRef(0);
  const alive = useRef(true);
  const channel = scenarios[scenario];

  useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);

  const run = useCallback(async (id: string, chosen: Scenario) => {
    if (busy.current) throw new Error('注册演示正在进行，请稍候再试。');
    const next = simulateRegistration(id, chosen, sequence.current + 1);
    sequence.current += 1;
    busy.current = true;
    setError(''); setUeId(next.id); setScenario(chosen); setRunning(true); setResult(null);
    try {
      const lastStage = next.state === 'success' ? 3 : 1;
      for (let stage = 0; stage <= lastStage; stage++) {
        if (!alive.current) throw new Error('页面已关闭');
        setPhase(stage);
        await new Promise((resolve) => setTimeout(resolve, 240));
      }
      if (!alive.current) throw new Error('页面已关闭');
      setResult(next);
      setRecords((current) => [next, ...current].slice(0, 6));
      return next;
    } finally {
      busy.current = false;
      if (alive.current) setRunning(false);
    }
  }, []);

  useEffect(() => {
    const context = (document as Document & { modelContext?: { registerTool: (tool: Record<string, unknown>, options?: { signal: AbortSignal }) => void | Promise<void> } }).modelContext;
    if (!context?.registerTool) return;
    const lifecycle = new AbortController();
    void Promise.resolve(context.registerTool({
      name: 'simulate_ue_registration', title: '模拟 UE 注册',
      description: '运行一次前端 UE 注册演示，更新流程和模拟记录，不调用真实核心网。',
      inputSchema: { type: 'object', properties: { ueId: { type: 'string', minLength: 1, maxLength: 48 }, scenario: { type: 'string', enum: Object.keys(scenarios) } }, required: ['ueId', 'scenario'], additionalProperties: false },
      annotations: { readOnlyHint: false, untrustedContentHint: false },
      execute: async (input: unknown) => {
        const values = input as { ueId?: unknown; scenario?: unknown } | null;
        if (!values || typeof values.ueId !== 'string' || typeof values.scenario !== 'string' || !Object.hasOwn(scenarios, values.scenario)) throw new Error('请提供有效的终端标识和信道场景。');
        const record = await run(values.ueId, values.scenario as Scenario);
        return { ueId: record.id, accepted: record.state === 'success', signalPowerDbm: record.signal, sinrDb: record.sinr, illustrativeIp: record.ip };
      },
    }, { signal: lifecycle.signal })).catch(() => {});
    return () => lifecycle.abort();
  }, [run]);

  return (
    <div className="app-shell">
      <a className="skip-link" href="#ue-access">跳转到注册演示</a>
      <header className="masthead">
        <a className="brand" href="#overview" aria-label="LiteCore 首页"><Radio aria-hidden="true" /><strong>LiteCore</strong><span>5G 实验室</span></a>
        <nav aria-label="页面导航"><a href="#ue-access">注册实验</a><a href="#experiment-analysis">实验结果</a><a href="#session-list">本次记录</a></nav>
        <span className="demo-label">前端演示版</span>
      </header>
      <main id="overview" className="workspace">
        <div className="page-heading"><div><h1>从一次注册，看懂核心网。</h1><p>选择信道，发起请求，跟随终端走完接入流程。</p></div><span className="context-note">UE → AMF → SMF → UPF</span></div>
        <div className="lab-workspace">
          <section className="flow-surface" aria-labelledby="flow-title">
            <div className="section-heading"><h2 id="flow-title">注册路径</h2><span>gRPC 服务架构</span></div>
            <div className="flow-summary"><span className={running ? 'state-dot working' : 'state-dot'} /><span>{running ? '正在演示：' + (nodes[phase]?.task ?? '准备请求') : result ? (result.state === 'success' ? '注册完成，会话已建立' : '请求在接入检查时停止') : '等待发起一次注册'}</span></div>
            <ol className="network-path" aria-label="注册流程">
              {nodes.map((node, index) => {
                const reached = phase >= index;
                const rejected = result?.state === 'rejected' && index === 1;
                return <li key={node.id} className={[reached ? 'reached' : '', running && phase === index ? 'current' : '', rejected ? 'stopped' : ''].join(' ')}>
                  <div className="node-symbol">{index === 0 ? <Smartphone aria-hidden="true" /> : <span>{node.id}</span>}<span className="node-result">{rejected ? <X /> : reached && (!running || phase > index) ? <Check /> : null}</span></div>
                  <strong>{node.id}</strong><span>{node.name}</span><small>{node.port}</small>
                  {index < 3 && <ArrowRight className="path-arrow" aria-hidden="true" />}
                </li>;
              })}
            </ol>
            <div className="gate-note"><CircleHelp aria-hidden="true" /><div><strong>接入的两道门槛</strong><p>信号功率 ≥ −110 dBm，且 SINR ≥ 0 dB。任意一项不满足，AMF 就会拒绝本次注册。</p></div></div>
            <div className="flow-footnote">此处展示服务关系与模拟进度，不表示后端在线状态。</div>
          </section>
          <section id="ue-access" className="register-surface" aria-labelledby="register-title">
            <div className="section-heading"><h2 id="register-title">试一次注册</h2><span>仅在浏览器中模拟</span></div>
            <form onSubmit={(event) => { event.preventDefault(); void run(ueId, scenario).catch((reason: Error) => setError(reason.message)); }}>
              <label htmlFor="ue-id">终端标识</label>
              <Input id="ue-id" className="field" value={ueId} maxLength={48} disabled={running} onChange={(event) => { setUeId(event.target.value); setError(''); }} aria-invalid={!!error} aria-describedby={error ? 'form-error' : undefined} autoComplete="off" />
              <label htmlFor="scenario">信道场景</label>
              <Select value={scenario} onValueChange={(value) => { if (value && Object.hasOwn(scenarios, value)) setScenario(value as Scenario); }} disabled={running}>
                <SelectTrigger id="scenario" className="field" aria-describedby="scenario-help"><SelectValue>{channel.label}</SelectValue></SelectTrigger>
                <SelectContent>{Object.entries(scenarios).map(([key, value]) => <SelectItem key={key} value={key}>{value.label}</SelectItem>)}</SelectContent>
              </Select>
              <p className="field-help" id="scenario-help">{channel.description}</p>
              <div className="channel-readout"><span>信号功率 <b>{channel.signal} <small>dBm</small></b></span><span>SINR <b>{channel.sinr} <small>dB</small></b></span></div>
              {error && <p id="form-error" className="form-error" role="alert">{error}</p>}
              <Button className="run-button" type="submit" disabled={running}>{running ? <LoaderCircle className="spinner" aria-hidden="true" /> : <ArrowRight aria-hidden="true" />}{running ? '注册演示进行中…' : '发起注册'}</Button>
            </form>
            <div className="result-slot" aria-live="polite" aria-atomic="true">
              {result ? <div className={'result ' + result.state}><strong>{result.state === 'success' ? <Check aria-hidden="true" /> : <X aria-hidden="true" />}{result.state === 'success' ? '注册成功' : '未通过信道检查'}</strong><p>{result.id}{result.state === 'success' ? ' · 模拟 IP ' + result.ip : ' · 可以切换到稳定信道重试。'}</p></div> : <p>{running ? '正在按顺序演示注册流程…' : '试试“弱信号”，对比一次被拒绝的接入。'}</p>}
            </div>
          </section>
        </div>
        <section id="experiment-analysis" className="analysis-section" aria-labelledby="analysis-title">
          <div className="section-heading"><div><h2 id="analysis-title">已有实验，放在一起看</h2><p>来自 benchmark.csv 的 100 条历史样本，与上方模拟独立。</p></div><span className="source-label">历史实验快照</span></div>
          <div className="analysis-grid">
            <div className="benchmark-summary"><div className="success-number"><strong>86<span>%</span></strong><span>注册成功率</span></div><div className="outcome-bar" role="img" aria-label="100 条样本中，86 条注册成功，14 条被拒绝"><span /><span /></div><div className="outcome-legend"><span>成功 86</span><span>拒绝 14</span></div><dl className="timings"><div><dt>平均时延</dt><dd>4.92 <span>ms</span></dd></div><div><dt>P95 时延</dt><dd>16.65 <span>ms</span></dd></div></dl><p className="muted-note">时延包含全部成功与失败样本。</p></div>
            <div className="distribution"><div className="chart-heading"><h3>大多数请求，在 3 ms 内结束</h3><span>样本数 / 条</span></div><div className="bar-chart" role="img" aria-label="时延分布：0到1毫秒12条，1到3毫秒49条，3到8毫秒19条，8到13毫秒2条，13到18毫秒18条。">{bins.map((bin) => <div className="bar-column" key={bin.range}><div className="bar-track"><div className={'bar ' + (bin.count === 49 ? 'bar-highlight' : '')} style={{ height: bin.count * 2 + '%' }}><span>{bin.count}</span></div></div><span>{bin.range}</span></div>)}</div><div className="chart-axis-label">注册时延 / ms</div></div>
          </div>
          <div className="sample-heading"><h3>原始样本节选</h3><span>最后 4 条记录</span></div>
          <div className="table-scroll" role="region" aria-label="历史实验样本，可横向滚动" tabIndex={0}><Table className="data-table"><TableHeader><TableRow><TableHead>终端</TableHead><TableHead>注册结果</TableHead><TableHead>信号功率 / dBm</TableHead><TableHead>SINR / dB</TableHead><TableHead className="numeric">时延 / ms</TableHead></TableRow></TableHeader><TableBody>{samples.map((sample) => <TableRow key={sample.id}><TableCell className="mono">{sample.id}</TableCell><TableCell><span className={'status-text ' + (sample.accepted ? 'success' : 'rejected')}>{sample.accepted ? <Check aria-hidden="true" /> : <X aria-hidden="true" />}{sample.accepted ? '成功' : '信道不足'}</span></TableCell><TableCell className="mono">{sample.signal}</TableCell><TableCell className="mono">{sample.sinr}</TableCell><TableCell className="numeric mono">{sample.latency}</TableCell></TableRow>)}</TableBody></Table></div>
        </section>
        <section id="session-list" className="history-section" aria-labelledby="history-title"><div className="section-heading"><div><h2 id="history-title">本次模拟记录 <span className="record-count">{records.length}</span></h2><p>保留最近 6 次，刷新页面后清空。</p></div><Button variant="ghost" className="reset-button" disabled={running || !records.length} onClick={() => { setRecords([]); setResult(null); setPhase(-1); }}><RotateCcw aria-hidden="true" />清空记录</Button></div>{records.length ? <ul className="simulation-list">{records.map((record) => <li key={record.key}><span className={'status-text ' + record.state}>{record.state === 'success' ? <Check aria-hidden="true" /> : <X aria-hidden="true" />}{record.state === 'success' ? '注册成功' : '注册被拒绝'}</span><strong>{record.id}</strong><span>{record.signal} dBm · SINR {record.sinr} dB</span><span className="mono">{record.ip === '—' ? '未分配 IP' : '模拟 IP ' + record.ip}</span></li>)}</ul> : <div className="empty-records"><span>还没有模拟记录</span><p>上方发起一次注册后，结果会留在这里，方便对比不同信道。</p></div>}</section>
        <footer><span>LiteCore · 轻量级 5G 核心网实验</span><span>历史样本是真实实验输出；交互注册是前端演示。</span></footer>
      </main>
    </div>
  );
}
