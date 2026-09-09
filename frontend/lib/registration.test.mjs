import assert from 'node:assert/strict';
import { test } from 'node:test';
import { simulateRegistration } from './registration.ts';

test('stable, edge and mixed examples pass the configured gate', () => {
  for (const scenario of ['stable', 'edge', 'mixed']) {
    assert.equal(simulateRegistration(' UE-test ', scenario, 1).state, 'success');
  }
});
test('weak signal is rejected without an assigned IP', () => {
  const result = simulateRegistration('UE-test', 'degrading', 2);
  assert.equal(result.state, 'rejected');
  assert.equal(result.ip, '—');
});
test('IDs are trimmed and invalid IDs rejected', () => {
  assert.equal(simulateRegistration(' UE-test ', 'stable', 1).id, 'UE-test');
  assert.throws(() => simulateRegistration(' ', 'stable', 1));
  assert.throws(() => simulateRegistration('x'.repeat(49), 'stable', 1));
});
test('sequential illustrative IPs do not duplicate across one subnet boundary', () => {
  assert.equal(simulateRegistration('UE-test', 'stable', 254).ip, '10.0.0.254');
  assert.equal(simulateRegistration('UE-test', 'stable', 255).ip, '10.0.1.1');
});
