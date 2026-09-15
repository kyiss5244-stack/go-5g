import { useEffect, useRef, useState } from 'react';
import {
  apiNodeAction,
  apiNodeLogs,
  apiNodeStatus,
  type NodeId,
  type NodeStatus,
  type ServiceNodeId,
} from '../api';

const NODE_META: Record<NodeId, { name: string; port: string }> = {
  UE: { name: '模拟终端', port: '浏览器' },
  AMF: { name: '接入管理', port: ':50051' },
  SMF: { name: '会话管理', port: ':50052' },
  UPF: { name: '用户面', port: ':50053' },
};

type Props = {
  activeNode: NodeId | null;
  mode: 'probing' | 'live' | 'demo';
  ueRunning: boolean;
  ueLogs: string[];
  onSelect: (node: NodeId) => void;
  onClose: () => void;
  onStopUE: () => void;
};

export function NodeTerminal({ activeNode, mode, ueRunning, ueLogs, onSelect, onClose, onStopUE }: Props) {
  const [logs, setLogs] = useState<Record<ServiceNodeId, string[]>>({ AMF: [], SMF: [], UPF: [] });
  const [statuses, setStatuses] = useState<Partial<Record<ServiceNodeId, NodeStatus>>>({});
  const [error, setError] = useState('');
  const [actionBusy, setActionBusy] = useState(false);
  const [autoScroll, setAutoScroll] = useState(true);
  const closeRef = useRef<HTMLButtonElement>(null);
  const outputRef = useRef<HTMLDivElement>(null);
  const dialogRef = useRef<HTMLElement>(null);
  const previousFocus = useRef<HTMLElement | null>(null);

  useEffect(() => setError(''), [activeNode]);

  const isOpen = activeNode !== null;
  useEffect(() => {
    if (!isOpen) return;
    previousFocus.current = document.activeElement as HTMLElement | null;
    document.body.classList.add('terminal-open');
    window.setTimeout(() => closeRef.current?.focus(), 0);
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose();
      if (event.key === 'Tab') {
        const focusable = dialogRef.current?.querySelectorAll<HTMLElement>('button:not(:disabled)');
        if (!focusable || focusable.length === 0) return;
        const first = focusable[0];
        const last = focusable[focusable.length - 1];
        if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus(); }
        if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); }
      }
    };
    window.addEventListener('keydown', onKeyDown);
    return () => {
      document.body.classList.remove('terminal-open');
      window.removeEventListener('keydown', onKeyDown);
      previousFocus.current?.focus();
    };
  }, [isOpen, onClose]);

  useEffect(() => {
    if (!activeNode || activeNode === 'UE' || mode !== 'live') return;
    let cancelled = false;
    const refresh = async () => {
      try {
        const [nextStatus, nextLogs] = await Promise.all([apiNodeStatus(activeNode), apiNodeLogs(activeNode)]);
        if (cancelled) return;
        setStatuses((prev) => ({ ...prev, [activeNode]: nextStatus }));
        setLogs((prev) => ({ ...prev, [activeNode]: nextLogs.lines.slice(-2000) }));
        setError('');
      } catch (cause) {
        if (!cancelled) setError(cause instanceof Error ? cause.message : '读取网元信息失败');
      }
    };
    void refresh();
    const timer = window.setInterval(() => void refresh(), 1500);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [activeNode, mode]);

  const visibleLogs = activeNode === 'UE' ? ueLogs : activeNode ? logs[activeNode] : [];

  useEffect(() => {
    if (autoScroll) outputRef.current?.scrollTo({ top: outputRef.current.scrollHeight, behavior: 'smooth' });
  }, [visibleLogs, autoScroll]);

  if (!activeNode) return null;
  const meta = NODE_META[activeNode];
  const serviceStatus = activeNode === 'UE' ? null : statuses[activeNode];
  const running = activeNode === 'UE' ? ueRunning : !!serviceStatus?.running;
  const statusLabel = mode === 'demo' ? '演示模式' : activeNode === 'UE' ? (ueRunning ? '请求进行中' : '等待请求') : serviceStatus ? (serviceStatus.running ? '运行中' : '已停止') : error ? '状态未知' : '读取中';

  const handleAction = async () => {
    if (activeNode === 'UE') {
      onStopUE();
      return;
    }
    setActionBusy(true);
    setError('');
    try {
      const next = await apiNodeAction(activeNode, running ? 'stop' : 'start');
      setStatuses((prev) => ({ ...prev, [activeNode]: next }));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '服务操作失败');
    } finally {
      setActionBusy(false);
    }
  };

  const copyLogs = async () => {
    try {
      await navigator.clipboard.writeText(visibleLogs.join('\n'));
    } catch {
      setError('浏览器不允许复制，请在日志区手动选择。');
    }
  };

  return (
    <div className="terminal-backdrop" role="presentation" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <section ref={dialogRef} className="node-terminal" role="dialog" aria-modal="true" aria-labelledby="terminal-title">
        <header className="terminal-header">
          <div>
            <span className="terminal-path mono">litecore / {activeNode.toLowerCase()}</span>
            <h2 id="terminal-title">{activeNode} · {meta.name}</h2>
          </div>
          <div className="terminal-heading-actions">
            <span className={`terminal-state ${running ? 'running' : ''}`}><i aria-hidden="true" />{statusLabel}</span>
            <button ref={closeRef} className="terminal-close" type="button" onClick={onClose} aria-label="关闭终端">关闭</button>
          </div>
        </header>

        <nav className="terminal-tabs" aria-label="切换网元终端">
          {(Object.keys(NODE_META) as NodeId[]).map((node) => (
            <button key={node} type="button" className={node === activeNode ? 'active' : ''} onClick={() => { setError(''); onSelect(node); }}>
              {node}<small>{NODE_META[node].port}</small>
            </button>
          ))}
        </nav>

        <div className="terminal-toolbar">
          <p>{activeNode === 'UE' ? '这里记录浏览器发出的注册请求和网关返回。' : `正在读取 ${serviceStatus?.container ?? activeNode.toLowerCase()} 的最近 500 条真实容器日志。`}</p>
          <div>
            <button type="button" onClick={() => setAutoScroll((value) => !value)}>{autoScroll ? '暂停滚动' : '继续滚动'}</button>
            <button type="button" onClick={() => void copyLogs()} disabled={visibleLogs.length === 0}>复制日志</button>
          </div>
        </div>

        <div ref={outputRef} className="terminal-output mono" role="log" aria-live="polite">
          {visibleLogs.length > 0 ? visibleLogs.map((line, index) => <div key={`${index}-${line}`}>{line}</div>) : (
            <p>{mode === 'demo' && activeNode !== 'UE' ? '演示模式不能读取 Docker 容器日志。' : activeNode === 'UE' ? '发起注册后，请求与返回会显示在这里。' : '等待该节点产生日志……'}</p>
          )}
        </div>

        <footer className="terminal-footer">
          <div>
            <strong>{visibleLogs.length.toLocaleString()} 行</strong>
            {visibleLogs.length >= 500 && <span>仅显示最近 500 条容器日志</span>}
          </div>
          <div className="terminal-control">
            <span>{activeNode === 'UE' ? '停止只取消当前注册请求。' : '停止会中断服务；重新启动后，该网元内存状态会清空。'}</span>
            <button
              className={running ? 'danger' : 'start'}
              type="button"
              disabled={actionBusy || mode !== 'live' || (activeNode === 'UE' ? !ueRunning : !serviceStatus)}
              onClick={() => void handleAction()}
            >
              {actionBusy ? '正在执行…' : activeNode === 'UE' ? '停止当前任务' : running ? `停止 ${activeNode}` : `启动 ${activeNode}`}
            </button>
          </div>
        </footer>
        {error && <p className="terminal-error" role="alert">{error}</p>}
      </section>
    </div>
  );
}
