export const scenarios = {
  stable: { label: '稳定信道', description: '信号充足，适合观察完整注册流程。', signal: -82.6, sinr: 18.4 },
  edge: { label: '小区边缘', description: '接近接入门限，仍满足本次注册条件。', signal: -108.4, sinr: 1.8 },
  degrading: { label: '弱信号', description: '低于接入门限，观察 AMF 拒绝注册。', signal: -116.2, sinr: -1.2 },
  mixed: { label: '混合信道样例', description: '固定的混合信道示例，不代表随机采样。', signal: -103.1, sinr: 3.4 },
} as const;

export type Scenario = keyof typeof scenarios;
export type Registration = {
  key: number;
  id: string;
  state: 'success' | 'rejected';
  signal: number;
  sinr: number;
  ip: string;
};

export function simulateRegistration(id: string, scenario: Scenario, sequence: number): Registration {
  if (!id.trim() || id.trim().length > 48) throw new Error('请填写 1–48 个字符的终端标识。');
  if (!Object.hasOwn(scenarios, scenario)) throw new Error('请选择有效的信道场景。');
  const { signal, sinr } = scenarios[scenario];
  const accepted = signal >= -110 && sinr >= 0;
  return { key: sequence, id: id.trim(), state: accepted ? 'success' : 'rejected', signal, sinr,
    ip: accepted ? `10.0.${Math.floor((sequence - 1) / 254) % 256}.${((sequence - 1) % 254) + 1}` : '—' };
}
