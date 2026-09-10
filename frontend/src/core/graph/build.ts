/**
 * package: core / graph
 * type:    logic
 * job:     turn claims into a graphology graph
 * limits:  headless: no renderer, no DOM (-> render/renderer)
 *
 * `addClaims` merges into an existing graph, because the explorer accumulates: query
 * after query folds into one union keyed by content address. `buildGraph` is the
 * one-shot form the benches use.
 */

import { DirectedGraph } from 'graphology';
import {
  contentComplete,
  contentEncoding,
  contentSize,
  inlineBytes,
  matchTypeList,
} from '@rankegraph/ranke';
import type { DrawnClaim } from '../claims.ts';
import { rememberContent } from '../content.ts';
import { hashString } from '../hash.ts';
import type { MockArchive } from '../mock/model.ts';
import { yieldToPaint } from '../scheduler.ts';

/** CLASS_COLOR maps a node class to its paint colour (Okabe–Ito, colour-safe). */
export const CLASS_COLOR: Record<string, string> = {
  source: '#0072b2',
  derivation: '#e69f00',
  entity: '#009e73',
  relation: '#cc79a7',
  contribution: '#7f7f7f',
};

/**
 * How far `colorFor` may nudge a class's base colour per axis. Hue stays tight — the class is
 * what the Okabe–Ito palette keeps colour-blind-safe, so a subtype must not drift into a
 * neighbouring class's hue — while saturation and lightness, which read fine without hue
 * discrimination, can move further.
 */
const HUE_JITTER_DEG = 10;
const SAT_JITTER = 0.18;
const LIGHT_JITTER = 0.2;

/**
 * Edges paint darker than nodes at the same colour, in the same class family — a graph reads
 * front-to-back as nodes-then-edges, so the edges recede rather than compete for the eye.
 */
export const EDGE_LIGHTNESS_SCALE = 0.6;

/**
 * contribution/* darkens further on top of whatever scale a node or edge already draws at — it
 * is the structural spine (branch tables, the head chain, signing identities), the bulk of the
 * graph and the least of its meaning, so it recedes further than an ordinary class already does.
 */
const CONTRIBUTION_LIGHTNESS_SCALE = 0.7;

/** lightnessScaleFor composes a class's extra darkening onto whatever scale the caller already
 * applies — contribution/* nodes and edges both go through this, so the two stay proportionate
 * to each other the same way every other class already is. */
function lightnessScaleFor(cls: string, scale: number): number {
  return cls === 'contribution' ? scale * CONTRIBUTION_LIGHTNESS_SCALE : scale;
}

/** hexToHsl reads a `#rrggbb` colour as hue (degrees), saturation and lightness (0..1). */
function hexToHsl(hex: string): [number, number, number] {
  const r = parseInt(hex.slice(1, 3), 16) / 255;
  const g = parseInt(hex.slice(3, 5), 16) / 255;
  const b = parseInt(hex.slice(5, 7), 16) / 255;
  const max = Math.max(r, g, b);
  const min = Math.min(r, g, b);
  const l = (max + min) / 2;
  const d = max - min;
  if (d === 0) return [0, 0, l];
  const s = d / (1 - Math.abs(2 * l - 1));
  let h: number;
  if (max === r) h = ((g - b) / d) % 6;
  else if (max === g) h = (b - r) / d + 2;
  else h = (r - g) / d + 4;
  h *= 60;
  if (h < 0) h += 360;
  return [h, s, l];
}

/** hslToHex is hexToHsl's inverse, back to `#rrggbb`. */
function hslToHex(h: number, s: number, l: number): string {
  const c = (1 - Math.abs(2 * l - 1)) * s;
  const x = c * (1 - Math.abs(((h / 60) % 2) - 1));
  const m = l - c / 2;
  let rgb: [number, number, number];
  if (h < 60) rgb = [c, x, 0];
  else if (h < 120) rgb = [x, c, 0];
  else if (h < 180) rgb = [0, c, x];
  else if (h < 240) rgb = [0, x, c];
  else if (h < 300) rgb = [x, 0, c];
  else rgb = [c, 0, x];
  const toHex = (v: number) => Math.round((v + m) * 255).toString(16).padStart(2, '0');
  return `#${toHex(rgb[0])}${toHex(rgb[1])}${toHex(rgb[2])}`;
}

/**
 * colorFor paints a claim within its class's base hue rather than flat on it: the class stays
 * the primary read, and the subtype nudges hue, saturation and lightness deterministically —
 * every claim of one subtype paints identically, and distinct subtypes within a class are told
 * apart at a glance, session to session, without a legend.
 */
export function colorFor(cls: string, typeSub: string, lightnessScale = 1): string {
  const base = CLASS_COLOR[cls];
  if (!base) return '#999999';
  const [h, s, l] = hexToHsl(base);
  // Three independent bytes of one hash, so the three axes don't move in lockstep.
  const hash = hashString(`${cls}/${typeSub}`);
  const unit = (byte: number) => (byte / 255) * 2 - 1; // a hash byte as a value in [-1, 1]
  const hue = (h + unit(hash & 0xff) * HUE_JITTER_DEG + 360) % 360;
  const sat = clamp01(s + unit((hash >>> 8) & 0xff) * SAT_JITTER);
  const light = clamp01((l + unit((hash >>> 16) & 0xff) * LIGHT_JITTER) * lightnessScale);
  return hslToHex(hue, sat, light);
}

function clamp01(v: number): number {
  return Math.max(0, Math.min(1, v));
}

/**
 * brighten raises a colour's lightness toward white by `amount` (0..1 of its remaining
 * headroom) — for highlighting an already-coloured node or edge without reassigning it to an
 * unrelated accent hue, which would cost it its class and subtype.
 */
export function brighten(hex: string, amount: number): string {
  const [h, s, l] = hexToHsl(hex);
  return hslToHex(h, s, clamp01(l + (1 - l) * amount));
}

/**
 * Fixed radius for `contribution/*` claims under `sizeByDegree` — deliberately not the degree
 * floor: a reader who has switched the stratum on wants to click these, and 1-2px is a mouse
 * target only in theory.
 */
export const CONTRIBUTION_SIZE = 6;

/**
 * CLASS_SIZE gives each class a base radius, before `sizeByDegree` scales content classes up —
 * `contribution` is excluded from that scaling and stays at `CONTRIBUTION_SIZE` (-> sizeByDegree).
 */
export const CLASS_SIZE: Record<string, number> = {
  source: 2,
  derivation: 1.5,
  entity: 3,
  relation: 2,
  contribution: CONTRIBUTION_SIZE,
};

export interface BuildOptions {
  /** `full` also stores the claim metadata a detail pane needs. */
  attrs?: 'lean' | 'full';
  /**
   * Edge types to leave out, as the type globs a query's `edges` takes — so
   * `contribution/*` drops a class and a leading `-` excludes. Every claim carries a
   * `contribution/contributor` edge, so a contributor node's degree equals the
   * number of claims it signed — a star that dominates layout and the picture.
   */
  dropEdgeTypes?: string[];
}

export interface MergeStats {
  addedNodes: number;
  addedEdges: number;
  duplicateClaims: number;
  /** References to claims outside the delivered set — real reads have these. */
  danglingRefs: number;
  mergeMs: number;
}

/** One shared empty record, so the attribute exists on every node at no cost per claim. */
const NO_FIELDS: Readonly<Record<string, string>> = Object.freeze({});

/** fieldsOf keeps a record's extension fields, deduplicating the common empty case. */
function fieldsOf(fields: Readonly<Record<string, string>>): Readonly<Record<string, string>> {
  return Object.keys(fields).length > 0 ? fields : NO_FIELDS;
}

/**
 * addClaims folds claims into a graph, skipping any already present. Node keys are
 * claim ids, so a claim delivered twice merges rather than duplicating.
 */
export function addClaims(
  graph: DirectedGraph,
  claims: DrawnClaim[],
  opts: BuildOptions = {},
): MergeStats {
  const t0 = performance.now();
  const nodes = addNodes(graph, claims, opts);
  const edges = addEdges(graph, claims, opts);
  return {
    addedNodes: nodes.addedNodes,
    duplicateClaims: nodes.duplicateClaims,
    addedEdges: edges.addedEdges,
    danglingRefs: edges.danglingRefs,
    mergeMs: performance.now() - t0,
  };
}

/** Claims merged between yields: small enough to keep a frame, large enough to be cheap. */
const MERGE_BATCH = 4000;

export interface ProgressReport {
  stage: string;
  done: number;
  total: number;
}

/**
 * addClaimsProgressively is `addClaims` with the loop broken into batches that yield
 * to the UI between them. The work is identical; only the scheduling differs — and
 * that is the whole difference between a progress indicator and an apparent hang.
 */
export async function addClaimsProgressively(
  graph: DirectedGraph,
  claims: DrawnClaim[],
  opts: BuildOptions = {},
  onProgress?: (report: ProgressReport) => void,
): Promise<MergeStats> {
  const t0 = performance.now();
  const totals: MergeStats = {
    addedNodes: 0,
    addedEdges: 0,
    duplicateClaims: 0,
    danglingRefs: 0,
    mergeMs: 0,
  };

  // Claims first, in batches; an edge can only be added once both ends exist.
  for (let from = 0; from < claims.length; from += MERGE_BATCH) {
    const stats = addNodes(graph, claims.slice(from, from + MERGE_BATCH), opts);
    totals.addedNodes += stats.addedNodes;
    totals.duplicateClaims += stats.duplicateClaims;
    const done = Math.min(from + MERGE_BATCH, claims.length);
    onProgress?.({ stage: 'merging claims', done, total: claims.length });
    if (done < claims.length) await yieldToPaint();
  }

  for (let from = 0; from < claims.length; from += MERGE_BATCH) {
    const stats = addEdges(graph, claims.slice(from, from + MERGE_BATCH), opts);
    totals.addedEdges += stats.addedEdges;
    totals.danglingRefs += stats.danglingRefs;
    const done = Math.min(from + MERGE_BATCH, claims.length);
    onProgress?.({ stage: 'merging edges', done, total: claims.length });
    if (done < claims.length) await yieldToPaint();
  }

  totals.mergeMs = performance.now() - t0;
  return totals;
}

/** addNodes merges the claims themselves, skipping any already present. */
function addNodes(
  graph: DirectedGraph,
  claims: DrawnClaim[],
  opts: BuildOptions,
): { addedNodes: number; duplicateClaims: number } {
  const attrs = opts.attrs ?? 'full';
  let addedNodes = 0;
  let duplicateClaims = 0;

  for (const drawn of claims) {
    const claim = drawn.claim;
    if (graph.hasNode(claim.id)) {
      duplicateClaims++;
      continue;
    }
    // The class arrives split by the decoder, so nothing here re-reads the type string.
    const cls = claim.typeClass;
    const base = {
      x: 0,
      y: 0,
      size: CLASS_SIZE[cls] ?? 2,
      color: colorFor(cls, claim.typeSub, lightnessScaleFor(cls, 1)),
      // `contribution` is in even the lean profile: it is the layout's axis, not
      // metadata. `claimType`, never `type` — Sigma reads `type` as the name of
      // the rendering program to use.
      contribution: drawn.contribution,
      cls,
    };
    // Bytes the record carries are already here, and a claim is content-addressed, so keeping
    // them is a cache that can never be stale — and the detail pane needs no read to show them.
    // Whole bytes only: a capped read may cut a body to a prefix (R-QCONTENT), and a prefix
    // cached as the content would be served as it wherever the id is next looked up.
    if (attrs !== 'lean') {
      const held = inlineBytes(claim.content);
      if (held && contentComplete(claim.content)) rememberContent(claim.id, held);
    }
    graph.addNode(
      claim.id,
      attrs === 'lean'
        ? base
        : {
            ...base,
            label: drawn.label,
            claimType: claim.type,
            createdAt: claim.createdAtMs,
            // As stored, precision included — the ms number above is the layout's.
            createdAtIso: claim.createdAt,
            height: claim.height,
            contentSize: contentSize(claim.content),
            encoding: contentEncoding(claim.content),
            contentKind: claim.content.kind,
            contentHash: claim.content.kind === 'external' ? claim.content.hash : '',
            fields: fieldsOf(claim.fields),
          },
    );
    addedNodes++;
  }
  return { addedNodes, duplicateClaims };
}

/**
 * addEdges merges each claim's typed references, once both ends are present.
 *
 * An edge is a claim's statement in its own right — it has a type, its own content and the
 * direction a relation reads in — so the full profile carries what a reader can be shown about
 * one. The lean profile keeps the type alone, which is all a reducer needs.
 */
function addEdges(
  graph: DirectedGraph,
  claims: DrawnClaim[],
  opts: BuildOptions,
): { addedEdges: number; danglingRefs: number } {
  const drop = opts.dropEdgeTypes ?? [];
  const attrs = opts.attrs ?? 'full';
  let addedEdges = 0;
  let danglingRefs = 0;
  for (const { claim } of claims) {
    for (const edge of claim.edges) {
      // Matched by the library's glob rules, which are the contract's: `contribution/*`
      // names a class, a `*` never crosses the `/`, and a leading `-` re-admits.
      if (drop.length > 0 && matchTypeList(drop, edge.type)) continue;
      if (!graph.hasNode(edge.reference)) {
        danglingRefs++;
        continue;
      }
      const before = graph.size;
      graph.mergeDirectedEdge(
        claim.id,
        edge.reference,
        attrs === 'lean'
          ? { claimType: edge.type }
          : {
              claimType: edge.type,
              color: colorFor(edge.typeClass, edge.typeSub, lightnessScaleFor(edge.typeClass, EDGE_LIGHTNESS_SCALE)),
              contentSize: contentSize(edge.content),
              encoding: contentEncoding(edge.content),
              contentKind: edge.content.kind,
              contentHash: edge.content.kind === 'external' ? edge.content.hash : '',
              fields: fieldsOf(edge.fields),
              // RelationFrom (+1) or RelationTo (-1) on relation/*, 0 elsewhere (§4.7).
              direction: edge.relationDirection,
            },
      );
      if (graph.size > before) addedEdges++;
    }
  }
  return { addedEdges, danglingRefs };
}

export interface BuildResult {
  graph: DirectedGraph;
  buildMs: number;
  mergedEdges: number;
  danglingRefs: number;
}

/** buildGraph turns one archive into a fresh graph — the benches' entry point. */
export function buildGraph(archive: MockArchive, opts: BuildOptions = {}): BuildResult {
  const graph = new DirectedGraph({ allowSelfLoops: false });
  const stats = addClaims(graph, archive.claims, opts);
  return {
    graph,
    buildMs: stats.mergeMs,
    mergedEdges: stats.duplicateClaims,
    danglingRefs: stats.danglingRefs,
  };
}

export interface DegreeStats {
  max: number;
  mean: number;
  p50: number;
  p99: number;
  /** Nodes with degree ≥ 100 — the hubs that dominate force layout cost. */
  hubs: number;
}

/** degreeStats summarises the degree distribution a layout has to cope with. */
export function degreeStats(graph: DirectedGraph): DegreeStats {
  const degrees = new Int32Array(graph.order);
  let i = 0;
  let sum = 0;
  let max = 0;
  let hubs = 0;
  graph.forEachNode((node) => {
    const d = graph.degree(node);
    degrees[i++] = d;
    sum += d;
    if (d > max) max = d;
    if (d >= 100) hubs++;
  });
  degrees.sort();
  const at = (q: number) => degrees[Math.min(degrees.length - 1, Math.floor(degrees.length * q))];
  return { max, mean: sum / (graph.order || 1), p50: at(0.5), p99: at(0.99), hubs };
}

/**
 * sizeByDegree scales node radius by degree so hubs read as hubs — semantic hubs, not
 * provenance ones. A `contribution/*` claim's degree is an artefact of the ADT (a signing key
 * referenced by every claim it signed, a branch table chaining every revision before it) rather
 * than a fact about what it says, so scaling by it would let the busiest key dwarf every
 * entity and relation on the canvas. Those claims are excluded from the degree range and held
 * at a fixed, clickable size instead; every other class scales across the full range as before.
 */
export function sizeByDegree(graph: DirectedGraph, minSize = 1.5, maxSize = 14): void {
  let max = 1;
  graph.forEachNode((node, attr) => {
    if (attr.cls === 'contribution') return;
    const d = graph.degree(node);
    if (d > max) max = d;
  });
  const logMax = Math.log1p(max);
  graph.updateEachNodeAttributes((node, attr) => {
    if (attr.cls === 'contribution') {
      attr.size = CONTRIBUTION_SIZE;
      return attr;
    }
    const t = Math.log1p(graph.degree(node)) / logMax;
    attr.size = minSize + t * (maxSize - minSize);
    return attr;
  });
}
