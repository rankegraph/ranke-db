/**
 * package: core / tests
 * type:    test
 * job:     pin that every way the canvas can be blank reports why
 * limits:  headless; the overlay renders what these produce (-> ui/shell/App)
 */

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { useExplorer, defaultView } from './store.ts';
import { load, selectScope } from './session.ts';
import { useConnections } from './connections.ts';
import { clear, graph } from './graph/universe.ts';
import { forgetMembers, membersOf } from './graph/members.ts';
import { MockSource } from './data/source.ts';

/** smallMock points the built-in generator at an archive a test can afford. */
function smallMock() {
  const mock = useConnections.getState().connections.find((c) => c.kind === 'mock')!;
  useConnections.getState().updateConnection(mock.id, {
    mock: { claims: 200, seed: 3, claimsPerContribution: 10 },
  });
  useConnections.getState().activate(mock.id);
  return mock;
}

/** reset puts the session back to a fresh state between cases. */
function reset() {
  clear();
  forgetMembers();
  useExplorer.setState({
    tabs: [],
    activeTabId: null,
    notice: null,
    scopes: { state: 'unknown', scopes: [], selected: null, error: null },
  });
}

// A read that fails must say so: this is the case that looked broken.
test('a failed read reports the failure', async () => {
  reset();
  const rest = useConnections.getState().connections.find((c) => c.kind === 'rest');
  assert.ok(rest, 'no REST connection is built in');
  useConnections.getState().activate(rest.id);

  const original = globalThis.fetch;
  globalThis.fetch = (async () => {
    throw new Error('Failed to fetch');
  }) as typeof globalThis.fetch;
  try {
    await load({ limit: 10 });
  } finally {
    globalThis.fetch = original;
  }

  const notice = useExplorer.getState().notice;
  assert.equal(notice?.level, 'error', `notice = ${JSON.stringify(notice)}`);
  assert.ok((notice?.hint ?? '').length > 0, 'the failure was reported with no reason');
});

// An empty scope is a state that persists, so it has to say so. "Listed but not loaded" is
// not: selecting loads, so it resolves itself rather than sitting there blank.
test('a scope with nothing in it says so rather than drawing blank', async () => {
  smallMock();
  reset();
  useExplorer.getState().addTab(defaultView('v1', 'v1'));

  await selectScope({ name: 'no-such-branch', head: 'no-such-head', branch: 'no-such-branch' });
  const notice = useExplorer.getState().notice;
  assert.ok(notice, 'an empty scope said nothing');
  assert.match(notice.text, /no claims/i);
  assert.equal(graph().order, 0, 'an empty scope drew something');
});

// A successful read clears whatever the last failure said.
test('a read that returns claims clears the notice', async () => {
  reset();
  smallMock();
  useExplorer.getState().setNotice({ level: 'error', text: 'stale' });
  await load({ limit: 200 });
  assert.equal(useExplorer.getState().notice, null, 'a stale failure survived a good read');
});

// Selecting a scope shows it: the read is the default action, not a step to be told about.
test('selecting a scope loads it without a second action', async () => {
  reset();
  const mock = smallMock();
  const source = new MockSource(mock.mock);
  const scopes = await source.branches();
  const branch = scopes.find((s) => s.name !== '$archive')!;

  assert.equal(graph().order, 0, 'the union should start empty');
  await selectScope(branch);

  assert.ok(graph().order > 0, 'selecting a scope drew nothing — a read was not issued');
  const drawn = [...(membersOf(branch.head) ?? [])].filter((id) => graph().hasNode(id));
  assert.ok(drawn.length > 0, 'none of the scope’s claims reached the union');
  assert.equal(useExplorer.getState().notice, null, `notice = ${JSON.stringify(useExplorer.getState().notice)}`);
});
