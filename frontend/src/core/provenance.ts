/**
 * package: core / provenance
 * type:    logic
 * job:     open a claim's derivation as a view, and read the closure behind it
 * limits:  headless; the scope and the merge are elsewhere (-> core/scope, core/graph/universe)
 *
 * A claim's provenance is a closure, and a closure rooted at a claim is a scope like any
 * other — so this opens an ordinary graph view and reads it through the query the explorer
 * already sends. What is particular to it is the second read: the claims arrive after the
 * view is drawn, so they merge without moving what a reader is already looking at.
 */

import { activeConnection, useConnections } from './connections.ts';
import { sourceFor } from './data/source.ts';
import type { DataSource } from './data/source.ts';
import type { DrawnClaim } from './claims.ts';
import { graph, mergeMore, unreadRefs } from './graph/universe.ts';
import { setMembers } from './graph/members.ts';
import { yieldToPaint } from './scheduler.ts';
import { activeView, defaultView, useExplorer } from './store.ts';
import type { ViewState } from './store.ts';
import { stretchOf } from './timeline.ts';
import { shortId } from './claims.ts';
import { isProvenance, provenanceScope, scopeKey } from './scope.ts';
import type { Scope } from './scope.ts';
import { layOut, nextViewId, notifyLoaded } from './session.ts';

/** log appends a line to the store's log, which the log pane renders. */
function log(line: string): void {
  useExplorer.getState().appendLog(line);
}

/**
 * openClaimProvenance draws a claim's derivation as an ordinary graph view: the closure rooted
 * at that claim, read within the branch it was reached through. One view per claim.
 */
export async function openClaimProvenance(id: string): Promise<void> {
  const store = useExplorer.getState();
  const existing = store.tabs.find(
    (t) => t.kind === 'graph' && t.scope !== null && t.scope.head === id && isProvenance(t.scope),
  );
  if (existing) {
    store.activateTab(existing.id);
    return;
  }

  // The branch the claim was reached through: a claim id is a head, never a scope name, and
  // R-QSCOPE wants a real one on the wire.
  const reachedThrough = activeView(store)?.scope?.branch ?? store.scopes.selected?.branch;
  if (!reachedThrough) {
    store.setNotice({
      level: 'error',
      text: 'No branch to read this claim within.',
      hint: 'Select a branch first — a query names a scope, and a claim id is a head rather than one.',
    });
    return;
  }
  const scope = provenanceScope(id, reachedThrough);

  const viewId = nextViewId();
  const view = defaultView(viewId, `provenance ${shortId(id)}`);
  view.scope = scope;
  // The closure read layer by layer, which is what a derivation is.
  view.layout = 'layered';
  store.addTab(view);
  await readProvenance(scope);
}

/**
 * readProvenance asks for the closure's identities first — that is what makes the shortfall
 * honest before a body arrives — then reads the claims the session lacks.
 */
async function readProvenance(scope: Scope): Promise<void> {
  const connection = activeConnection();
  if (!connection) {
    log('provenance  no source to ask which claims are in the closure');
    return;
  }
  const source = sourceFor(connection, useConnections.getState().secretOf(connection.id));
  const store = useExplorer.getState();

  let ids: string[];
  try {
    ids = await source.scopeIds(scope);
  } catch (err) {
    log(`provenance  failed — ${String(err)}`);
    store.setNotice({
      level: 'error',
      text: "Could not read the claim's provenance.",
      hint: err instanceof Error ? err.message : String(err),
    });
    return;
  }
  setMembers(scopeKey(scope), ids);
  log(`provenance  ${ids.length.toLocaleString('en-US')} claims in the closure of ${shortId(scope.head)}`);
  // What the session already holds is laid out and framed now — this view has just opened, so
  // there is no reading to disturb. The rest follows without moving it.
  await layOut('layered', { x: 1, y: 1 }, 'fit');
  await loadMore(scope, ids);
}

/**
 * loadMore reads the claims a closure names and the session lacks, and merges them without
 * reframing or dropping the lens. References held for want of a target are retried as the
 * targets arrive, so a claim reads as an initial claim only once it is one.
 */
export async function loadMore(scope: Scope, ids: string[]): Promise<void> {
  const g = graph();
  const missing = ids.filter((id) => !g.hasNode(id));
  if (missing.length === 0) {
    notifyLoaded('fit');
    return;
  }

  const connection = activeConnection();
  if (!connection) return;
  const source = sourceFor(connection, useConnections.getState().secretOf(connection.id));
  const store = useExplorer.getState();
  // Captured before the reads: the loop yields, so a tab switch mid-read is reachable, and
  // another view's layout and stretch is a mismatch the camera bound would be wrong about.
  const view = store.tabs.find(
    (t): t is ViewState =>
      t.kind === 'graph' && t.scope != null && isProvenance(t.scope) && t.scope.head === scope.head,
  );
  store.patchStatus({ busy: `reading ${missing.length.toLocaleString('en-US')} claims`, progress: null });
  await yieldToPaint();

  const claims = await readMissing(source, scope, missing, ids.length, store);
  const merged = mergeMore(claims);
  const waiting = unreadRefs();
  log(
    `provenance  +${merged.addedNodes} claims, +${merged.addedEdges} references` +
      `${waiting > 0 ? `, ${waiting} still waiting on a target` : ''}`,
  );
  useExplorer.getState().patchStatus({ busy: null, progress: null, nodes: g.order, edges: g.size });
  // What arrived has no position until a layout runs; 'keep' leaves the camera alone.
  await layOut(view?.layout ?? 'layered', view ? stretchOf(view) : { x: 1, y: 1 }, 'keep');
}

/**
 * Above this many absent claims, one scoped read beats a request apiece. Below it, reading the
 * closure again to collect a handful costs the whole closure.
 */
const BY_ID_BELOW = 64;

/** readMissing collects the claims a closure names and the session lacks, by whichever read is cheaper. */
async function readMissing(
  source: DataSource,
  scope: Scope,
  missing: string[],
  closureSize: number,
  store: ReturnType<typeof useExplorer.getState>,
): Promise<DrawnClaim[]> {
  if (missing.length < BY_ID_BELOW && missing.length < closureSize) {
    const claims: DrawnClaim[] = [];
    let failures = 0;
    for (const [i, id] of missing.entries()) {
      try {
        const claim = await source.claimAt(scope, id);
        if (claim) claims.push(claim);
      } catch (err) {
        // A claim the grant does not reach, or one dropped since the ids were answered — the
        // rest still draws. Reported once: a fill that fails for every claim looks identical
        // to an honest shortfall, which is how a route built from the wrong field hid.
        if (failures === 0) log(`provenance  a claim could not be read — ${String(err)}`);
        failures++;
      }
      store.patchStatus({ busy: `reading ${i + 1} of ${missing.length}`, progress: (i + 1) / missing.length });
      if (i % 8 === 0) await yieldToPaint();
    }
    if (failures > 0) log(`provenance  ${failures} of ${missing.length} could not be read`);
    return claims;
  }
  try {
    const page = await source.fetch({ limit: closureSize, scope });
    return page.claims;
  } catch (err) {
    log(`provenance  read failed — ${String(err)}`);
    return [];
  }
}
