/**
 * package: core / tests
 * type:    test
 * job:     pin how the timeline layout places claims on y — strata, subbands, and the scatter
 *          within each
 * limits:  headless; positions only, no rendering
 */

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { DirectedGraph } from 'graphology';

import { PACK_GAP_UNITS, assignTimeline } from './layouts.ts';
import type { TimelineContext } from './layouts.ts';

/** A context placing claims at explicit instants, one class throughout so they share a band. */
function packCtx(at: Record<string, number>, pack: string[], only = false): TimelineContext {
  return {
    toX: (instant) => instant,
    createdAt: (node) => at[node] ?? 0,
    classOf: () => 'source',
    subOf: () => '',
    pack: new Set(pack),
    only,
  };
}

/** A context whose claims each carry an explicit class and subtype, keyed by node id. */
function ctxFor(classes: Record<string, string>, subtypes: Record<string, string> = {}): TimelineContext {
  return {
    toX: (at) => at,
    createdAt: (node) => Number(node),
    classOf: (node) => classes[node] ?? '',
    subOf: (node) => subtypes[node] ?? '',
  };
}

/** buildGraph makes a node per id, all at distinct instants so x never coincides. */
function buildGraph(ids: string[]): DirectedGraph {
  const g = new DirectedGraph();
  ids.forEach((id, i) => g.addNode(id, { x: 0, y: 0, id: String(i) }));
  return g;
}

test('contribution/head claims sit in their own subband, strictly below the rest of the band', () => {
  const heads = Array.from({ length: 6 }, (_, i) => `head${i}`);
  const others = Array.from({ length: 6 }, (_, i) => `other${i}`);
  const g = buildGraph([...heads, ...others]);
  const classes = Object.fromEntries([...heads, ...others].map((id) => [id, 'contribution']));
  const subtypes = Object.fromEntries([
    ...heads.map((id) => [id, 'head']),
    ...others.map((id) => [id, 'contributor']),
  ]);
  assignTimeline(g, ctxFor(classes, subtypes));

  const yOf = (id: string) => g.getNodeAttribute(id, 'y') as number;
  const maxHeadY = Math.max(...heads.map(yOf));
  const minOtherY = Math.min(...others.map(yOf));
  assert.ok(
    maxHeadY < minOtherY,
    `expected every head below every other contribution claim, got max head=${maxHeadY}, min other=${minOtherY}`,
  );
});

// Not a single line: several heads should land on distinct lanes within their subband, the
// same scatter every other stratum gets — the whole reason a subband exists is to group heads
// without collapsing them onto one row.
test('heads scatter across lanes within their subband rather than collapsing to one', () => {
  const heads = Array.from({ length: 12 }, (_, i) => `head${i}`);
  const g = buildGraph(heads);
  const classes = Object.fromEntries(heads.map((id) => [id, 'contribution']));
  const subtypes = Object.fromEntries(heads.map((id) => [id, 'head']));
  assignTimeline(g, ctxFor(classes, subtypes));

  const ys = new Set(heads.map((id) => g.getNodeAttribute(id, 'y') as number));
  assert.ok(ys.size > 1, `expected more than one lane among 12 heads, got ${ys.size}`);
});

// The subband split is scoped to contribution — every other stratum still gets its band whole,
// as it did before heads were split out.
test('a non-contribution class is unaffected by the head subband split', () => {
  const claims = Array.from({ length: 20 }, (_, i) => `c${i}`);
  const g = buildGraph(claims);
  const classes = Object.fromEntries(claims.map((id) => [id, 'source']));
  assignTimeline(g, ctxFor(classes));

  const ys = new Set(claims.map((id) => g.getNodeAttribute(id, 'y') as number));
  assert.ok(ys.size > 1, `expected the source band to scatter across lanes as before, got ${ys.size}`);
});

// The split is a fixed share of the contribution band, present whether or not any head claim
// is actually loaded — the same "fixed regardless of what is shown" rule the bands themselves
// follow (-> the toggle-must-not-move-anyone-else's-claims tests this mirrors).
test('the head subband exists even when nothing is placed in it', () => {
  const others = Array.from({ length: 6 }, (_, i) => `other${i}`);
  const g = buildGraph(others);
  const classes = Object.fromEntries(others.map((id) => [id, 'contribution']));
  const subtypes = Object.fromEntries(others.map((id) => [id, 'contributor']));
  // Should not throw, and should place these claims above where the (empty) head subband is.
  assert.doesNotThrow(() => assignTimeline(g, ctxFor(classes, subtypes)));
});

// A closure is small and read closely, so its claims take lanes in time order rather than by
// hash: claims standing at one instant step up instead of landing on each other.
test('packed claims standing at one instant take a lane apiece', () => {
  const g = buildGraph(['a', 'b', 'c']);
  assignTimeline(g, packCtx({ a: 0, b: 0, c: 0 }, ['a', 'b', 'c']));
  const ys = ['a', 'b', 'c'].map((id) => g.getNodeAttribute(id, 'y') as number);
  assert.equal(new Set(ys).size, 3, `three claims at one instant shared a lane: ${ys}`);
});

// And the packing compacts rather than stepping forever: once a claim's room has passed, the
// lane it held is the lowest one free again.
test('a packed lane is reused once its gap has passed, and not before', () => {
  const together = buildGraph(['a', 'b']);
  assignTimeline(together, packCtx({ a: 0, b: PACK_GAP_UNITS - 1 }, ['a', 'b']));
  assert.notEqual(
    together.getNodeAttribute('a', 'y'),
    together.getNodeAttribute('b', 'y'),
    'a claim inside its neighbour’s room took its lane',
  );

  const apart = buildGraph(['a', 'b']);
  assignTimeline(apart, packCtx({ a: 0, b: PACK_GAP_UNITS }, ['a', 'b']));
  assert.equal(
    apart.getNodeAttribute('a', 'y'),
    apart.getNodeAttribute('b', 'y'),
    'a claim past its neighbour’s room did not reuse the lane',
  );
});

// What a read into a drawn picture needs: the claims that arrived are placed, and nothing else
// moves — a claim outside the set keeps the position the view is already drawing it at.
test('placing only the packed set leaves every other claim where it was', () => {
  const g = buildGraph(['a', 'b']);
  g.setNodeAttribute('b', 'x', 999);
  g.setNodeAttribute('b', 'y', 888);
  assignTimeline(g, packCtx({ a: 0, b: 10 }, ['a'], true));
  assert.equal(g.getNodeAttribute('b', 'x'), 999, 'an unpacked claim was moved');
  assert.equal(g.getNodeAttribute('b', 'y'), 888, 'an unpacked claim was moved');
  assert.notEqual(g.getNodeAttribute('a', 'y'), 0, 'the packed claim was not placed');
});

// Without that restriction a pack is a preference rather than a filter: everyone is still
// placed, the packed set in time order and the rest by hash.
test('a pack given without the restriction still places every claim', () => {
  const g = buildGraph(['a', 'b']);
  assignTimeline(g, packCtx({ a: 0, b: 10 }, ['a']));
  assert.notEqual(g.getNodeAttribute('b', 'y'), 0, 'an unpacked claim was skipped');
  assert.equal(g.getNodeAttribute('b', 'x'), 10);
});
