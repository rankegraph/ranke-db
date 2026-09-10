/**
 * package: core / graph
 * type:    data
 * job:     hold every claim loaded this session in one graphology instance
 * limits:  headless storage; neither layout nor rendering (-> core/layout, render)
 *
 * Module scope, deliberately outside framework state: React holds a handle, never the
 * data. An id is a content address, so a claim reached twice merges to one node — the
 * size is the union, not the sum.
 */

import { DirectedGraph } from 'graphology';
import type { DrawnClaim } from '../claims.ts';
import {
  CLASS_COLOR,
  CLASS_SIZE,
  addClaims,
  addClaimsProgressively,
  forgetPending,
  pendingCount,
} from './build.ts';
import type { ProgressReport } from './build.ts';

/** The single graphology instance the renderer reads from. */
const universe = new DirectedGraph({ allowSelfLoops: false });

/** Contribution counter across everything merged in, for view ranges. */
let contributions = 0;

export interface MergeResult {
  addedNodes: number;
  addedEdges: number;
  /** Claims already present, which is the accumulation working as intended. */
  duplicateClaims: number;
  danglingRefs: number;
  mergeMs: number;
}

/** graph exposes the union for the renderer and core operations to read. */
export function graph(): DirectedGraph {
  return universe;
}

/** totalContributions is the highest contribution index merged so far. */
export function totalContributions(): number {
  return contributions;
}

/**
 * mergeClaims folds a page of claims into the union — what a query response does,
 * whether it came from a generator or a server.
 */
export function mergeClaims(claims: DrawnClaim[], contributionCount: number): MergeResult {
  const result = addClaims(universe, claims);
  contributions = Math.max(contributions, contributionCount);
  return result;
}

/**
 * mergeClaimsProgressively is `mergeClaims` in batches that yield to the UI, so a
 * large page reports progress instead of freezing the tab.
 */
export async function mergeClaimsProgressively(
  claims: DrawnClaim[],
  contributionCount: number,
  onProgress?: (report: ProgressReport) => void,
): Promise<MergeResult> {
  const result = await addClaimsProgressively(universe, claims, {}, onProgress);
  contributions = Math.max(contributions, contributionCount);
  return result;
}

/** clear empties the union — a session reset, not something a query does. */
export function clear(): void {
  universe.clear();
  forgetPending();
  contributions = 0;
}

/**
 * mergeMore folds claims in without any of what `load` does around a read — no relayout, no
 * reframe, no lens drop, no time axis rebuilt. What a view already draws stays where it is,
 * which is what lets a closure be read a piece at a time.
 */
export function mergeMore(claims: DrawnClaim[]): MergeResult {
  return addClaims(universe, claims);
}

/** unreadRefs is how many references still wait on a target no read has supplied. */
export function unreadRefs(): number {
  return pendingCount();
}

export { CLASS_COLOR, CLASS_SIZE };
