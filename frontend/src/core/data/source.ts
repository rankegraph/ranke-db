/**
 * package: core / data
 * type:    interface
 * job:     the data-source port — where claims come from
 * limits:  contract only; backends are the mock generator and a REST connection (-> core/mock)
 *
 * A mock's "server details" are the generator's parameters, so configuring either is the same
 * act: one data path, not a real and a test one.
 */

import { EncodeQuery, decodeClaim, newSeqReader, readIds } from '@rankegraph/ranke';
import type { Query as RankeQuery } from '@rankegraph/ranke';
import { contributionUnknown, drawn } from '../claims.ts';
import type { DrawnClaim } from '../claims.ts';
import { generate } from '../mock/generate.ts';
import type { MockArchive } from '../mock/model.ts';
import { rememberClaimBytes } from '../claimBytes.ts';
import { ARCHIVE_SCOPE } from '../scope.ts';
import type { Scope } from '../scope.ts';
import { apiFor, probe } from '../connections.ts';
import type { Connection, MockParams, ProbeResult } from '../connections.ts';
import { Api } from './openapi.gen.ts';
import type { Error as ApiError, HttpResponse, Query as ApiQuery } from './openapi.gen.ts';

/**
 * The most content to carry per claim, on the wire and on demand alike: a bulk read inlines
 * a body up to this size, and selection fetches one only up to it. One threshold, so what
 * the bulk read withholds is exactly what the detail pane reports as too large.
 */
export const CONTENT_LIMIT = 4096;

/** What a read returns: claims, and what the source can say about them. */
export interface ClaimPage {
  claims: DrawnClaim[];
  contributions: number;
  /** Time the source spent producing the page. */
  elapsedMs: number;
  /** How the page was obtained, for the log. */
  origin: string;
}

export interface FetchRequest {
  /** Cap on claims returned — `limit.results` in the query contract. */
  limit: number;
  /**
   * Scope to read — `select.branch`. A server read requires one, the contract making a
   * scope mandatory; a generator ignores it, having one archive.
   */
  scope?: Scope | null;
  /**
   * Called as claims arrive: how many are decoded so far, and how many bytes the reader has
   * taken. The count of claims yet to come would cost a second closure walk; the bytes are
   * the reader's own tally, so they cost nothing and move even between two whole records.
   */
  onProgress?: (read: number, bytesRead: number) => void;
}

export interface DataSource {
  readonly kind: 'mock' | 'rest';
  /** One line naming what this source is, shown in the UI. */
  describe(): string;
  health(): Promise<ProbeResult>;
  /** branches returns every scope with a head: each branch, and the archive. */
  branches(): Promise<Scope[]>;
  /**
   * scopeIds asks which claims a scope contains, as identities only (`output.detail: id`).
   * The engine answers it — a branch's membership is its closure — and the client intersects
   * the id set with its cache.
   */
  scopeIds(scope: Scope): Promise<string[]>;
  fetch(request: FetchRequest): Promise<ClaimPage>;
  /**
   * content reads one claim's bytes. Content is addressed by the claim that holds it, so the
   * scope the claim was read in is the scope its content is read in — and nothing about
   * whether the bytes sit inline or in a blob reaches the caller.
   */
  content(scope: Scope | null, id: string): Promise<Uint8Array>;
  /** claimBytes reads the claim's own signed CBOR — what its id is computed over, not what it declares. */
  claimBytes(scope: Scope | null, id: string): Promise<Uint8Array>;
  /**
   * claimAt reads one claim over the claim route, for filling a shortfall — re-reading a whole
   * closure to collect the few claims a session lacks costs the closure. The bytes are the
   * claim's own, so the id is checkable against them and the CBOR tab's cache is filled too.
   */
  claimAt(scope: Scope, id: string): Promise<DrawnClaim | null>;
}

/** MockSource generates an archive locally. Its parameters are its server details. */
export class MockSource implements DataSource {
  readonly kind = 'mock' as const;

  private params: MockParams;

  constructor(params: MockParams) {
    this.params = params;
  }

  describe(): string {
    return (
      `generated · seed ${this.params.seed}, up to ${this.params.claims.toLocaleString('en-US')} ` +
      `claims, ~${this.params.claimsPerContribution} per contribution`
    );
  }

  /** A generator is always healthy; reporting so keeps the UI uniform. */
  async health(): Promise<ProbeResult> {
    return { state: 'ok', latencyMs: 0, detail: this.describe() };
  }

  /**
   * branches reads the branch table the generator recorded, describing the whole archive
   * rather than the slice a capped read returns — so a listed branch may be unloaded, as
   * against a real instance.
   */
  async branches(): Promise<Scope[]> {
    const archive = this.wholeArchive();
    return [
      { name: ARCHIVE_SCOPE, head: archive.head, branch: ARCHIVE_SCOPE },
      ...Object.entries(archive.branches).map(([name, head]) => ({ name, head, branch: name })),
    ];
  }

  /**
   * scopeIds reads the branch each claim was stamped with. An unstamped claim is shared —
   * contributors, the branch table — so it belongs to every scope.
   */
  async scopeIds(scope: Scope): Promise<string[]> {
    return scopedClaims(this.wholeArchive(), scope.branch, scope.head).map((c) => c.claim.id);
  }

  /**
   * fetch answers the scoped read, as the server does: the scope generates the result set and
   * the limit caps it. Capping the archive instead would return a prefix, which disagrees
   * with what `scopeIds` says the scope contains — the two have to describe one archive.
   */
  async fetch(request: FetchRequest): Promise<ClaimPage> {
    const archive = this.wholeArchive();
    const inScope = scopedClaims(archive, request.scope?.name);
    const claims = inScope.slice(0, request.limit);
    return {
      claims,
      contributions: archive.stats.contributions,
      elapsedMs: archive.stats.generateMs,
      origin: request.scope ? `generator · ${request.scope.name}` : 'generator',
    };
  }

  /**
   * content is only ever asked for what a generated claim does not carry. What the mock's claims
   * say is inline, so the merge already holds it (-> core/graph/build); what is left is the
   * sources, whose bytes are documents in a Universe this generator does not stand in for.
   */
  async content(): Promise<Uint8Array> {
    throw new Error(
      'a generated source declares an address and a size, not bytes — read content from a ' +
        'connection to a real instance instead.',
    );
  }

  /** claimAt hands back a generated claim, which the archive already holds decoded. */
  async claimAt(scope: Scope, id: string): Promise<DrawnClaim | null> {
    return scopedClaims(this.wholeArchive(), scope.branch).find((c) => c.claim.id === id) ?? null;
  }

  /** A generated claim was never encoded as signed CBOR — nothing honest to hand back. */
  async claimBytes(): Promise<Uint8Array> {
    throw new Error(
      'a generated claim was never encoded as signed CBOR — read it from a connection to a ' +
        'real instance instead.',
    );
  }

  /** wholeArchive is this source's full archive, whatever a read of it is capped at. */
  private wholeArchive(): MockArchive {
    return memoised(this.params, this.params.claims);
  }
}

/**
 * routeBranch picks which scope's route serves a claim. `branch`, never `name`: a provenance
 * scope is named for its claim and read within a branch, so the name is no route at all.
 */
function routeBranch(scope: Scope | null): string | null {
  if (scope === null || scope.branch === ARCHIVE_SCOPE) return null;
  return scope.branch;
}

/**
 * scopedClaims is the mock's one answer to "what is in this scope", so a read and an id
 * listing cannot disagree. An unstamped claim is shared and belongs to every scope; a name
 * the branch table does not hold is no scope, and contains nothing.
 *
 * A head narrows that to its closure, which is what a provenance scope asks for. The walk is
 * the engine's work against a server; here the mock *is* the archive, so it answers the same
 * question the server would rather than the explorer inferring it.
 */
function scopedClaims(archive: MockArchive, scope?: string, head?: string): DrawnClaim[] {
  const within =
    !scope || scope === ARCHIVE_SCOPE
      ? archive.claims
      : scope in archive.branches
        ? archive.claims.filter((c) => c.branch === scope || c.branch === '')
        : [];
  if (!head || head === archive.head || head === archive.branches[scope ?? '']) return within;
  return closureOf(within, head);
}

/** closureOf walks references from a head, which terminate: the graph is acyclic and finite. */
function closureOf(claims: DrawnClaim[], head: string): DrawnClaim[] {
  const byId = new Map(claims.map((c) => [c.claim.id, c]));
  const reached = new Set<string>();
  const pending = [head];
  while (pending.length > 0) {
    const id = pending.pop() as string;
    if (reached.has(id)) continue;
    reached.add(id);
    for (const edge of byId.get(id)?.claim.edges ?? []) pending.push(edge.reference);
  }
  return claims.filter((c) => reached.has(c.claim.id));
}

/**
 * memoised reuses the last archive when the parameters match. The generator is deterministic,
 * so this changes no answer — it stops a branch listing regenerating 300k claims.
 */
let lastKey = '';
let lastArchive: MockArchive | null = null;

function memoised(params: MockParams, claims: number): MockArchive {
  const key = `${params.seed}/${params.claimsPerContribution}/${claims}`;
  if (key !== lastKey || !lastArchive) {
    lastArchive = generate(claims, {
      seed: params.seed,
      claimsPerContribution: params.claimsPerContribution,
    });
    lastKey = key;
  }
  return lastArchive;
}

/**
 * RestSource talks to a real instance through the client generated from `openapi.yaml`, so
 * every route, path and response type it uses is the contract's rather than this file's.
 */
export class RestSource implements DataSource {
  readonly kind = 'rest' as const;

  private connection: Connection;
  private secret: string;
  private api: Api<unknown>;

  constructor(connection: Connection, secret: string) {
    this.connection = connection;
    this.secret = secret;
    this.api = apiFor(connection, secret);
  }

  describe(): string {
    return `${this.connection.baseUrl} · ${this.connection.authKind}`;
  }

  health(): Promise<ProbeResult> {
    return probe(this.connection, this.secret);
  }

  /**
   * branches reads the branch listing, plus the archive report for the branch-table head —
   * the only route reporting it, and so the only way the archive becomes a nameable scope.
   * If that read fails the archive is dropped rather than guessed.
   */
  async branches(): Promise<Scope[]> {
    const listed = await this.answer(() => this.api.branches.listBranches(), 'listing branches');
    const branches = (listed.data.branches ?? [])
      .filter((b) => Boolean(b.name && b.head))
      .map(({ name, head }) => ({ name, head, branch: name }));

    try {
      const archive = await this.answer(
        () => this.api.archive.getArchiveInfo(),
        'reading the archive',
      );
      if (archive.data.head) {
        return [{ name: ARCHIVE_SCOPE, head: archive.data.head, branch: ARCHIVE_SCOPE }, ...branches];
      }
    } catch {
      // Not grantable to this subject, or an older instance: the branches still stand.
    }
    return branches;
  }

  /**
   * scopeIds runs the scoped query: `select.branch` names the scope, `output.detail: id` asks
   * for identities. The library reads that sequence too, so nothing here knows how a record
   * of ids is framed — and a route arrives flattened into the claims along it.
   */
  async scopeIds(scope: Scope): Promise<string[]> {
    const response = await this.query(
      {
        select: { branch: scope.branch, head: scope.head },
        output: { detail: 'id', encoding: 'json' },
      },
      `reading the ids in ${scope.name}`,
    );
    const ids: string[] = [];
    for await (const id of readIds(await bodyStream(response), 'json')) ids.push(id);
    return ids;
  }

  /**
   * fetch reads claims: `detail: claims` carries each with its edges, `form: original` skips
   * diff resolution, `limit.results` caps it. Content rides along up to CONTENT_LIMIT per
   * claim, `overflow: omit` keeping each body whole or absent (R-QCONTENT); a larger claim
   * arrives size-only, its bytes waiting on the content route until it is selected. A
   * projection to draw, not the verifiable shaping (R-QCANON).
   */
  async fetch(request: FetchRequest): Promise<ClaimPage> {
    if (!request.scope) {
      throw new Error(
        'a server read needs a scope: pick a branch in the header first, since the query ' +
          'contract makes select.branch mandatory',
      );
    }
    const t0 = performance.now();
    const response = await this.query(
      {
        select: { branch: request.scope.branch, head: request.scope.head },
        output: {
          detail: 'claims',
          form: 'original',
          encoding: 'json',
          content: { max: CONTENT_LIMIT, overflow: 'omit' },
        },
        limit: { results: request.limit },
      },
      `reading ${request.scope.name}`,
    );
    // Read as it arrives rather than in one piece: 24,000 claims take long enough that a
    // reader deserves to watch the count climb, and the library's reader is a push parser,
    // so the decode is incremental anyway.
    const claims = await claimsFromBody(response, request.onProgress);
    return {
      claims,
      // A server read carries no contribution index; see contributionUnknown in core/claims.
      contributions: contributionUnknown,
      elapsedMs: performance.now() - t0,
      origin: `${request.scope.name} · query`,
    };
  }

  /**
   * content reads the claim's bytes from the route that serves them for its scope: this picks
   * which scope's route answers, and the client builds the path.
   */
  async content(scope: Scope | null, id: string): Promise<Uint8Array> {
    const branch = routeBranch(scope);
    const response = await this.answer(
      () =>
        branch === null
          ? this.api.archive.getArchiveClaimContent(id)
          : this.api.branches.getBranchClaimContent(segment(branch), id),
      `reading the content of ${id.slice(0, 12)}…`,
    );
    return new Uint8Array(await response.arrayBuffer());
  }

  /** The claim route, not content's — same scope logic, raw `Response` read directly as bytes. */
  async claimBytes(scope: Scope | null, id: string): Promise<Uint8Array> {
    const branch = routeBranch(scope);
    const response = await this.answer(
      () =>
        branch === null
          ? this.api.archive.getArchiveClaim(id)
          : this.api.branches.getBranchClaim(segment(branch), id),
      `reading the CBOR of ${id.slice(0, 12)}…`,
    );
    return new Uint8Array(await response.arrayBuffer());
  }

  /**
   * claimAt reads the claim route and decodes the bytes. Not an anchored query: an empty
   * `select.path` answers exactly as an absent one does — the frontier's whole outward closure,
   * measured against a dev instance — so capping it at one result returns some other claim.
   */
  async claimAt(scope: Scope, id: string): Promise<DrawnClaim | null> {
    const bytes = await this.claimBytes(scope, id);
    const claim = decodeClaim(bytes, id);
    rememberClaimBytes(id, bytes);
    return drawn(claim);
  }

  /** headOf reads a branch head — the one moving target in an archive. */
  async headOf(branch: string): Promise<string> {
    const reported = await this.answer(
      () => this.api.branches.getBranchHead(segment(branch)),
      `reading ${branch} head`,
    );
    // The route answers `{"head": "<id>"}`, so the id has to be read out of it — reading the
    // body as text returned the envelope.
    if (typeof reported.data.head !== 'string') {
      throw new Error(`no head in the answer for ${branch}`);
    }
    return reported.data.head;
  }

  /**
   * query posts a RankeQL query with the response format unset — the route declares `json`,
   * which would parse a *sequence* as one document. Unset, the client hands back the response
   * for the readers below. `EncodeQuery` decides what goes on the wire; its text returns as an
   * object because a string body would be JSON-encoded twice.
   */
  private query(query: RankeQuery, what: string): Promise<Response> {
    const canonical = JSON.parse(EncodeQuery(query)) as ApiQuery;
    return this.answer(() => this.api.query.query(canonical, { format: undefined }), what);
  }

  /**
   * answer runs one generated call, naming the read in whatever goes wrong. The client throws
   * the response rather than returning it, so a failure is only a status until it is told what
   * was being read.
   */
  private async answer<T>(
    call: () => Promise<HttpResponse<T, ApiError>>,
    what: string,
  ): Promise<HttpResponse<T, ApiError>> {
    try {
      return await call();
    } catch (thrown) {
      throw await failedRead(thrown, what);
    }
  }
}

/**
 * segment escapes a value the client interpolates into a route path. A claim id is base32 by
 * the contract's pattern and needs none; a branch name is an unconstrained string, so a `/`
 * or a space in one would otherwise change which route is called.
 */
function segment(value: string): string {
  return encodeURIComponent(value);
}

/**
 * failedRead names the read in whatever a generated call threw: a parsed body carries the
 * contract's `Error`, an unparsed one still has its body, and a request that never arrived
 * throws an `Error`.
 */
async function failedRead(thrown: unknown, what: string): Promise<Error> {
  if (thrown instanceof Error) return new Error(`${what} failed: ${thrown.message}`);
  if (typeof thrown !== 'object' || thrown === null) {
    return new Error(`${what} failed: ${String(thrown)}`);
  }
  const answer = thrown as HttpResponse<unknown, ApiError>;
  if (typeof answer.status !== 'number') return new Error(`${what} failed: ${String(thrown)}`);
  const detail = statedError(answer.error) || (await failureBody(answer));
  return new Error(`HTTP ${answer.status} ${what}${detail ? `: ${detail}` : ''}`);
}

/**
 * statedError reads the message out of the contract's error body: parsed, where the route asked
 * the client to parse one, or still text, where it did not.
 */
function statedError(body: unknown): string {
  if (typeof body === 'string') {
    try {
      return statedError(JSON.parse(body));
    } catch {
      return ''; // not the contract's shape, so the body itself is the best there is
    }
  }
  if (!body || typeof body !== 'object') return '';
  const { code, error } = body as Partial<ApiError>;
  return typeof error === 'string' ? error : typeof code === 'string' ? code : '';
}

/**
 * failureBody reads a failure body the client left unparsed, capped — it lands in a log line,
 * not a document.
 */
async function failureBody(response: Response): Promise<string> {
  let text = '';
  try {
    text = (await response.text()).trim();
  } catch {
    // A body already read, or none at all: the status still says what happened.
    return '';
  }
  return statedError(text) || text.slice(0, 400);
}

/** encoder turns a body already in hand into the bytes the library's readers take. */
const encoder = new TextEncoder();

/**
 * bodyStream is the response's own stream, or a one-chunk stream over a body already read —
 * which is what a stubbed response gives. The library's readers take a stream, so this is
 * where that difference stops.
 */
async function bodyStream(response: Response): Promise<ReadableStream<Uint8Array>> {
  if (response.body) return response.body;
  const bytes = encoder.encode(await response.text());
  return new ReadableStream<Uint8Array>({
    start(controller) {
      controller.enqueue(bytes);
      controller.close();
    },
  });
}

/**
 * claimsFromBody decodes a result sequence with the library's reader, reporting as it goes:
 * how many claims are decoded, and how many bytes the reader has taken. A record split across
 * two chunks is the reader's business, not this loop's.
 */
export async function claimsFromBody(
  response: Response,
  onProgress?: (read: number, bytesRead: number) => void,
): Promise<DrawnClaim[]> {
  const reader = newSeqReader('json');
  const claims: DrawnClaim[] = [];
  const chunks = (await bodyStream(response)).getReader();
  try {
    for (;;) {
      const { done, value } = await chunks.read();
      if (done) break;
      for (const claim of reader.push(value)) claims.push(drawn(claim));
      // Reported per chunk rather than per claim: the point is a moving number, not every one.
      onProgress?.(claims.length, reader.bytesRead);
    }
    for (const claim of reader.end()) claims.push(drawn(claim));
  } finally {
    chunks.releaseLock();
  }
  onProgress?.(claims.length, reader.bytesRead);
  return claims;
}

/** sourceFor builds the source a connection stands for. */
export function sourceFor(connection: Connection, secret: string): DataSource {
  return connection.kind === 'mock'
    ? new MockSource(connection.mock)
    : new RestSource(connection, secret);
}
