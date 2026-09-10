/**
 * package: render / camera
 * type:    logic
 * job:     where the picture sits on the canvas, and the two zooms that scale it
 * limits:  acts on whichever instance is showing (-> render/instances); the arithmetic of the
 *          bound is bounds' and hold's (-> render/bounds, render/hold)
 *
 * A reader zooms one axis at a time, which the camera cannot: it takes both and magnifies the
 * claims with the space between them. So both zooms are stretches, and the camera says where.
 */

import type Sigma from 'sigma';
import { graph } from '../core/graph/universe.ts';
import { axisWidth, stretchX, stretchY, timeDenseExtent } from '../core/timeline.ts';
import type { Stretch } from '../core/timeline.ts';
import { activeView, useExplorer } from '../core/store.ts';
import { ratioFloor, stretchFloor } from './bounds.ts';
import { ceiling, drawnRect, fitStretch, holdTo, offCentreY } from './hold.ts';
import {
  both,
  canvasHeight,
  canvasWidth,
  repaint,
  showing,
  shownGraph,
} from './instances.ts';

/**
 * graphXAt converts a canvas position to the graph x under it, so the ruler needs no arithmetic.
 */
export function graphXAt(viewportX: number): number | null {
  return showing()?.viewportToGraph({ x: viewportX, y: 0 }).x ?? null;
}

/** graphYAt is the same question about the other axis — the stratum under a point. */
export function graphYAt(viewportY: number): number | null {
  return showing()?.viewportToGraph({ x: 0, y: viewportY }).y ?? null;
}

/** viewportXAt converts a graph x to the position on the canvas it draws at. */
export function viewportXAt(graphX: number): number | null {
  const instance = showing();
  return instance?.graphToViewport({ x: graphX, y: 0 }, cameraNow(instance)).x ?? null;
}
/** stretchNow is how far each axis is stretched, which the drawn graph carries but the camera does not. */
function stretchNow(): Stretch {
  const view = activeView(useExplorer.getState());
  return { x: view?.xStretch ?? 1, y: view?.yStretch ?? 1 };
}

/**
 * applyBound gives both cameras the ceiling and the floor. As settings, not onto the camera:
 * every settings update reinstalls the limits from settings, wiping a ceiling written to the
 * field. The pair goes in one call — Sigma validates both on every write and throws on a floor
 * above the ceiling, so a floor written before the ceiling that admits it never lands.
 */
export function applyBound(): void {
  const limit = ceiling(showing(), canvasWidth(), canvasHeight(), stretchNow());
  const floor = ratioFloor(limit);
  for (const each of both()) {
    if (!each) continue;
    // Setting schedules a refresh, and a stretch applies this on every wheel tick.
    if (each.getSetting('maxCameraRatio') === limit && each.getSetting('minCameraRatio') === floor) {
      continue;
    }
    each.setSettings({ maxCameraRatio: limit, minCameraRatio: floor });
  }
}

/** holdCamera brings the picture back inside the bound after something moved it. */
export function holdCamera(): void {
  holdTo(showing(), canvasWidth(), canvasHeight(), stretchNow());
}

/**
 * correctDrift moves the camera by a distance the picture drifted, through the transform the
 * renderer draws with — a viewport distance means nothing to the camera.
 */
function correctDrift(instance: Sigma, dx: number, dy: number): void {
  const origin = instance.viewportToFramedGraph({ x: 0, y: 0 });
  const moved = instance.viewportToFramedGraph({ x: dx, y: dy });
  const camera = instance.getCamera();
  const state = camera.getState();
  camera.setState({ ...state, x: state.x + (moved.x - origin.x), y: state.y + (moved.y - origin.y) });
}

/**
 * anchorAt pans so a graph position stays under a point on the canvas — what zooming means.
 */
export function anchorAt(viewportX: number, graphX: number): void {
  const instance = showing();
  if (!instance) return;
  const drifted = instance.graphToViewport({ x: graphX, y: 0 }, cameraNow(instance)).x - viewportX;
  if (Math.abs(drifted) < 0.01) return;
  correctDrift(instance, drifted, 0);
}

/** anchorYAt keeps the stratum under the pointer where it is, which is the same for y. */
export function anchorYAt(viewportY: number, graphY: number): void {
  const instance = showing();
  if (!instance) return;
  const drifted = instance.graphToViewport({ x: 0, y: graphY }, cameraNow(instance)).y - viewportY;
  if (Math.abs(drifted) < 0.01) return;
  correctDrift(instance, 0, drifted);
}

/**
 * cameraNow reads the camera as it is, not as the last frame drew it. Two corrections in one
 * frame is ordinary, and without this the first is measured again and applied twice.
 */
function cameraNow(instance: Sigma) {
  return { cameraState: instance.getCamera().getState() };
}

/**
 * compressionFloor is the least x stretch leaving the bound's share of the canvas on graph. A
 * y-zoom leaves the drawn axis the width it went in at, so this has nothing to say about it.
 */
function compressionFloor(stretch: number): number {
  const width = axisWidth();
  const canvas = canvasWidth();
  if (width === null || width <= 0 || canvas <= 0 || stretch <= 0) return 0;
  const drawn = axisSpanOnScreen(width * stretch);
  if (drawn === null || drawn <= 0) return 0;
  return stretchFloor(drawn, stretch, canvas) ?? 0;
}

/**
 * The two zooms a reader has: one axis apiece, each leaving the other as it was and keeping what
 * is under the pointer. Both stretch the layout — a camera zoom deep enough to fill the height
 * with a shallow band of strata would draw it as one solid mass.
 */
export function zoomX(factor: number, viewportX: number): void {
  // A stretch multiplies graph x, so where the content lands is arithmetic.
  const under = graphXAt(viewportX);
  const { applied } = stretchX(factor, shownGraph() ?? undefined, compressionFloor(stretchNow().x));
  if (applied === 1) return;
  repaint();
  if (under !== null) anchorAt(viewportX, under * applied);
  // The stretch moved the picture, not the camera, so the camera's share of the bound shifted.
  applyBound();
  holdCamera();
}

export function zoomY(factor: number, viewportY: number): void {
  const under = graphYAt(viewportY);
  // No floor here: a band of strata shorter than the canvas is ordinary, not stranded in space.
  const { applied } = stretchY(factor, shownGraph() ?? undefined);
  if (applied === 1) return;
  repaint();
  if (under !== null) anchorYAt(viewportY, under * applied);
  holdCamera();
}

/**
 * fitHeight brings the strata to the full height and centres them — the y-zoom at a measured
 * target, so time is untouched. With no pinned extent there are no strata, so the graph is framed.
 */
export function fitHeight(): void {
  const instance = showing();
  if (!instance) return;
  const height = canvasHeight();
  const target = fitStretch(instance, height, stretchNow());
  if (target === null) {
    const camera = instance.getCamera();
    camera.animate({ ...camera.getState(), y: 0.5, ratio: 1 }, { duration: 180 });
    return;
  }

  const { applied } = stretchY(target / stretchNow().y, shownGraph() ?? undefined);
  if (applied !== 1) repaint();
  // Measured again after the stretch: where the strata ended up is what centring is about.
  const drift = offCentreY(instance, height, stretchNow());
  if (drift !== null && Math.abs(drift) >= 0.5) correctDrift(instance, 0, drift);
  holdCamera();
}

/**
 * fitDenseTime stretches and centres time on the archive's dense range (-> core/timeline
 * timeDenseExtent) — where a first look lands, rather than the full axis fitHeight's box would
 * otherwise show in full. The bound stays the full axis regardless (-> core/timeline
 * restExtent): a remote outlier a stretch has pushed off screen is reached by zooming out
 * exactly as it always was, the same wheel gesture and the same ceiling — only where the
 * *first* look lands changes, the way panIntoView moves the camera without touching the bound.
 *
 * An archive without a lone remote outlier has `denseExtent` equal to the full axis, and gets
 * that whole axis brought to the canvas: a handful of claims minutes apart span a few units of
 * a box a thousand tall, drawn as a huddle a dozen pixels wide in the middle of an empty
 * canvas. Where the axis already fills the width the factor comes out at 1 and nothing moves.
 */
export function fitDenseTime(): void {
  const instance = showing();
  const width = canvasWidth();
  const dense = timeDenseExtent();
  const full = axisWidth();
  if (!instance || width <= 0 || !dense || full === null || full <= 0) return;
  const denseWidth = dense.x1 - dense.x0;
  if (!(denseWidth > 0)) return;

  const stretch = stretchNow().x;
  const now = cameraNow(instance);
  const left = instance.graphToViewport({ x: dense.x0 * stretch, y: 0 }, now).x;
  const right = instance.graphToViewport({ x: dense.x1 * stretch, y: 0 }, now).x;
  const drawnPx = Math.abs(right - left);
  if (!(drawnPx > 0)) return;
  const factor = width / drawnPx;
  if (!(factor > 1)) return;

  const { applied } = stretchX(factor, shownGraph() ?? undefined, compressionFloor(stretch));
  if (applied === 1) return;
  repaint();
  panTo(((dense.x0 + dense.x1) / 2) * stretch * applied);
  applyBound();
  holdCamera();
}

/**
 * axisSpanOnScreen is how wide the drawn axis currently is, in canvas pixels: the stretch and the
 * camera's ratio composed. Compression is measured against it, by the caller that owns the bound.
 */
export function axisSpanOnScreen(axisWidth: number): number | null {
  const instance = showing();
  if (!instance) return null;
  const now = cameraNow(instance);
  const left = instance.graphToViewport({ x: 0, y: 0 }, now).x;
  const right = instance.graphToViewport({ x: axisWidth, y: 0 }, now).x;
  return Math.abs(right - left);
}

/**
 * resetCamera frames the pinned extent again: the whole picture, both axes. `then` runs once the
 * animation is over, since anything that measures the picture must wait for it to stop moving.
 */
export function resetCamera(then?: () => void): void {
  const instance = showing();
  if (!instance) return;
  instance.getCamera().animate({ x: 0.5, y: 0.5, ratio: 1, angle: 0 }, { duration: 200 }, () => then?.());
}

/**
 * panIntoView brings a claim onto the canvas, leaving the picture alone when it is already there.
 * A claim reached from a list is centred, but the zoom is the reader's and only position moves.
 */
export function panIntoView(node: string): void {
  const instance = showing();
  const g = graph();
  if (!instance || !g.hasNode(node)) return;
  const at = instance.graphToViewport(
    { x: Number(g.getNodeAttribute(node, 'x')), y: Number(g.getNodeAttribute(node, 'y')) },
    cameraNow(instance),
  );
  const { width, height } = instance.getDimensions();
  const margin = 0.08 * Math.min(width, height);
  const inside =
    at.x >= margin && at.x <= width - margin && at.y >= margin && at.y <= height - margin;
  if (inside) return;

  const origin = instance.viewportToFramedGraph({ x: 0, y: 0 });
  const moved = instance.viewportToFramedGraph({ x: at.x - width / 2, y: at.y - height / 2 });
  const camera = instance.getCamera();
  const state = camera.getState();
  camera.animate(
    { ...state, x: state.x + (moved.x - origin.x), y: state.y + (moved.y - origin.y) },
    { duration: 220 },
  );
}

/**
 * geometry is what the bound is measured from, in its own units: the pinned extent, where it
 * lands on the canvas, and the canvas size. Null off the timeline, where nothing is pinned.
 */
export function geometry(): {
  stretch: Stretch;
  rect: { x: number; y: number; width: number; height: number };
  canvas: { width: number; height: number };
} | null {
  const instance = showing();
  const stretch = stretchNow();
  const rect = drawnRect(instance, stretch);
  if (!rect) return null;
  return { stretch, rect, canvas: { width: canvasWidth(), height: canvasHeight() } };
}

/** panTo centres a graph x, keeping the height as it is. */
export function panTo(graphX: number): void {
  const instance = showing();
  if (!instance) return;
  const framed = instance.graphToViewport({ x: graphX, y: 0 }, cameraNow(instance));
  const { width } = instance.getDimensions();
  const origin = instance.viewportToFramedGraph({ x: 0, y: 0 });
  const moved = instance.viewportToFramedGraph({ x: framed.x - width / 2, y: 0 });
  const camera = instance.getCamera();
  const state = camera.getState();
  camera.animate({ ...state, x: state.x + (moved.x - origin.x) }, { duration: 180 });
}


/** A rectangle being drawn, in canvas coordinates. */
export interface Box {
  x0: number;
  y0: number;
  x1: number;
  y1: number;
}

/**
 * zoomToBox makes a marked rectangle fill the viewport, by camera: a box asks what to look at,
 * not what the axes mean. The ratio takes whichever side needs more room, so the whole box shows.
 */
export function zoomToBox(box: Box): void {
  const instance = showing();
  if (!instance) return;
  const { width, height } = instance.getDimensions();
  if (width <= 0 || height <= 0) return;

  const a = instance.viewportToFramedGraph({ x: Math.min(box.x0, box.x1), y: Math.min(box.y0, box.y1) });
  const b = instance.viewportToFramedGraph({ x: Math.max(box.x0, box.x1), y: Math.max(box.y0, box.y1) });
  const fraction = Math.max(
    Math.abs(box.x1 - box.x0) / width,
    Math.abs(box.y1 - box.y0) / height,
  );
  if (!(fraction > 0)) return;

  const camera = instance.getCamera();
  const state = camera.getState();
  camera.animate(
    { x: (a.x + b.x) / 2, y: (a.y + b.y) / 2, ratio: state.ratio * fraction },
    { duration: 180 },
  );
}

