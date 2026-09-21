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

import { asText, contentOf, isTextual } from './content.ts';
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

/**
 * claimText is a claim's content read as characters, or empty where the encoding says the bytes
 * are not text or no read has brought them in. The encoding travels on the claim; the bytes sit
 * in the content cache, which a capped read fills for every small body it returns.
 */
export function claimText(id: string): string {
  const g = graph();
  if (!g.hasNode(id)) return '';
  if (!isTextual(g.getNodeAttribute(id, 'encoding') as string | undefined)) return '';
  const bytes = contentOf(id);
  return bytes ? asText(bytes).trim() : '';
}

/** How much of a claim's first line a caption carries. */
export const CAPTION_TEXT_CHARS = 60;

/**
 * captionText is what a claim says, for the second line of its caption: the first line of its
 * content, cut to a length that reads beside a dot. A type names what kind of thing a claim is
 * and this names the thing — which is the difference between reading a provenance chain and
 * clicking through it claim by claim.
 */
export function captionText(id: string): string {
  const first = claimText(id).split('\n', 1)[0]?.trim() ?? '';
  if (first.length <= CAPTION_TEXT_CHARS) return first;
  return `${first.slice(0, CAPTION_TEXT_CHARS).trimEnd()}…`;
}
