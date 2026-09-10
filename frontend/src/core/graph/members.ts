/**
 * package: core / graph
 * type:    data
 * job:     hold the claim ids a scope contains, as the source reported them
 * limits:  storage only; which ids those are is the query engine's answer (-> ranke-go)
 *
 * Membership is not computed here. A scoped query with `output.detail: id` returns the
 * identities in that scope, and this holds the answer so the renderer can intersect it
 * with what is cached. Module scope, like the graph itself: an id set of a large branch is
 * graph data, and graph data never enters React state.
 *
 * Keyed by head (-> core/scope scopeKey), never by name: a name identifies a branch, where a
 * head identifies the closure, and two claims' provenance read within one branch share a name.
 */

const members = new Map<string, Set<string>>();

/** setMembers records the ids a scope reported, replacing any earlier answer. */
export function setMembers(head: string, ids: Iterable<string>): Set<string> {
  const set = new Set(ids);
  members.set(head, set);
  return set;
}

/** membersOf returns a scope's reported ids, or null when it has not been asked. */
export function membersOf(head: string): Set<string> | null {
  return members.get(head) ?? null;
}

/** forgetMembers drops every answer — a session reset, or a source change. */
export function forgetMembers(): void {
  members.clear();
}
