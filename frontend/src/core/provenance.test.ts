/**
 * package: core / tests
 * type:    test
 * job:     pin a claim's provenance as a scope — the closure read, the repair, the picker
 * limits:  headless; no DOM, no Sigma (-> render is exercised by the render bench)
 *
 * Run with `make -C frontend test`. Node's own runner strips the types, so this needs no
 * framework, the way the other core tests already run.
 */

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { DirectedGraph } from 'graphology';

import { MockSource } from './data/source.ts';
import { forgetMembers, membersOf, setMembers } from './graph/members.ts';
import { addClaims, forgetPending, pendingCount } from './graph/build.ts';
import { apply } from './layout/layouts.ts';
import { contributionOf, depths } from './graph/shape.ts';
import { ARCHIVE_SCOPE, isProvenance, provenanceScope, scopeKey, scopeOptions } from './scope.ts';
import type { Scope } from './scope.ts';
import type { DrawnClaim } from './claims.ts';

const MOCK_PARAMS = { claims: 200, seed: 11, claimsPerContribution: 10 };

// 1.1/1.4 — a provenance scope names the claim as its head and the branch as what it is read
// within, since R-QSCOPE wants a real scope on the wire and a claim id is never one.
test('a provenance scope carries the claim as head and the branch it is read within', () => {
  const scope = provenanceScope('claim-xyz', 'main');
  assert.equal(scope.head, 'claim-xyz');
  assert.equal(scope.branch, 'main');
  assert.ok(isProvenance(scope), 'a provenance scope should be recognisable as one');
  assert.ok(!isProvenance({ name: 'main', head: 'h', branch: 'main' }), 'a branch is not provenance');
});

// 1.3 — two closures read within one branch share that branch's name, so a name cannot key
// membership: the head can, being what identifies a closure.
test('two provenance scopes hold two answers', () => {
  forgetMembers();
  const first = provenanceScope('claim-a', 'main');
  const second = provenanceScope('claim-b', 'main');
  setMembers(scopeKey(first), ['claim-a', 'anc-1']);
  setMembers(scopeKey(second), ['claim-b', 'anc-2', 'anc-3']);

  assert.equal(membersOf(scopeKey(first))?.size, 2);
  assert.equal(membersOf(scopeKey(second))?.size, 3);
  assert.ok(!membersOf(scopeKey(first))?.has('anc-2'), "one closure took the other's answer");
});

// The mock stands in for the engine, so a provenance read there answers the closure of the
// claim rather than the whole branch — otherwise the two sources disagree about one question.
test('the mock answers a provenance scope with the closure of its head', async () => {
  const source = new MockSource(MOCK_PARAMS);
  const branch = (await source.branches()).find((s) => s.name !== ARCHIVE_SCOPE)!;
  const whole = await source.scopeIds(branch);
  assert.ok(whole.length > 1, 'the fixture branch should hold several claims');

  // A claim partway down the branch: its closure is a strict subset, and holds itself.
  const claim = whole[Math.floor(whole.length / 2)];
  const closure = await source.scopeIds(provenanceScope(claim, branch.name));
  assert.ok(closure.includes(claim), 'a claim is in its own closure');
  assert.ok(closure.length <= whole.length, 'a closure cannot exceed the branch it is read within');
  for (const id of closure) assert.ok(whole.includes(id), 'the closure left the branch');
});

/** claimWith builds the least DrawnClaim the merge reads, referencing the ids given. */
function claimWith(id: string, references: string[], height = 0): DrawnClaim {
  return {
    claim: {
      id,
      type: 'derivation/statement',
      typeClass: 'derivation',
      typeSub: 'statement',
      createdAt: '2024-01-01T00:00:00Z',
      createdAtMs: 1704067200000,
      height,
      content: { kind: 'none' },
      fields: {},
      edges: references.map((reference) => ({
        reference,
        type: 'derivation/from',
        typeClass: 'derivation',
        typeSub: 'from',
        content: { kind: 'none' },
        fields: {},
        relationDirection: 0,
      })),
    },
    contribution: 1,
    branch: 'main',
    label: 'statement',
  } as unknown as DrawnClaim;
}

// 4.1 — the correctness bug the lazy read turns a statistic into: a reference dropped for want
// of its target draws the claim that stated it as an initial claim, which is the assertion the
// whole view exists to make.
test('a reference dropped for want of its target is drawn once the target arrives', () => {
  forgetPending();
  const graph = new DirectedGraph({ allowSelfLoops: false });

  addClaims(graph, [claimWith('child', ['parent'], 1)]);
  assert.equal(graph.order, 1, 'only the claim read so far is a node');
  assert.equal(graph.size, 0, 'its reference has no target yet');
  assert.equal(pendingCount(), 1, 'the reference should be held, not discarded');

  addClaims(graph, [claimWith('parent', [])]);
  assert.equal(graph.order, 2);
  assert.equal(graph.size, 1, 'the held reference was not restored when its target arrived');
  assert.ok(graph.hasDirectedEdge('child', 'parent'));
  assert.equal(pendingCount(), 0, 'nothing should still be waiting');
});

// 4.3 — height is what tells the two apart: an initial claim carries 0, so a claim above it
// with nothing drawn is a claim the session has not finished reading.
test('height distinguishes an unread claim from an initial one', () => {
  forgetPending();
  const graph = new DirectedGraph({ allowSelfLoops: false });
  addClaims(graph, [claimWith('unread', ['absent'], 3), claimWith('initial', [])]);

  assert.equal(graph.outDegree('unread'), 0, 'its reference is still waiting on a target');
  assert.equal(graph.getNodeAttribute('unread', 'height'), 3);
  assert.equal(graph.getNodeAttribute('initial', 'height'), 0);
});

// 5.2 — the picker builds from the discovered branches, which a provenance scope is never
// among, so without an entry of its own it would display a branch the view is not confined to.
test('the picker describes a provenance view rather than naming a branch', () => {
  const branches: Scope[] = [
    { name: 'main', head: 'main-head', branch: 'main' },
    { name: 'work', head: 'work-head', branch: 'work' },
  ];
  const scope = provenanceScope('claim-xyz', 'main');
  const options = scopeOptions(branches, scope);

  const selected = options.filter((o) => o.selected);
  assert.equal(selected.length, 1, 'exactly one option should read as selected');
  assert.equal(selected[0].value, scope.name);
  assert.ok(!branches.some((b) => b.name === selected[0].value), 'it named a branch instead');
});

// A branch view is still what the picker governs, so the entry it selects is that branch's.
test('the picker still selects a branch for a branch view', () => {
  const branches: Scope[] = [{ name: 'main', head: 'main-head', branch: 'main' }];
  const options = scopeOptions(branches, branches[0]);
  const selected = options.filter((o) => o.selected);
  assert.equal(selected.length, 1);
  assert.equal(selected[0].value, 'main');
});

// 4.2 — a page carrying claims already merged re-walks their references, so an unsatisfied one
// would be held again on every pass and the count the shortfall reports would climb.
test('re-merging a page does not hold the same reference twice', () => {
  forgetPending();
  const graph = new DirectedGraph({ allowSelfLoops: false });
  const page = [claimWith('child', ['parent'], 1)];

  addClaims(graph, page);
  assert.equal(pendingCount(), 1);
  addClaims(graph, page);
  addClaims(graph, page);
  assert.equal(pendingCount(), 1, 'the same unsatisfied reference was held more than once');
});

// 2 — a claim states how many references it has, so a partly-read list can say so rather than
// reading as complete.
test('a claim carries how many references it states', () => {
  forgetPending();
  const graph = new DirectedGraph({ allowSelfLoops: false });
  addClaims(graph, [claimWith('claim', ['a', 'b', 'c'], 2), claimWith('a', [])]);

  assert.equal(graph.getNodeAttribute('claim', 'references'), 3, 'the stated count should travel');
  assert.equal(graph.outDegree('claim'), 1, 'only the loaded target is drawn');
});

// 1 — the lazy read merges claims with no position; without a layout they all sit at the origin,
// which is the difference between the view working and not.
test('a layout gives merged claims positions off the origin', async () => {
  forgetPending();
  const graph = new DirectedGraph({ allowSelfLoops: false });
  addClaims(graph, [claimWith('root', ['mid'], 2), claimWith('mid', ['leaf'], 1), claimWith('leaf', [])]);

  for (const node of graph.nodes()) {
    assert.equal(graph.getNodeAttribute(node, 'x'), 0, 'a merged claim starts at the origin');
  }
  await apply(graph, 'layered', { depth: depths(graph).depth, contribution: contributionOf(graph) });
  const spread = new Set(graph.nodes().map((n) => `${graph.getNodeAttribute(n, 'x')},${graph.getNodeAttribute(n, 'y')}`));
  assert.ok(spread.size > 1, 'every claim is still at one point — the layout did not run');
});

// 2 — a branch view's scope.head is that branch's head, so the provenance of a claim that IS a
// branch head (a contribution/head claim — the case most worth inspecting) collides with it.
// Matching on the head alone would hand a branch view's layout and stretch to the layout pass.
test('a provenance view is found by kind, not by head alone', () => {
  const head = 'bciq-head';
  const branchView = { kind: 'graph', id: 'v1', scope: { name: 'main', head, branch: 'main' } };
  const provView = { kind: 'graph', id: 'v2', scope: provenanceScope(head, 'main') };
  const tabs = [branchView, provView];

  const byHeadAlone = tabs.find((t) => t.scope.head === head);
  assert.equal(byHeadAlone?.id, 'v1', 'the branch view is first, which is the trap');

  const byKind = tabs.find((t) => isProvenance(t.scope) && t.scope.head === head);
  assert.equal(byKind?.id, 'v2', 'the provenance view is the one this read belongs to');
});
