// 与 gateway（:8080）的 HTTP 接口约定。字段命名与 gateway 的 JSON 一一对应。

export type Scenario = 'stable' | 'edge' | 'degrading' | 'mixed';

export const SCENARIOS: Record<Scenario, { label: string; description: string }> = {
  stable: { label: '稳定信道', description: '信号充足，适合观察完整注册流程。' },
  edge: { label: '小区边缘', description: '接近接入门限，多数样本仍满足本次注册条件。' },
  degrading: { label: '弱信号', description: '低于接入门限，观察 AMF 拒绝注册。' },
  mixed: { label: '混合信道', description: '70% 稳定、20% 边缘、10% 恶化，每次随机。' },
};

export type RegisterResult = {
  success: boolean;
  ueId: string;
  amfUeId?: string;
  sessionId?: string;
  ueIp?: string;
  signalPower: number;
  sinr: number;
  retries: number;
  processingMs: number;
  gatewayMs: number;
  message: string;
  inputSource?: 'oai';
  datasetId?: string;
  recordIndex?: number;
  sourceLine?: number;
  rnti?: string;
};

export type UEEntry = {
  ueId: string;
  amfUeId?: string;
  sessionId?: string;
  ueIp?: string;
  amfState?: string;
  sessionState?: string;
  ruleId?: string;
  packetsForwarded: number;
  active: boolean;
  registeredAt?: string;
};

export type ServiceStatus = 'SERVING' | 'NOT_SERVING' | 'UNREACHABLE';
export type ServiceHealth = { name: string; address: string; status: ServiceStatus };
export type Health = { gateway: string; checkedAt: string; services: ServiceHealth[] };

export type DeregisterResult = { success: boolean; ueId: string; message: string; releasedIp?: string };

const API_BASE = (import.meta.env.VITE_API_BASE as string | undefined) ?? 'http://localhost:8080';

export class ApiError extends Error {}

async function request<T>(path: string, init?: RequestInit, timeoutMs = 6000): Promise<T> {
  const controller = new AbortController();
  let timedOut = false;
  const externalSignal = init?.signal;
  const forwardAbort = () => controller.abort();
  externalSignal?.addEventListener('abort', forwardAbort, { once: true });
  const timer = window.setTimeout(() => {
    timedOut = true;
    controller.abort();
  }, timeoutMs);
  let data: T & { error?: string };
  try {
    const response = await fetch(`${API_BASE}${path}`, { ...init, signal: controller.signal });
    data = (await response.json()) as T & { error?: string };
  } catch (cause) {
    const aborted = cause instanceof DOMException && cause.name === 'AbortError';
    throw new ApiError(aborted ? (timedOut ? '请求超时，网关没有回应' : '注册请求已停止') : '连不上网关（:8080）');
  } finally {
    window.clearTimeout(timer);
    externalSignal?.removeEventListener('abort', forwardAbort);
  }
  if (data && typeof data === 'object' && typeof data.error === 'string') {
    throw new ApiError(data.error);
  }
  return data;
}

export function apiHealth(): Promise<Health> {
  return request<Health>('/api/health', undefined, 2500);
}

// 注册在网关内部可能带重试（最多 2 次，每次最长 4 秒），给足超时。
export type OAIRegisterInput = {
  datasetId: string;
  recordIndex: number;
  signalPower: number;
};

export function apiRegister(ueId: string, scenario: Scenario, signal?: AbortSignal, oai?: OAIRegisterInput): Promise<RegisterResult> {
  return request<RegisterResult>(
    '/api/register',
    { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ ueId, scenario, oai }), signal },
    20000,
  );
}

export type NodeId = 'UE' | 'AMF' | 'SMF' | 'UPF';
export type ServiceNodeId = Exclude<NodeId, 'UE'>;
export type NodeStatus = {
  service: string;
  container: string;
  status: string;
  running: boolean;
  startedAt?: string;
  finishedAt?: string;
};

export function apiNodeStatus(node: ServiceNodeId): Promise<NodeStatus> {
  return request<NodeStatus>(`/api/nodes/${node.toLowerCase()}/status`, undefined, 4000);
}

export function apiNodeLogs(node: ServiceNodeId): Promise<{ service: string; lines: string[] }> {
  return request<{ service: string; lines: string[] }>(`/api/nodes/${node.toLowerCase()}/logs`, undefined, 5000);
}

export function apiNodeAction(node: ServiceNodeId, action: 'start' | 'stop'): Promise<NodeStatus> {
  return request<NodeStatus>(`/api/nodes/${node.toLowerCase()}/${action}`, { method: 'POST' }, 15000);
}

export function apiDeregister(ueId: string): Promise<DeregisterResult> {
  return request<DeregisterResult>(
    '/api/deregister',
    { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ ueId }) },
    10000,
  );
}

export function apiUEs(): Promise<{ ues: UEEntry[] }> {
  return request<{ ues: UEEntry[] }>('/api/ues', undefined, 10000);
}

export type OAIDataset = {
  id: string;
  name: string;
  source: string;
  sourceUrl: string;
  sourceSha256: string;
  commit: string;
  environment: string;
  license: string;
  timeAxis: string;
  recordCount: number;
  ues: string[];
  minSinrDb: number;
  maxSinrDb: number;
  loadError?: string;
};

export type OAIRecord = {
  index: number;
  sourceLine: number;
  frame: number;
  slot: number;
  rnti: string;
  sinrDb: number;
  transportBlockBytes: number;
  mcs: number;
  prbs: number;
  pendingUlBytes: number;
  holDelayMs: number;
  ulDelayMs: number;
};

export type OAIStats = {
  count: number;
  avgSinrDb: number;
  avgMcs: number;
  avgPrbs: number;
};

export function apiOAIDatasets(): Promise<{ datasets: OAIDataset[] }> {
  return request<{ datasets: OAIDataset[] }>('/api/oai/datasets', undefined, 5000);
}

export function apiOAIRecords(datasetId: string, rnti: string, limit = 100): Promise<{ records: OAIRecord[]; total: number }> {
  const query = new URLSearchParams({ rnti, limit: String(limit) });
  return request<{ records: OAIRecord[]; total: number }>(`/api/oai/datasets/${encodeURIComponent(datasetId)}/records?${query}`, undefined, 5000);
}

export function apiOAISeries(datasetId: string, rnti: string): Promise<{ points: OAIRecord[]; sourceCount: number; downsampled: boolean; stats: OAIStats }> {
  const query = new URLSearchParams({ rnti, maxPoints: '600' });
  return request<{ points: OAIRecord[]; sourceCount: number; downsampled: boolean; stats: OAIStats }>(`/api/oai/datasets/${encodeURIComponent(datasetId)}/series?${query}`, undefined, 5000);
}
