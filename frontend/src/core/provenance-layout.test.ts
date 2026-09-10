/**
 * package: core / tests
 * type:    test
 * job:     pin what a provenance view asks of the layout — the closure packed, every claim
 *          captioned, and a read that moved the axis placing everyone
 * limits:  headless; the placement itself is layout/layouts' (-> layouts.test.ts)
 *
 * Its own file rather than provenance.test.ts's: these drive the store and the union graph,
 * and node's runner gives a file its own process, so neither leaks into the tests there.
 */

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { forgetContent, rememberContent } from './content.ts';
import { CAPTION_TEXT_CHARS, captionText } from './detail.ts';
import { forgetMembers, setMembers } from './graph/members.ts';
import { clear, graph } from './graph/universe.ts';
import { LABEL_EVERY_UPTO, labelEveryClaim } from './provenance.ts';
import { provenanceScope, scopeKey } from './scope.ts';
import { defaultView, useExplorer } from './store.ts';
import type { ViewState } from './store.ts';
import { timelineContext } from './timeline.ts';

/** A provenance view over a closure of `size` claims, its membership already answered. */
function provenanceView(id: string, claim: string, size: number): ViewState {
  const scope = provenanceScope(claim, 'main');
  setMembers(
    scopeKey(scope),
    Array.from({ length: size }, (_, i) => `${claim}-anc-${i}`),
  );
  const view = defaultView(id, `provenance ${claim}`);
  view.scope = scope;
  return view;
}

/** A branch view, which packs nothing and captions by the ordinary density gate. */
function branchView(id: string): ViewState {
  const view = defaultView(id, 'main');
  view.scope = { name: 'main', head: 'branch-head', branch: 'main' };
  return view;
}

// The timeline, not a layout of its own: a provenance view is the historical view filtered to
// one closure, so the ruler, the cursor and both stretches go on meaning what they mean.
test('a provenance view draws on the timeline like any other view', () => {
  forgetMembers();
  assert.equal(provenanceView('v1', 'claim-a', 3).layout, 'timeline');
  assert.equal(branchView('v2').layout, 'timeline');
});

// A closure is read closely, so the reader wants to know what each dot is without clicking it.
// Past the ceiling a caption apiece is a wall of text, and Sigma's density gate takes over.
test('every claim of a small closure is captioned, and of a large one none forced', () => {
  forgetMembers();
  assert.ok(labelEveryClaim(provenanceView('v1', 'claim-a', LABEL_EVERY_UPTO)));
  assert.ok(!labelEveryClaim(provenanceView('v2', 'claim-b', LABEL_EVERY_UPTO + 1)));
  assert.ok(!labelEveryClaim(branchView('v3')), 'a branch view forced captions');
  assert.ok(!labelEveryClaim(defaultView('v4', 'view 1')), 'a view with no scope forced captions');
  assert.ok(!labelEveryClaim(null));
});

// The pack is derived from the view being drawn rather than passed in, so a stretch, a load and
// a provenance read all pack the same claims — and a branch view packs none of them.
test('the layout packs the active view’s closure, and a branch view’s nothing', () => {
  forgetMembers();
  const view = provenanceView('v1', 'claim-a', 3);
  useExplorer.getState().addTab(view);
  const packed = timelineContext({ x: 1, y: 1 }).pack;
  assert.equal(packed?.size, 3, 'a provenance view named no pack');

  useExplorer.getState().addTab(branchView('v2'));
  assert.equal(timelineContext({ x: 1, y: 1 }).pack, undefined, 'a branch view named a pack');
});

// A read that inserted an instant moved every claim's x with it, so placing only what arrived
// would draw two axes at once — the restriction is dropped and everyone is placed again.
test('a read that moved the axis places every claim, not the few that arrived', () => {
  clear();
  forgetMembers();
  const g = graph();
  g.addNode('a', { createdAt: 1_000, cls: 'source', claimType: 'source/letter', x: 0, y: 0 });
  timelineContext({ x: 1, y: 1 }, true);
  assert.equal(timelineContext({ x: 1, y: 1 }, true).only, true, 'a standing axis kept nothing');

  g.addNode('b', { createdAt: 5_000_000, cls: 'source', claimType: 'source/letter', x: 0, y: 0 });
  assert.equal(timelineContext({ x: 1, y: 1 }, true).only, false, 'a moved axis kept the restriction');
});

// The second line of a caption: what the claim says, where the first line says only what kind
// of thing it is. Cut to a length that reads beside a dot, and silent where there is nothing
// honest to quote — bytes the encoding does not call text, or a body no read has brought in.
test('a caption quotes the first line of what a claim says, and nothing it has not read', () => {
  clear();
  forgetContent();
  const g = graph();
  const text = (body: string) => new TextEncoder().encode(body);

  g.addNode('c1', { encoding: 'text/plain', x: 0, y: 0 });
  rememberContent('c1', text('first line\nsecond line'));
  assert.equal(captionText('c1'), 'first line');

  g.addNode('c2', { encoding: 'text/plain', x: 0, y: 0 });
  rememberContent('c2', text('x'.repeat(CAPTION_TEXT_CHARS + 20)));
  const cut = captionText('c2');
  assert.ok(cut.endsWith('…'), 'a cut line did not say it was cut');
  assert.equal(cut.length, CAPTION_TEXT_CHARS + 1);

  g.addNode('c3', { encoding: 'application/octet-stream', x: 0, y: 0 });
  rememberContent('c3', text('bytes no one asked to read as characters'));
  assert.equal(captionText('c3'), '', 'bytes the encoding does not call text were quoted');

  g.addNode('c4', { encoding: 'text/plain', x: 0, y: 0 });
  assert.equal(captionText('c4'), '', 'a claim whose content was never read quoted something');
});
