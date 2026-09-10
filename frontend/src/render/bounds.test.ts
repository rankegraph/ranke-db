/**
 * package: render / tests
 * type:    test
 * job:     pin the one bound — that neither zoom nor pan can leave the viewport off the graph
 * limits:  arithmetic only; no camera and no DOM
 */

import { test } from 'node:test';
import assert from 'node:assert/strict';

import {
  MAX_NODE_GROWTH,
  MAX_OUTSIDE,
  MIN_COVER,
  RATIO_FLOOR,
  covered,
  fillRatio,
  fillStretch,
  hold,
  holdRange,
  needed,
  ratioCeiling,
  ratioCeilingBoth,
  ratioFloor,
  stretchFloor,
} from './bounds.ts';

/** A 1000 px viewport, so a share reads directly as pixels. */
const VIEWPORT = 1000;

// What the bound holds is the emptiness at an edge, never the graph: a reader zoomed in pushes
// most of the picture off the canvas on purpose, and is only stopped from panning into nothing.
test('a graph past the viewport may hang off either edge, up to the empty share', () => {
  const size = 2000; // larger than the viewport, as a stretched axis or a fitted band is
  const { min, max } = holdRange(size, VIEWPORT);
  for (const start of [min, max, (min + max) / 2]) {
    assert.ok(
      covered(start, size, VIEWPORT) >= MIN_COVER * VIEWPORT - 1e-6,
      `a near edge at ${start} left only ${covered(start, size, VIEWPORT)} px on graph`,
    );
  }
  // The two ends are the same margin at opposite edges, which is what makes them symmetric.
  assert.equal(max, MAX_OUTSIDE * VIEWPORT, 'this much may be empty before the near edge');
  assert.equal(size + min, MIN_COVER * VIEWPORT, 'and this much reaches past the far one');
  assert.ok(covered(min - 1, size, VIEWPORT) < MIN_COVER * VIEWPORT);
  assert.ok(covered(max + 1, size, VIEWPORT) < MIN_COVER * VIEWPORT);
});

// A fit fills the viewport exactly, and is then free to move like anything else its size: the
// far edge may lift off the far border, which is a margin the reader asked for by dragging.
test('a graph the size of the viewport keeps its margin at either edge', () => {
  assert.deepEqual(holdRange(VIEWPORT, VIEWPORT), {
    min: -MAX_OUTSIDE * VIEWPORT,
    max: MAX_OUTSIDE * VIEWPORT,
  });
  assert.equal(hold(500, VIEWPORT, VIEWPORT), -100, 'pushed past the empty share, not before it');
  assert.equal(hold(300, VIEWPORT, VIEWPORT), 0, 'a margin the bound allows was taken back');
});

test('a graph smaller than the viewport is only asked to stay wholly in view', () => {
  const size = 200; // well under MIN_COVER of the viewport
  assert.equal(needed(size, VIEWPORT), size);
  const { min, max } = holdRange(size, VIEWPORT);
  assert.equal(min, 0, 'its near edge may not go past the near edge of the viewport');
  assert.equal(max, VIEWPORT - size, 'nor its far edge past the far one');
});

// Taking `needed` at whichever of its two branches applies, the range has room at both: the
// containment branch leaves the slack the graph does not fill, and the coverage branch leaves
// what may hang off each edge. So hold never has an inverted range to defend against.
test('the range holds an edge rather than inverting, at any size against any viewport', () => {
  for (const viewport of [1, 250, 1000]) {
    for (const size of [0.01, 0.5, MIN_COVER, 0.99, 1, 2, 50].map((f) => f * viewport)) {
      const { min, max } = holdRange(size, viewport);
      assert.ok(min < max, `${size} px of graph in ${viewport} px inverted to [${min}, ${max}]`);
    }
  }
});

test('hold moves an out-of-bounds edge back and leaves an in-bounds one alone', () => {
  const size = 2000;
  const { min, max } = holdRange(size, VIEWPORT);
  assert.equal(hold(min - 300, size, VIEWPORT), 300, 'a graph panned off to the left');
  assert.equal(hold(max + 300, size, VIEWPORT), -300, 'a graph panned off to the right');
  assert.equal(hold((min + max) / 2, size, VIEWPORT), 0, 'a graph already in view was moved');
});

test('the ratio ceiling is where the graph has shrunk to exactly the bound', () => {
  // Drawn at 1200 px with ratio 1, so it may grow to 1200 / 600 = 2 before it covers only 60%.
  const ceiling = ratioCeiling(1200, 1, VIEWPORT);
  assert.equal(ceiling, 2);
  // Size runs inversely with ratio, so at the ceiling the graph covers the bound exactly.
  assert.equal(1200 / ceiling, MIN_COVER * VIEWPORT);
});

// The width alone is what held a one-instant archive at a thousandfold zoom: every claim shares
// an x, so the picture is a column a node wide, and asking that width to cover its share of the
// canvas set a ceiling of about a thousandth — which Sigma's camera then clamped the ratio to.
test('the ceiling over both axes is the one the better-covered axis allows', () => {
  const canvas = { width: 800, height: 650 };
  // A column: 1 px wide, the full height. The height's ceiling is the one that means anything.
  const column = ratioCeilingBoth({ width: 1, height: 650 }, 1, canvas) as number;
  assert.equal(column, ratioCeiling(650, 1, canvas.height));
  assert.ok(column > 1, `a picture filling the height was held at ${column}`);
  // A wide, shallow band is the same case turned on its side.
  const band = ratioCeilingBoth({ width: 800, height: 4 }, 1, canvas) as number;
  assert.equal(band, ratioCeiling(800, 1, canvas.width));
  // With neither axis degenerate, the looser is still the answer, and a measurement that says
  // nothing on one axis leaves the other to answer alone.
  assert.equal(ratioCeilingBoth({ width: 800, height: 325 }, 1, canvas), band);
  assert.equal(ratioCeilingBoth({ width: 0, height: 650 }, 1, canvas), column);
  assert.equal(ratioCeilingBoth({ width: 0, height: 0 }, 1, canvas), null);
});

// The bound from the other end: a camera deep enough draws each claim over its neighbours, so
// what the reader sees is one disc. Stated as growth, since that is what a reader sees of it.
test('the ratio floor is where a node reaches the growth it is allowed', () => {
  assert.equal(1 / Math.sqrt(RATIO_FLOOR), MAX_NODE_GROWTH);
  assert.equal(ratioFloor(null), RATIO_FLOOR);
  assert.equal(ratioFloor(2), RATIO_FLOOR, 'a ceiling above the floor leaves the floor alone');
  // Sigma refuses a floor above the ceiling, and a picture that small is already held in place.
  assert.equal(ratioFloor(0.001), 0.001);
});

test('the stretch floor is the same bound solved the other way round', () => {
  // Drawn at 1200 px at stretch 1: compressing to half of that lands exactly on the bound.
  const floor = stretchFloor(1200, 1, VIEWPORT);
  assert.equal(floor, 0.5);
  assert.equal(1200 * floor, MIN_COVER * VIEWPORT);
});

// Fitting is the same arithmetic as the bound, asked for the whole viewport rather than the
// bound's share of it — which is what fit-height did wrong while it sent a fixed ratio.
test('the fitting ratio brings a drawn span to exactly the viewport, either way round', () => {
  // A picture drawn at 400 px must grow to 1000: the camera zooms in, so its ratio falls.
  const short = fillRatio(400, 1, VIEWPORT) as number;
  assert.equal(short, 0.4);
  assert.equal(400 / short, VIEWPORT);
  // And one drawn at 2500 px must shrink to it, from whatever ratio it was measured at.
  const tall = fillRatio(2500, 2, VIEWPORT) as number;
  assert.equal(tall, 5);
  assert.equal((2500 * 2) / tall, VIEWPORT);
});

// The strata are fitted by stretching them, since the camera would magnify the claims with them.
test('the fitting stretch brings a drawn span to exactly the viewport, either way round', () => {
  // Drawn 20 px tall at stretch 1 — a band of strata against a wide axis, which is the case the
  // fit exists for. Filling 1000 px asks for fifty times the room.
  const band = fillStretch(20, 1, VIEWPORT) as number;
  assert.equal(band, 50);
  assert.equal(20 * band, VIEWPORT);
  // Size runs with the stretch, so a picture already taller than the viewport comes back down.
  const tall = fillStretch(4000, 4, VIEWPORT) as number;
  assert.equal(tall, 1);
  assert.equal((4000 / 4) * tall, VIEWPORT);
});

test('a measurement that says nothing yields no bound rather than a wrong one', () => {
  for (const bad of [0, -1]) {
    assert.equal(ratioCeiling(bad, 1, VIEWPORT), null);
    assert.equal(ratioCeiling(1200, bad, VIEWPORT), null);
    assert.equal(ratioCeiling(1200, 1, bad), null);
    assert.equal(stretchFloor(bad, 1, VIEWPORT), null);
    assert.equal(fillRatio(bad, 1, VIEWPORT), null);
    assert.equal(fillRatio(1200, bad, VIEWPORT), null);
    assert.equal(fillRatio(1200, 1, bad), null);
    assert.equal(fillStretch(bad, 1, VIEWPORT), null);
    assert.equal(fillStretch(1200, bad, VIEWPORT), null);
    assert.equal(fillStretch(1200, 1, bad), null);
  }
});

// The two instruments moved the picture under different limits, which is what let a load land
// where the wheel could not follow. Reaching the bound either way must give the same picture.
test('reaching the bound by camera and by stretch leaves the same drawn width', () => {
  const drawn = 1200;
  const byCamera = drawn / (ratioCeiling(drawn, 1, VIEWPORT) as number);
  const byStretch = drawn * (stretchFloor(drawn, 1, VIEWPORT) as number);
  assert.equal(byCamera, byStretch);
  assert.equal(byCamera, MIN_COVER * VIEWPORT);
});
