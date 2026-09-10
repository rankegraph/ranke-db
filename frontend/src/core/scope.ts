/**
 * package: core / scope
 * type:    data
 * job:     name the scopes a view can sit in
 * limits:  vocabulary only; membership is computed in core/graph (-> core/graph/closure)
 *
 * A scope is browsable exactly when it has a head, because a view predicate is a closure
 * test and a closure needs a root. That single rule decides the whole set: each named
 * branch has a head, the archive has the branch-table head, and `$universe` has none —
 * which is why RankeQL refuses to read there without an explicit head. So `$universe` is
 * absent by the rule rather than by a special case.
 */

/** The reserved name of the archive-wide scope, as grants and RankeQL spell it. */
export const ARCHIVE_SCOPE = '$archive';

/** A scope a view can be confined to: a name, the head whose closure it is, and where it is read. */
export interface Scope {
  /** `$archive`, a branch name, or `provenance of <claim>`. */
  name: string;
  /** What kind of closure this is. Absent means a branch or the archive, named by `name`. */
  kind?: 'provenance';
  /** The claim id this scope's closure is rooted at. */
  head: string;
  /**
   * The scope a query names to read this one (`select.branch`, R-QSCOPE). A branch and the
   * archive are read within themselves; a claim's provenance within the branch it was reached
   * through, since a claim id is a head and never a scope name.
   */
  branch: string;
}

/** isArchive reports whether a scope is the archive-wide one rather than a branch. */
export function isArchive(scope: Scope): boolean {
  return scope.name === ARCHIVE_SCOPE;
}

/** How a provenance scope's name opens, which is also how one is recognised again. */
const PROVENANCE_PREFIX = 'provenance of ';

/**
 * provenanceScope names a claim's closure as a scope: the claim is the head, and the branch it
 * was reached through is what the query names, a claim id being no scope name of its own.
 */
export function provenanceScope(claimId: string, branch: string): Scope {
  return { name: `${PROVENANCE_PREFIX}${shortHead(claimId)}`, head: claimId, branch, kind: 'provenance' };
}

/**
 * isProvenance reports whether a scope is a claim's closure rather than a branch or the
 * archive. Read off `kind`, never the name: a name is a label, and two claims can share the
 * abbreviated one the label is built from.
 */
export function isProvenance(scope: Scope): boolean {
  return scope.kind === 'provenance';
}

/**
 * scopeKey is what a remembered membership answer is filed under. The head, because that is
 * what identifies a closure: two claims' provenance read within one branch share its name.
 */
export function scopeKey(scope: Scope): string {
  return scope.head;
}

/** scopeLabel is how a scope reads in the picker: the name it is written as everywhere else. */
export function scopeLabel(scope: Scope): string {
  return scope.name;
}

/** shortHead abbreviates a head id for a label, where the full id would not fit. */
export function shortHead(head: string): string {
  return head.length > 12 ? `${head.slice(0, 12)}…` : head;
}

/** One entry of the scope picker. The empty value lifts the confinement. */
export interface ScopeOption {
  /** Scope name, or '' for everything loaded. */
  value: string;
  label: string;
  selected: boolean;
}

/**
 * scopeOptions is what the picker offers: everything loaded, then each scope with the head
 * its closure is rooted at. Composed here rather than in the view, so what the picker
 * offers is decided where the rule lives and can be tested without a browser.
 *
 * A provenance scope is never among the listed branches — there is one per claim — so when a
 * provenance view is active it gets an entry of its own. Without it the picker would match no
 * option and fall back to displaying a branch the view is not confined to.
 */
export function scopeOptions(scopes: Scope[], selected: Scope | null): ScopeOption[] {
  const listed = scopes.some((s) => selected && s.name === selected.name);
  return [
    // A prompt rather than a description: with nothing chosen this is the caption a reader
    // sees, and it should say what to do.
    { value: '', label: 'select branch…', selected: selected === null },
    ...(selected && !listed
      ? [{ value: selected.name, label: scopeLabel(selected), selected: true }]
      : []),
    // The name alone. A head is a 100-character content address, which identifies a scope
    // to a machine and tells a reader nothing — the Info pane has room to show it.
    ...scopes.map((scope) => ({
      value: scope.name,
      label: scopeLabel(scope),
      selected: selected?.name === scope.name,
    })),
  ];
}
