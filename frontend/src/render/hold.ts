/**
 * package: render / hold
 * type:    logic
 * job:     keep the camera inside the bound, whichever instrument moved the picture
 * limits:  acts on the camera it is handed; which instance is showing is the renderer's
 *          (-> render/renderer), and the arithmetic is bounds' (-> render/bounds)
 *
 * A limit on the stretch or on the camera alone is one the other walks past, so everything here
 * works from the drawn graph: zooming out too far and panning off the edge are one violation.
 */

import type Sigma from 'sigma';
import { fillStretch, hold, ratioCeilingBoth } from './bounds.ts';

export interface Extent {
  x0: number;
  x1: number;
  y0: number;
  y1: number;
}

/** The extent the camera is held against, kept from the last pin. */
let pinned: Extent | null = null;
/** Set while the bound is moving the camera, so its own correction does not re-enter. */
let holding = false;

/** pin records what the camera is held against; null is a layout that wants plain fitting. */
export function pin(extent: Extent | null): void {
  pinned = extent;
}

/** How far each axis is stretched away from the pinned extent. */
export interface Stretch {
  x: number;
  y: number;
}

/**
 * drawnGraph is where the graph sits on the canvas, in viewport pixels. Both axes carry their
 * stretch, which moves the picture while the camera stands still.
 */
function drawnGraph(
  showing: Sigma,
  stretch: Stretch,
): { x: number; y: number; width: number; height: number } | null {
  if (!pinned) return null;
  // Against the camera as it is now: Sigma's cached projection is last frame's, so a correction
  // applied since would be measured again and applied twice.
  const now = { cameraState: showing.getCamera().getState() };
  const near = showing.graphToViewport({ x: pinned.x0 * stretch.x, y: pinned.y0 * stretch.y }, now);
  const far = showing.graphToViewport({ x: pinned.x1 * stretch.x, y: pinned.y1 * stretch.y }, now);
  return {
    x: Math.min(near.x, far.x),
    y: Math.min(near.y, far.y),
    width: Math.abs(far.x - near.x),
    height: Math.abs(far.y - near.y),
  };
}

/**
 * ceiling is the largest ratio that still leaves the bound's share of the viewport on graph,
 * taken over both axes (-> bounds ratioCeilingBoth) — the width alone would hold a one-instant
 * archive's column of claims at the ratio its own narrowness set. Sigma's camera checks it on
 * every state it accepts, so every instrument stops in one place.
 */
export function ceiling(
  showing: Sigma | null,
  width: number,
  height: number,
  stretch: Stretch,
): number | null {
  if (!showing || (width <= 0 && height <= 0)) return null;
  const rect = drawnGraph(showing, stretch);
  if (!rect) return null;
  return ratioCeilingBoth(rect, showing.getCamera().getState().ratio, { width, height });
}

/**
 * fitStretch is the y stretch bringing the strata to the full height — a fit, not a share of it.
 */
export function fitStretch(showing: Sigma | null, height: number, stretch: Stretch): number | null {
  if (!showing || height <= 0) return null;
  const rect = drawnGraph(showing, stretch);
  if (!rect) return null;
  return fillStretch(rect.height, stretch.y, height);
}

/**
 * offCentreY is how far the drawn strata sit below the middle, which a fit says nothing about.
 */
export function offCentreY(showing: Sigma | null, height: number, stretch: Stretch): number | null {
  if (!showing || height <= 0) return null;
  const rect = drawnGraph(showing, stretch);
  if (!rect) return null;
  return rect.y + rect.height / 2 - height / 2;
}

/**
 * drawnRect exposes that same measurement, so a picture sitting wrong is reported as numbers.
 */
export function drawnRect(
  showing: Sigma | null,
  stretch: Stretch,
): { x: number; y: number; width: number; height: number } | null {
  return showing ? drawnGraph(showing, stretch) : null;
}

/**
 * holdTo puts the picture back inside the bound — the pan only, the ratio being the camera's own
 * ceiling. Sigma's `cameraPanBoundaries` measures the graph it knows, which a stretch has moved.
 */
export function holdTo(showing: Sigma | null, width: number, height: number, stretch: Stretch): void {
  if (!showing || !pinned || holding || width <= 0 || height <= 0) return;

  holding = true;
  try {
    const camera = showing.getCamera();
    const rect = drawnGraph(showing, stretch);
    if (!rect) return;
    const dx = hold(rect.x, rect.width, width);
    const dy = hold(rect.y, rect.height, height);
    if (dx === 0 && dy === 0) return;

    // Converted through the transform the renderer draws with; the camera moves the other way.
    const origin = showing.viewportToFramedGraph({ x: 0, y: 0 });
    const moved = showing.viewportToFramedGraph({ x: dx, y: dy });
    const now = camera.getState();
    camera.setState({ ...now, x: now.x - (moved.x - origin.x), y: now.y - (moved.y - origin.y) });
  } finally {
    holding = false;
  }
}
