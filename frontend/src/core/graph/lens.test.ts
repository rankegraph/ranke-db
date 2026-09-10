/**
 * package: core / tests
 * type:    test
 * job:     pin what a lens contains, and that the union is untouched by cutting one
 * limits:  the graph only; which Sigma draws it is the renderer's
 */

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { DirectedGraph } from 'graphology';

import { OUTSIDE_ATTR, covers, lensOf, windowAround } from './lens.ts';

/** a row of claims at x = 0, 10, 20 … with each citing the one before. */
function chain(n: number): DirectedGraph {
  const g = new DirectedGraph({ allowSelfLoops: false });
  for (let i = 0; i < n; i++) g.addNode(`c${i}`, { x: i * 10, y: 0, cls: 'source', size: 3 });
  for (let i = 1; i < n; i++) g.addDirectedEdgeWithKey(`e${i}`, `c${i}`, `c${i - 1}`, { claimType: 'derivation/input' });
  return g;
}

test('a lens holds the window and nothing beyond it', () => {
  const union = chain(20);
  const lens = lensOf(union, { x0: 50, x1: 90 });

  assert.equal(lens.inside, 5, 'c5…c9 lie in 50…90');
  for (const node of ['c5', 'c6', 'c7', 'c8', 'c9']) {
    assert.ok(lens.graph.hasNode(node), `${node} should be in the lens`);
  }
  assert.ok(!lens.graph.hasNode('c12'), 'a claim well outside the window came along');
  assert.ok(lens.graph.order < union.order, 'the lens is not smaller than the union');
});

// The lens is for the eye; the archive is the union, and cutting one must not touch it.
test('cutting a lens leaves the union untouched', () => {
  const union = chain(20);
  const order = union.order;
  const size = union.size;
  const x = union.getNodeAttribute('c7', 'x');

  const lens = lensOf(union, { x0: 50, x1: 90 });
  lens.graph.setNodeAttribute('c7', 'x', 999);

  assert.equal(union.order, order);
  assert.equal(union.size, size);
  assert.equal(union.getNodeAttribute('c7', 'x'), x, 'moving a claim in the lens moved it in the union');
});

// Following a leaving edge to its far end was the first design; measuring it found stubs
// outnumbering real claims two to one, since provenance reaches back arbitrarily far.
test('an edge leaving the window is counted on the claim, not followed', () => {
  const union = chain(20);
  const lens = lensOf(union, { x0: 50, x1: 90 });

  assert.ok(!lens.graph.hasNode('c4'), 'the far end of a leaving edge was pulled in');
  assert.ok(!lens.graph.hasEdge('e5'), 'an edge reaching out of the view was drawn');
  assert.equal(lens.graph.order, lens.inside, 'the lens is exactly the window');

  // c5 references c4 below the window; c9 is referenced by c10 above it. Each knows.
  assert.equal(lens.graph.getNodeAttribute('c5', OUTSIDE_ATTR), 1);
  assert.equal(lens.graph.getNodeAttribute('c9', OUTSIDE_ATTR), 1);
  // A claim wholly inside references nothing out of view.
  assert.equal(lens.graph.getNodeAttribute('c7', OUTSIDE_ATTR), 0);
  assert.equal(lens.leaving, 2);
});

test('an edge with neither end in the window is left out', () => {
  const union = chain(20);
  const lens = lensOf(union, { x0: 50, x1: 90 });
  // e15 joins c15 and c14, both far outside.
  assert.ok(!lens.graph.hasEdge('e15'));
  assert.ok(!lens.graph.hasNode('c15'));
});

test('a window is widened so a small pan needs no new lens', () => {
  const window = windowAround(100, 200, 0.5);
  assert.equal(window.x0, 50);
  assert.equal(window.x1, 250);
  assert.ok(covers(window, 100, 200), 'the window should hold the viewport it was made for');
  assert.ok(covers(window, 60, 240), 'a pan within the margin should need no rebuild');
  assert.ok(!covers(window, 40, 240), 'a pan past the margin needs a new lens');
  // Given backwards, it still yields an ordered window.
  assert.deepEqual(windowAround(200, 100, 0), { x0: 100, x1: 200 });
});

test('an empty window yields an empty lens rather than failing', () => {
  const lens = lensOf(chain(20), { x0: 10_000, x1: 20_000 });
  assert.equal(lens.inside, 0);
  assert.equal(lens.graph.order, 0);
});
