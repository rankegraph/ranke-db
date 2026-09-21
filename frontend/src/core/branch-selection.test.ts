/**
 * package: core / tests
 * type:    test
 * job:     pin the branch surface — discovery, selection, and a view confined to a closure
 * limits:  headless; no DOM, no Sigma (-> render is exercised by the render bench)
 *
 * Run with `make -C frontend test`. Node's own runner strips the types, which is how the
 * benches already run, so this needs no framework.
 */

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { DirectedGraph } from 'graphology';

import { CONTENT_LIMIT, MockSource, RestSource } from './data/source.ts';
import type { DataSource } from './data/source.ts';
import { forgetMembers, membersOf, setMembers } from './graph/members.ts';
import { ARCHIVE_SCOPE, scopeOptions } from './scope.ts';
import type { Scope } from './scope.ts';
import type { ViewState } from './store.ts';
import { DEFAULT_QUERY } from './query.ts';
import type { Connection } from './connections.ts';

const MOCK_PARAMS = { claims: 400, seed: 7, claimsPerContribution: 10 };

/** viewWith is a view carrying just the predicate under test. */
function viewWith(scope: Scope | null): ViewState {
  return {
    kind: 'graph',
    id: 'v1',
    label: 'v1',
    contributionRange: null,
    scope,
    classes: [],
    layout: 'history',
    xStretch: 1,
    yStretch: 1,
    edges: true,
    edgesOnMove: false,
    labels: true,
    labelsOnMove: true,
    sizeByDegree: true,
  };
}

/**
 * stubFetch answers one URL with one body, so the REST backend is driven over the wire
 * shape it actually parses rather than through a hand-made double of itself. The generated
 * client clones a response before parsing it, so the stub answers `clone` as well — the
 * request reaching `fetch` at all is what makes this a wire test.
 */
function stubFetch(handler: (url: string, init?: RequestInit) => { status?: number; body: unknown }) {
  const original = globalThis.fetch;
  globalThis.fetch = (async (input: string | URL | Request, init?: RequestInit) => {
    const { status = 200, body } = handler(String(input), init);
    const response: Partial<Response> = {
      ok: status >= 200 && status < 300,
      status,
      headers: new Headers(),
      json: async () => body,
      text: async () => JSON.stringify(body),
    };
    response.clone = () => response as Response;
    return response as Response;
  }) as typeof globalThis.fetch;
  return () => {
    globalThis.fetch = original;
  };
}

const restConnection: Connection = {
  id: 'c1',
  name: 'local',
  kind: 'rest',
  baseUrl: 'http://localhost:8080',
  authKind: 'none',
  remember: false,
  mock: MOCK_PARAMS,
};

// 5.1 — one port method, two backends, the same shape out of both.
test('branches list identically through the port from mock and REST', async () => {
  const mock: DataSource = new MockSource(MOCK_PARAMS);
  const fromMock = await mock.branches();

  const named = fromMock.filter((s) => s.name !== ARCHIVE_SCOPE);
  assert.ok(named.length > 0, 'the generator recorded no branches');

  const restore = stubFetch(() => ({
    body: { branches: named.map(({ name, head }) => ({ name, head })) },
  }));
  try {
    const rest: DataSource = new RestSource(restConnection, '');
    const fromRest = await rest.branches();
    assert.deepEqual(fromRest, named, 'the two backends disagree on the same branch table');
  } finally {
    restore();
  }
});

// 2.2 — the routes are the generated client's, so a read is pinned to the contract's paths
// rather than to a string in this repo. The archive head is what makes the archive a scope,
// and it comes from the one route that reports it.
test('the branch reads go out on the contract routes, archive head included', async () => {
  const asked: string[] = [];
  const restore = stubFetch((url) => {
    asked.push(new URL(url).pathname);
    return url.endsWith('/archive/info')
      ? { body: { head: 'archive-head', height: 2, updatedAt: '2024-01-01T00:00:00Z', branches: 1 } }
      : { body: { branches: [{ name: 'main', head: 'main-head', branch: 'main' }] } };
  });
  try {
    const scopes = await new RestSource(restConnection, '').branches();
    assert.deepEqual(scopes, [
      { name: ARCHIVE_SCOPE, head: 'archive-head', branch: ARCHIVE_SCOPE },
      { name: 'main', head: 'main-head', branch: 'main' },
    ]);
  } finally {
    restore();
  }
  assert.deepEqual(asked, ['/branches', '/archive/info']);
});

// 2.1 — auth stays the explorer's: the client is handed the headers the connection's kind
// calls for, and carries them on every route it builds.
test('a connection’s credential rides on a generated read', async () => {
  const sent: (HeadersInit | undefined)[] = [];
  const restore = stubFetch((_url, init) => {
    sent.push(init?.headers);
    return { body: { branches: [] } };
  });
  try {
    await new RestSource({ ...restConnection, authKind: 'apikey' }, 'the-key').branches();
  } finally {
    restore();
  }
  assert.ok(sent.length > 0, 'no request was made');
  for (const headers of sent) {
    assert.equal((headers as Record<string, string> | undefined)?.['X-API-Key'], 'the-key');
  }
});

// 5.7 — $universe has no head, so the rule that admits a scope excludes it.
test('no backend offers a $universe scope', async () => {
  const fromMock = await new MockSource(MOCK_PARAMS).branches();
  assert.ok(!fromMock.some((s) => s.name === '$universe'));
  assert.ok(fromMock.every((s) => s.head.length > 0), 'a scope was offered with no head');

  // Even asked to, the REST backend drops an entry with no head.
  const restore = stubFetch(() => ({
    body: { branches: [{ name: 'main', head: 'h1', branch: 'main' }, { name: '$universe' }] },
  }));
  try {
    const fromRest = await new RestSource(restConnection, '').branches();
    assert.deepEqual(fromRest, [{ name: 'main', head: 'h1', branch: 'main' }]);
  } finally {
    restore();
  }
});

// 4.1 / 5.7 — what the picker offers, decided in core so it is testable without a browser.
test('the picker prompts, then offers the archive and each branch by name', async () => {
  const scopes = await new MockSource(MOCK_PARAMS).branches();
  const options = scopeOptions(scopes, null);

  assert.equal(options[0].value, '', 'the first entry does not lift the confinement');
  assert.ok(options[0].selected, 'nothing is selected, yet no entry reads as selected');
  assert.equal(options.length, scopes.length + 1);
  assert.ok(!options.some((o) => o.value === '$universe'));
  // A name, and nothing else: a head is a content address, which identifies a scope to a
  // machine and tells a reader nothing. The Info pane shows it instead.
  for (const option of options.slice(1)) {
    assert.equal(option.label, option.value, `entry ${option.value} is captioned with more than its name`);
  }

  const branch = scopes.find((s) => s.name !== ARCHIVE_SCOPE) as Scope;
  const withSelection = scopeOptions(scopes, branch);
  assert.deepEqual(
    withSelection.filter((o) => o.selected).map((o) => o.value),
    [branch.name],
    'exactly one entry should read as selected',
  );
});

// 5.8 — nothing is assumed before the listing answers.
test('no branch name is assumed before discovery', () => {
  assert.equal(DEFAULT_QUERY.branch, null, 'the query still defaults to a guessed branch');
  assert.equal(viewWith(null).scope, null, 'a fresh view starts confined to something');
});

// The bulk read asks for content up to the explorer's own threshold — the same number the
// detail pane fetches under — and for whole bodies only, so no caption or cache ever holds
// a prefix. Without the ask, every claim arrives size-only and reads as having no content.
test('a claims read asks for content up to the cap, whole bodies only', async () => {
  const sent: string[] = [];
  const original = globalThis.fetch;
  globalThis.fetch = (async (_input: string | URL | Request, init?: RequestInit) => {
    sent.push(String(init?.body ?? ''));
    return new Response('');
  }) as typeof globalThis.fetch;
  try {
    const head = 'bciqdlnrhbcnkalcqxrpxpmroin6iu5w6dgfjqoemvxlvvhtwepbe6ma';
    const page = await new RestSource(restConnection, '').fetch({
      limit: 10,
      scope: { name: 'main', head, branch: 'main' },
    });
    assert.equal(page.claims.length, 0);
  } finally {
    globalThis.fetch = original;
  }
  const query = JSON.parse(sent[0]);
  assert.deepEqual(query.output.content, { max: CONTENT_LIMIT, overflow: 'omit' });
});

/**
 * The scope tests work on an id set as a source returns one, since that is what the client
 * holds: the engine answers which claims a scope contains, and this only looks in it.
 */
function graphOf(nodes: string[]): DirectedGraph {
  const g = new DirectedGraph({ allowSelfLoops: false });
  for (const node of nodes) g.addNode(node, { contribution: 1, cls: 'source' });
  return g;
}

// 5.2 — the answer narrows what is admitted; nothing is removed from the cache.
test('a scope narrows what is admitted while the cache keeps every claim', async () => {
  forgetMembers();
  const source = new MockSource(MOCK_PARAMS);
  const scopes = await source.branches();
  const branch = scopes.find((s) => s.name !== ARCHIVE_SCOPE) as Scope;
  const whole = scopes.find((s) => s.name === ARCHIVE_SCOPE) as Scope;

  const inBranch = await source.scopeIds(branch);
  const inArchive = await source.scopeIds(whole);
  assert.ok(inBranch.length > 0, 'the branch reported no claims');
  assert.ok(
    inBranch.length < inArchive.length,
    'a branch should be a proper subset of the archive',
  );

  const members = setMembers(branch.name, inBranch);
  assert.equal(members.size, new Set(inBranch).size);
  // A read of the archive scope returns what that scope contains, so the two agree.
  const page = await source.fetch({ limit: inArchive.length, scope: whole });
  assert.equal(page.claims.length, inArchive.length);
  // And a scoped read returns that scope, not a prefix of the archive.
  const scoped = await source.fetch({ limit: inBranch.length, scope: branch });
  assert.equal(scoped.claims.length, inBranch.length);
});

// 5.4 — a claim in two branches is one claim, reported under either.
test('a claim two scopes both contain is one claim in the cache', async () => {
  forgetMembers();
  const source = new MockSource(MOCK_PARAMS);
  const scopes = (await source.branches()).filter((s) => s.name !== ARCHIVE_SCOPE);
  assert.ok(scopes.length >= 2, 'need two branches to compare');

  const [a, b] = [await source.scopeIds(scopes[0]), await source.scopeIds(scopes[1])];
  const shared = a.filter((id) => b.includes(id));
  assert.ok(shared.length > 0, 'no claim is in both scopes — nothing to check');

  const g = graphOf([...new Set([...a, ...b])]);
  for (const id of shared) {
    assert.ok(g.hasNode(id), 'a shared claim is missing from the union');
  }
  assert.equal(g.order, new Set([...a, ...b]).size, 'a shared claim was duplicated');
});

// 5.3 — switching between answered scopes and back re-asks nothing and mutates nothing.
test('switching between scopes and back keeps both answers', async () => {
  forgetMembers();
  const source = new MockSource(MOCK_PARAMS);
  const scopes = (await source.branches()).filter((s) => s.name !== ARCHIVE_SCOPE);

  setMembers(scopes[0].head, await source.scopeIds(scopes[0]));
  setMembers(scopes[1].head, await source.scopeIds(scopes[1]));

  const first = membersOf(scopes[0].head);
  const second = membersOf(scopes[1].head);
  assert.ok(first && second, 'an answer was dropped on switching');
  assert.notEqual(first.size, 0);
  // Reselecting reads the held answer rather than asking again.
  assert.equal(membersOf(scopes[0].head), first);
});

// 5.5 — an unanswered scope admits everything rather than drawing blank.
test('a scope with no answer yet admits everything', () => {
  forgetMembers();
  assert.equal(membersOf('never-asked'), null);
});

// 5.6 — ids the answer names but the cache lacks are countable, not silently absent.
test('claims a scope names but the cache lacks are countable', () => {
  forgetMembers();
  const members = setMembers('main', ['cached-1', 'cached-2', 'only-on-the-server']);
  const g = graphOf(['cached-1', 'cached-2']);
  let missing = 0;
  for (const id of members) if (!g.hasNode(id)) missing++;
  assert.equal(missing, 1, 'a claim the server has and the cache does not went unnoticed');
});

// An ids-only read decodes through the library, framing included: the endpoint writes each
// identity as a bare string record (serve.go, KindClaimId), and the execution report a query
// may append is not an identity — the reader passes it over rather than this file doing so.
test('an ids-only read decodes through the library reader', async () => {
  const body =
    '\u001e"id-a"\n\u001e"id-b"\n\u001e["id-c","id-d"]\n\u001e{"started_at":"t","events":[]}\n';
  const original = globalThis.fetch;
  globalThis.fetch = (async () => new Response(body)) as typeof globalThis.fetch;
  try {
    // A real head: the query codec validates `select.head` as a multibase id before the
    // request leaves, so a stand-in that is not one is refused here rather than by a server.
    const head = 'bciqdlnrhbcnkalcqxrpxpmroin6iu5w6dgfjqoemvxlvvhtwepbe6ma';
    const ids = await new RestSource(restConnection, '').scopeIds({ name: 'main', head, branch: 'main' });
    // A route arrives flattened into the claims along it, which is the reader's doing.
    assert.deepEqual(ids, ['id-a', 'id-b', 'id-c', 'id-d']);
  } finally {
    globalThis.fetch = original;
  }
});
