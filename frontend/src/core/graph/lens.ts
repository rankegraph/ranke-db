/**
 * package: core / graph
 * type:    logic
 * job:     cut a window of the union out as its own small graph
 * limits:  builds a graph; which Sigma draws it is the renderer's (-> render)
 *
 * Stretching x means rewriting every node's position, which is affordable only when there are
 * few of them. So zooming in hands the renderer a *subset* — the claims inside the window —
 * and every stretch step is then O(window) rather than O(union).
 *
 * The union stays authoritative. A lens is a copy made for the eye: derived, read-only, and
 * discarded on the way out, so there is never a question of which graph is the archive.
 *
 * Edges leaving the window are *not* followed. Bringing the far end along as a stub was the
 * first design, and measuring it settled the question: provenance reaches back arbitrarily far
 * in time, so a 4% window of 100k claims pulled in 8,333 stubs against 3,994 real claims. A
 * lens two-thirds composed of placeholders is not a lens.
 *
 * So a claim keeps a count of the references that leave the view, and the renderer can mark it
 * as citing something out of sight. The signal belongs on the claim that has the references,
 * not on a crowd of dots at the edge that individually say nothing.
 */

import { DirectedGraph } from 'graphology';

/** The attribute a claim carries when it references, or is referenced by, something out of the window. */
export const OUTSIDE_ATTR = 'outside';

export interface Window {
  /** The x range to include, in graph units. */
  x0: number;
  x1: number;
}

export interface Lens {
  graph: DirectedGraph;
  /** Claims inside the window — which is the whole lens. */
  inside: number;
  /** Edges reaching out of the window, counted rather than drawn. */
  leaving: number;
  buildMs: number;
}

/**
 * lensOf copies the claims whose x lies in the window and the edges among them, and counts the
 * edges that leave. Node attributes are copied, so a layout over the lens cannot move the
 * union — the lens is for the eye and the union is the archive.
 */
export function lensOf(union: DirectedGraph, window: Window): Lens {
  const t0 = performance.now();
  const lens = new DirectedGraph({ allowSelfLoops: false });
  const { x0, x1 } = window;

  const within = (node: string): boolean => {
    const x = union.getNodeAttribute(node, 'x') as number;
    return typeof x === 'number' && x >= x0 && x <= x1;
  };

  let inside = 0;
  union.forEachNode((node, attr) => {
    if (!within(node)) return;
    lens.addNode(node, { ...attr, [OUTSIDE_ATTR]: 0 });
    inside++;
  });

  let leaving = 0;
  union.forEachEdge((edge, attr, source, target) => {
    const fromInside = lens.hasNode(source);
    const toInside = lens.hasNode(target);
    if (fromInside && toInside) {
      lens.addEdgeWithKey(edge, source, target, { ...attr });
      return;
    }
    // One end only: the edge reaches out of the view, and the end that is here says so.
    const here = fromInside ? source : toInside ? target : null;
    if (here === null) return;
    lens.setNodeAttribute(here, OUTSIDE_ATTR, (lens.getNodeAttribute(here, OUTSIDE_ATTR) as number) + 1);
    leaving++;
  });

  return { graph: lens, inside, leaving, buildMs: performance.now() - t0 };
}

/**
 * windowAround is the window a viewport shows, widened by a margin so a pan of a screen or so
 * needs no new lens. Wider costs more to build; narrower rebuilds more often.
 */
export function windowAround(x0: number, x1: number, margin = 0.5): Window {
  const width = Math.abs(x1 - x0);
  const grow = width * margin;
  return { x0: Math.min(x0, x1) - grow, x1: Math.max(x0, x1) + grow };
}

/** covers reports whether a window still holds a viewport, so a lens can be reused. */
export function covers(window: Window, x0: number, x1: number): boolean {
  return window.x0 <= Math.min(x0, x1) && window.x1 >= Math.max(x0, x1);
}
