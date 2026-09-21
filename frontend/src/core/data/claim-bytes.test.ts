/**
 * package: core / tests
 * type:    test
 * job:     pin claimBytes — the route it picks per scope, and that a mock refuses honestly
 * limits:  the wire shape is the generated client's; this pins which route this port calls
 */

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { MockSource, RestSource } from './source.ts';
import type { DataSource } from './source.ts';
import { ARCHIVE_SCOPE, provenanceScope } from '../scope.ts';
import type { Connection } from '../connections.ts';

const MOCK_PARAMS = { claims: 50, seed: 1, claimsPerContribution: 10 };

/** stubBytes answers every request with the same bytes, and records the path asked. */
function stubBytes(bytes: Uint8Array, asked: string[]) {
  const original = globalThis.fetch;
  globalThis.fetch = (async (input: string | URL | Request) => {
    asked.push(new URL(String(input)).pathname);
    const response: Partial<Response> = {
      ok: true,
      status: 200,
      headers: new Headers(),
      arrayBuffer: async () =>
        bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer,
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

test('claimBytes reads the branch route for a named scope and the archive route otherwise', async () => {
  const bytes = new Uint8Array([1, 2, 3, 4]);
  const asked: string[] = [];
  const restore = stubBytes(bytes, asked);
  try {
    const rest: DataSource = new RestSource(restConnection, '');

    const fromBranch = await rest.claimBytes({ name: 'main', head: 'h', branch: 'main' }, 'claim-1');
    assert.deepEqual([...fromBranch], [...bytes]);

    const fromArchive = await rest.claimBytes({ name: ARCHIVE_SCOPE, head: 'h', branch: ARCHIVE_SCOPE }, 'claim-1');
    assert.deepEqual([...fromArchive], [...bytes]);

    const fromNull = await rest.claimBytes(null, 'claim-1');
    assert.deepEqual([...fromNull], [...bytes]);
  } finally {
    restore();
  }
  assert.deepEqual(asked, [
    '/branches/main/claims/claim-1',
    '/archive/claims/claim-1',
    '/archive/claims/claim-1',
  ]);
});

// The route comes from the branch a scope is read within, never its name: a provenance scope
// is named for its claim, so a name-derived route asks for a branch no archive holds — and the
// by-id fill that calls this swallows the 404 as an ordinary unreadable claim.
test('a provenance scope reads the route of the branch it is read within', async () => {
  const bytes = new Uint8Array([9, 9]);
  const asked: string[] = [];
  const restore = stubBytes(bytes, asked);
  try {
    const rest: DataSource = new RestSource(restConnection, '');
    await rest.claimBytes(provenanceScope('claim-xyz', 'main'), 'claim-1');
    await rest.claimBytes(provenanceScope('claim-xyz', ARCHIVE_SCOPE), 'claim-1');
  } finally {
    restore();
  }
  assert.deepEqual(asked, ['/branches/main/claims/claim-1', '/archive/claims/claim-1']);
});

// A generated claim is a record this generator built in memory, never signed CBOR — there
// are no bytes to hand back, and making some up would be a lie about what a mock is.
test('a generated source refuses claimBytes rather than inventing signed bytes', async () => {
  const mock: DataSource = new MockSource(MOCK_PARAMS);
  await assert.rejects(() => mock.claimBytes(null, 'claim-1'));
});
