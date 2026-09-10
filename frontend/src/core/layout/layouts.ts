/**
 * package: core / layout
 * type:    logic
 * job:     position calculators — take a graph, return coordinates
 * limits:  headless; they draw nothing (-> render/renderer)
 *
 * `history` is the default: 21 ms at 50k, deterministic. ForceAtlas2 suits a few thousand
 * claims (13–130 ms/iter); at archive scale it runs ~1 500 ms/iter.
 */

import forceAtlas2 from 'graphology-layout-forceatlas2';
// Explicit extension: the package has no exports map, so Node's ESM resolver
// needs the file name. Vite resolves it either way.
import FA2Layout from 'graphology-layout-forceatlas2/worker.js';
import { circlepack, circular, random } from 'graphology-layout';
import type { DirectedGraph } from 'graphology';
import { requireContribution } from '../graph/shape.ts';
import { hashString } from '../hash.ts';

export type LayoutName =
  | 'timeline'
  | 'history'
  | 'layered'
  | 'random'
  | 'circular'
  | 'circlepack'
  | 'fa2-worker';

export const LAYOUT_LABELS: Record<LayoutName, string> = {
  timeline: 'time, in strata by class',
  history: 'rows by contribution',
  layered: 'layers by provenance depth',
  random: 'random',
  circular: 'circular',
  circlepack: 'circle-packed by class',
  'fa2-worker': 'ForceAtlas2 (worker)',
};

/**
 * STRATA are the classes a claim may belong to — each keeps its own colour (-> core/graph/
 * build.ts CLASS_COLOR) and filter toggle; which vertical band it draws in is BANDS' question,
 * not this one (entity and relation share a band).
 */
export const STRATA = ['contribution', 'source', 'derivation', 'entity', 'relation'] as const;

/**
 * BANDS are the timeline layout's vertical strata, bottom first (Sigma draws a larger graph y
 * higher up, so the array index is the y ordering). The order follows provenance — bookkeeping,
 * then what entered, what was concluded, then the semantic layer — so an edge points down or
 * sideways, almost never up. Entity and relation share the top band, each still with its own
 * colour and filter toggle (-> STRATA): left separate, relation is rare enough in most archives
 * that its own band sits nearly empty while the rest are cramped for room.
 */
const BANDS = ['contribution', 'source', 'derivation', 'semantic'] as const;

/** bandOf names the band a class draws in — entity and relation share one, semantic. */
function bandOf(cls: string): (typeof BANDS)[number] {
  if (cls === 'entity' || cls === 'relation') return 'semantic';
  return (BANDS as readonly string[]).includes(cls) ? (cls as (typeof BANDS)[number]) : 'source';
}

/** stratumOf is the band index a claim class draws in; an unknown class sits with the sources. */
export function stratumOf(cls: string): number {
  return BANDS.indexOf(bandOf(cls));
}

export interface TimelineContext {
  /** Where each claim's instant sits on the axis. */
  toX: (at: number) => number;
  /** The instant a claim carries, in epoch ms. */
  createdAt: (node: string) => number;
  /** The claim's class — the band it sits in. */
  classOf: (node: string) => string;
  /** The claim's subtype — which subband of its band it sits in (-> HEAD_SUBBAND_FRACTION). */
  subOf: (node: string) => string;
  /** Stretch on the strata, 1 being the height below — scales finished positions rather than
   * the layout height, so a band keeps its lanes and neighbours and only the room between grows. */
  yStretch?: number;
  /** Stretch on time, which the packing gap is measured in so a zoom never reshuffles lanes. */
  xStretch?: number;
  /**
   * Claims to place in packed lanes rather than by hash: within a band each takes the lowest
   * lane its neighbour in time leaves free, so a small set reads without overlap and every dot
   * can carry a caption. A provenance view passes its own closure (-> core/provenance).
   */
  pack?: ReadonlySet<string>;
  /** Place only the packed claims, leaving every other claim at the position it holds. */
  only?: boolean;
}

/**
 * Room a packed claim keeps to its right, in unstretched axis units — about nine node
 * diameters (-> timescale minStep), which is what a short caption spans at the zoom a fit
 * opens on. Captions are drawn in screen space, so no gap in graph units clears them at every
 * zoom; this one clears them where a reader starts.
 */
export const PACK_GAP_UNITS = 72;

/** How tall the whole picture is, in graph units — the extent the renderer normalises against. */
export const TIMELINE_HEIGHT = 1000;

/** Room one lane wants, which sets how many a band can hold. */
const LANE_UNITS = 16;

/**
 * Room below the bottom band and above the top — asymmetric, since the ruler (its own border
 * and background, not just empty pane) sits right at graph-y 0, making the same gap read as
 * tight there and spacious at the top; the bottom margin overcompensates rather than matching
 * pixels. Sigma draws a larger graph y higher up, so graph-y 0 is the one the ruler sits by.
 */
const TIMELINE_MARGIN_BOTTOM = LANE_UNITS * 2.5;
const TIMELINE_MARGIN_TOP = LANE_UNITS / 8;

/**
 * Share of the contribution band given to `contribution/head` claims, at the bottom; every
 * other subtype (contributor, branches, delete, expiry) fills the rest. A head is one per open
 * line of work, not one per claim like the others — set apart, it reads as what it is rather
 * than one more dot among the bookkeeping.
 */
const HEAD_SUBBAND_FRACTION = 0.3;

/** One vertical slice a claim may be placed in: where it starts, and how many lanes it holds. */
interface Slot {
  base: number;
  lanes: number;
  laneGap: number;
}

/** slotOf builds the lane geometry for a height, floored at one lane so a slice never vanishes. */
function slotOf(base: number, height: number): Slot {
  const lanes = Math.max(1, Math.floor(height / LANE_UNITS));
  return { base, lanes, laneGap: height / lanes };
}

/**
 * assignTimeline puts time on x and class strata on y. Claims sharing an instant share an x.
 * A packed set takes its lanes in time order instead (-> ctx.pack), which is affordable for the
 * few a provenance view draws and reads better than a scatter.
 * Within a slot, lane is otherwise a hash of the claim id — a time-sorted round-robin put
 * neighbours in time on consecutive lanes, drawing a diagonal staircase of collisions instead of
 * a scatter. Every band — and the contribution band's head subband (-> HEAD_SUBBAND_FRACTION) —
 * gets a fixed share of height regardless of which classes are currently shown, so a View-tab
 * toggle never moves anyone else's claims.
 */
export function assignTimeline(graph: DirectedGraph, ctx: TimelineContext): void {
  const bandHeight = (TIMELINE_HEIGHT - TIMELINE_MARGIN_BOTTOM - TIMELINE_MARGIN_TOP) / BANDS.length;
  // Bottom of the picture first, matching BANDS — Sigma draws a larger y higher up.
  const slotsOf = new Map<string, { rest: Slot; head?: Slot }>(
    BANDS.map((band, i) => {
      const bandBase = TIMELINE_MARGIN_BOTTOM + i * bandHeight;
      if (band !== 'contribution') return [band, { rest: slotOf(bandBase, bandHeight) }];
      const headHeight = bandHeight * HEAD_SUBBAND_FRACTION;
      return [
        band,
        { head: slotOf(bandBase, headHeight), rest: slotOf(bandBase + headHeight, bandHeight - headHeight) },
      ];
    }),
  );

  const stretch = ctx.yStretch ?? 1;
  const placed = new Map<string, { x: number; y: number }>();

  /** slotFor is the slice a claim belongs in: its band, and the head subband within it. */
  const slotFor = (node: string): Slot => {
    const slots = slotsOf.get(BANDS[stratumOf(ctx.classOf(node))]);
    return (slots?.head && ctx.subOf(node) === 'head' ? slots.head : slots?.rest) ?? slotOf(0, 0);
  };

  const packing: { node: string; x: number; slot: Slot }[] = [];
  // A restriction with nothing to restrict to would place no claim at all, which is never what
  // a caller means by it — an empty pack is a view whose membership has not been answered.
  const only = ctx.only === true && (ctx.pack?.size ?? 0) > 0;
  graph.forEachNode((node) => {
    const inPack = ctx.pack?.has(node) ?? false;
    if (only && !inPack) return;
    const x = ctx.toX(ctx.createdAt(node));
    const slot = slotFor(node);
    if (inPack) {
      packing.push({ node, x, slot });
      return;
    }
    const lane = hashString(node) % slot.lanes;
    placed.set(node, { x, y: (slot.base + lane * slot.laneGap) * stretch });
  });

  // In time order, so "the lane its neighbour leaves free" is a question about one pass.
  packing.sort((a, b) => a.x - b.x);
  const gap = PACK_GAP_UNITS * (ctx.xStretch ?? 1);
  const freeFrom = new Map<Slot, number[]>();
  for (const { node, x, slot } of packing) {
    let lanes = freeFrom.get(slot);
    if (!lanes) {
      lanes = new Array<number>(slot.lanes).fill(-Infinity);
      freeFrom.set(slot, lanes);
    }
    let lane = lanes.findIndex((from) => from <= x);
    // Every lane still held: more claims stand together than the band has lanes, so the one
    // free longest takes it. No lane would avoid the overlap, and the band keeps its height.
    if (lane < 0) lane = lanes.indexOf(Math.min(...lanes));
    lanes[lane] = x + gap;
    placed.set(node, { x, y: (slot.base + lane * slot.laneGap) * stretch });
  }

  graph.updateEachNodeAttributes((node, attr) => {
    const at = placed.get(node);
    if (at) {
      attr.x = at.x;
      attr.y = at.y;
    }
    return attr;
  });
}

/**
 * assignHistory positions claims by the contribution that added them: y is the
 * contribution, x is the position within it.
 */
export function assignHistory(
  graph: DirectedGraph,
  contribution: (node: string) => number,
  gap = { x: 24, y: 18 },
): void {
  const perRow = new Map<number, number>();
  graph.forEachNode((node) => {
    const c = requireContribution(contribution, node);
    perRow.set(c, (perRow.get(c) ?? 0) + 1);
  });
  const cursor = new Map<number, number>();
  graph.updateEachNodeAttributes((node, attr) => {
    const c = requireContribution(contribution, node);
    const i = cursor.get(c) ?? 0;
    cursor.set(c, i + 1);
    const width = perRow.get(c) ?? 1;
    attr.x = (i - (width - 1) / 2) * gap.x;
    attr.y = c * gap.y;
    return attr;
  });
}

/** assignLayered positions claims by provenance depth: y is the layer. */
export function assignLayered(
  graph: DirectedGraph,
  depth: Map<string, number>,
  gap = { x: 24, y: 12 },
): void {
  const perLayer = new Map<number, number>();
  graph.forEachNode((node) => {
    const d = depth.get(node) ?? 0;
    perLayer.set(d, (perLayer.get(d) ?? 0) + 1);
  });
  const cursor = new Map<number, number>();
  graph.updateEachNodeAttributes((node, attr) => {
    const d = depth.get(node) ?? 0;
    const i = cursor.get(d) ?? 0;
    cursor.set(d, i + 1);
    const width = perLayer.get(d) ?? 1;
    attr.x = (i - (width - 1) / 2) * gap.x;
    attr.y = d * gap.y;
    return attr;
  });
}

export interface LayoutContext {
  depth: Map<string, number>;
  contribution: (node: string) => number;
  /** What the timeline layout needs; absent, it falls back to provenance depth. */
  timeline?: TimelineContext;
  /** Wall-clock budget for the worker layout, in ms. */
  fa2Ms?: number;
}

/** apply runs a named layout and returns how long it took. */
export async function apply(
  graph: DirectedGraph,
  layout: LayoutName,
  ctx: LayoutContext,
): Promise<number> {
  const t0 = performance.now();
  switch (layout) {
    case 'timeline':
      // Without a scale there is no axis, so depth stands in rather than piling every claim
      // at x = 0.
      if (ctx.timeline) assignTimeline(graph, ctx.timeline);
      else assignLayered(graph, ctx.depth);
      break;
    case 'history':
      assignHistory(graph, ctx.contribution);
      break;
    case 'layered':
      assignLayered(graph, ctx.depth);
      break;
    case 'random':
      random.assign(graph, { scale: 1000 });
      break;
    case 'circular':
      circular.assign(graph, { scale: 1000 });
      break;
    case 'circlepack':
      circlepack.assign(graph, { hierarchyAttributes: ['cls'] });
      break;
    case 'fa2-worker': {
      // Off the main thread, per the design's rule that a blocked main thread is
      // 0 fps. Seeded cheaply first, then settled for a budget.
      random.assign(graph, { scale: 1000 });
      const supervisor = new FA2Layout(graph, {
        settings: { ...forceAtlas2.inferSettings(graph), barnesHutOptimize: true },
      });
      supervisor.start();
      await new Promise((resolve) => setTimeout(resolve, ctx.fa2Ms ?? 5000));
      supervisor.kill();
      break;
    }
  }
  return performance.now() - t0;
}
