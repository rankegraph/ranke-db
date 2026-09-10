/**
 * package: core / detail
 * type:    logic
 * job:     read one claim or edge out of the union, as a pane lists it
 * limits:  headless reads; holds nothing, decides nothing (-> core/graph/universe)
 *
 * A pane asks about what is selected, and gets a plain record back. Both ends of a reference
 * travel together — the edge that states it and the claim at the far end — so a row never has
 * to say "type" and leave which one open.
 */

import { graph } from './graph/universe.ts';

/**
 * edgeDetail gathers what the detail pane shows for one edge: its type, and the two claims it
 * joins. An edge belongs to the claim it points *from* — that claim created it — so the source
 * is where its provenance is read.
 */
export function edgeDetail(key: string) {
  const g = graph();
  if (!g.hasEdge(key)) return null;
  const from = g.source(key);
  const to = g.target(key);
  const attrs = g.getEdgeAttributes(key) as Record<string, unknown>;
  const label = (node: string) => String(g.getNodeAttribute(node, 'label') ?? '');
  const claimType = (node: string) => String(g.getNodeAttribute(node, 'claimType') ?? '');
  return {
    key,
    edgeType: String(attrs.claimType ?? ''),
    contentSize: attrs.contentSize as number | undefined,
    encoding: attrs.encoding as string | undefined,
    contentKind: String(attrs.contentKind ?? 'none'),
    contentHash: String(attrs.contentHash ?? ''),
    fields: (attrs.fields ?? {}) as Readonly<Record<string, string>>,
    direction: Number(attrs.direction ?? 0),
    from,
    fromLabel: label(from),
    fromType: claimType(from),
    to,
    toLabel: label(to),
    toType: claimType(to),
  };
}

/**
 * Reference is one end of an edge as a pane lists it: the edge that states it, and the claim at
 * the other end. Both travel, so a row never has to say "type" and leave which one open.
 */
export interface Reference {
  edge: string;
  edgeType: string;
  id: string;
  claimType: string;
}

/** reference reads one row off an edge and the claim at its far end. */
function reference(edge: string, edgeAttrs: unknown, far: string, farAttrs: unknown): Reference {
  return {
    edge,
    edgeType: String((edgeAttrs as { claimType?: string }).claimType ?? ''),
    id: far,
    claimType: String((farAttrs as { claimType?: string }).claimType ?? ''),
  };
}

/** claimDetail gathers what the detail pane shows for one claim. */
export function claimDetail(id: string) {
  const g = graph();
  if (!g.hasNode(id)) return null;
  const attrs = g.getNodeAttributes(id) as Record<string, unknown>;
  // A reference is two things a reader may ask about: the edge that states it, and the claim it
  // points at. Both travel, so the pane never has to say "type" and leave which one open.
  const references: Reference[] = [];
  g.forEachOutEdge(id, (edge, edgeAttrs, _s, target, _sa, targetAttrs) => {
    references.push(reference(edge, edgeAttrs, target, targetAttrs));
  });
  // The other half: an edge belongs to the claim it points from, so this is somebody else's
  // statement about this one — answerable only by the union, the referencing claim need not be drawn.
  const referencedBy: Reference[] = [];
  g.forEachInEdge(id, (edge, edgeAttrs, source, _t, sourceAttrs) => {
    referencedBy.push(reference(edge, edgeAttrs, source, sourceAttrs));
  });
  return {
    id,
    claimType: String(attrs.claimType ?? ''),
    contribution: Number(attrs.contribution ?? 0),
    createdAt: Number(attrs.createdAt ?? 0),
    createdAtIso: String(attrs.createdAtIso ?? ''),
    height: attrs.height as number | undefined,
    /** References the claim states, against `references.length` drawn. */
    statedReferences: attrs.references as number | undefined,
    contentSize: attrs.contentSize as number | undefined,
    encoding: attrs.encoding as string | undefined,
    contentKind: String(attrs.contentKind ?? 'none'),
    contentHash: String(attrs.contentHash ?? ''),
    fields: (attrs.fields ?? {}) as Readonly<Record<string, string>>,
    label: String(attrs.label ?? ''),
    degree: g.degree(id),
    references,
    referencedBy,
    referencedByCount: g.inDegree(id),
  };
}
