/**
 * package: core / connections
 * type:    data
 * job:     hold the ranke-db instances the explorer can talk to, and their credentials
 * limits:  an address book, plus the health check that says whether an entry answers;
 *          reading an archive is core/data/source's
 *
 * A static bundle holding several instances at once, or none (mock data). Auth kinds
 * mirror `openapi/openapi.yaml`. Secrets stay in memory unless `remember` opts in —
 * anything in `localStorage` is one XSS from lost.
 */

import { create } from 'zustand';

import { Api } from './data/openapi.gen.ts';

export type AuthKind = 'none' | 'apikey' | 'jwt' | 'macaroon';

export const AUTH_LABELS: Record<AuthKind, string> = {
  none: 'no auth',
  apikey: 'API key',
  jwt: 'JWT bearer',
  macaroon: 'macaroon',
};

/** How a secret is described in the UI, per kind. */
export const AUTH_SECRET_LABELS: Record<AuthKind, string | null> = {
  none: null,
  apikey: 'API key',
  jwt: 'bearer token',
  macaroon: 'macaroon',
};

/**
 * A mock connection's "server details": `seed` and `claimsPerContribution` say *which*
 * archive this is, `claims` how large it may get — a ceiling, not a read's `limit`.
 */
export interface MockParams {
  /** Ceiling on the archive's size. */
  claims: number;
  claimsPerContribution: number;
  seed: number;
}

export interface Connection {
  id: string;
  name: string;
  /** `mock` is a generator standing in for an instance, read through the same port. */
  kind: 'mock' | 'rest';
  /** Base URL of a ranke-db instance, e.g. `http://localhost:8080`. Unused when mock. */
  baseUrl: string;
  authKind: AuthKind;
  /** Persist the secret to localStorage. Off by default, deliberately. */
  remember: boolean;
  /** Generator parameters, meaningful when `kind` is `mock`. */
  mock: MockParams;
}

/**
 * The generator's defaults. `claims` is a ceiling on the archive, and it is deliberately not
 * the largest one that works: every claim is built, encoded and hashed by the library, which
 * costs ~0.12 ms — so 10k generates in about a second and 100k in twelve, with nothing to look
 * at until it finishes. Raise it in the Server pane when the point is scale; the benches measure
 * the ceiling and report what it costs.
 */
export const DEFAULT_MOCK: MockParams = { claims: 10000, claimsPerContribution: 10, seed: 0x5eed };

/** Where `make dev` serves, which is what a local dev instance answers on. */
export const LOCAL_DEV_URL = 'http://localhost:8080';

/** builtInMock is a generator: an archive to look at with no server running. */
export function builtInMock(): Connection {
  return {
    id: 'mock-local',
    name: 'mock archive',
    kind: 'mock',
    baseUrl: '',
    authKind: 'none',
    remember: false,
    mock: { ...DEFAULT_MOCK },
  };
}

/**
 * builtInLocalDev is the instance `make dev` starts, configured as it is configured: the
 * minimal example serves on :8080 with a noauth endpoint, so no-auth and that URL are not
 * defaults to fall back on but the actual settings.
 *
 * Present from the first run so developing against a local server needs no setup — and
 * harmless with none running, since a connection is only dialled when it is used or probed.
 */
export function builtInLocalDev(): Connection {
  return {
    id: 'local-dev',
    name: 'local dev',
    kind: 'rest',
    baseUrl: LOCAL_DEV_URL,
    authKind: 'none',
    remember: false,
    mock: { ...DEFAULT_MOCK },
  };
}

/**
 * selfOrigin is this page's own origin when a ranke-db instance is what served it — true
 * only at /explorer, the exact path its embed mounts (-> ../../adapters/endpoints/
 * rest_http). Vite's dev server never serves that path, so `make -C frontend dev` reaches
 * this the same as before: null, falling through to the mock/local-dev defaults. `window`
 * itself is absent under Node's test runner, which is core's — no DOM, no browser globals.
 */
export function selfOrigin(): string | null {
  if (typeof window === 'undefined') return null;
  return window.location.pathname === '/explorer' ? window.location.origin : null;
}

/** builtInSelf is the live instance that served this page (-> selfOrigin). */
function builtInSelf(origin: string): Connection {
  return {
    id: 'self',
    name: 'this server',
    kind: 'rest',
    baseUrl: origin,
    authKind: 'none',
    remember: false,
    mock: { ...DEFAULT_MOCK },
  };
}

export type ProbeState = 'unknown' | 'probing' | 'ok' | 'failed';

export interface ProbeResult {
  state: ProbeState;
  /** Round-trip of the health request, in ms. */
  latencyMs?: number;
  detail?: string;
  /** Whatever `/health` reported, verbatim. */
  body?: string;
  /** The server build, as `/health` named it. Absent where the body did not parse. */
  version?: string;
}

interface ConnectionsState {
  connections: Connection[];
  /** The connection in use. A mock generator counts as one. */
  activeId: string | null;
  probes: Record<string, ProbeResult>;

  addConnection: (c: Omit<Connection, 'id'>) => string;
  updateConnection: (id: string, patch: Partial<Connection>) => void;
  removeConnection: (id: string) => void;
  activate: (id: string | null) => void;
  setSecret: (id: string, secret: string) => void;
  secretOf: (id: string) => string;
  setProbe: (id: string, probe: ProbeResult) => void;
}

const STORE_KEY = 'ranke-explorer/connections';
const SECRET_KEY = 'ranke-explorer/secrets';

/** In-memory secrets, keyed by connection id. Never written unless `remember`. */
const secrets = new Map<string, string>();

interface Persisted {
  connections: Connection[];
  activeId: string | null;
}

function loadPersisted(): Persisted {
  try {
    const raw = localStorage.getItem(STORE_KEY);
    const parsed = raw ? (JSON.parse(raw) as Persisted) : null;
    const remembered = JSON.parse(localStorage.getItem(SECRET_KEY) ?? '{}') as Record<string, string>;
    for (const [id, secret] of Object.entries(remembered)) secrets.set(id, secret);
    if (parsed && Array.isArray(parsed.connections) && parsed.connections.length > 0) {
      // Older entries predate `kind`/`mock`; fill them in rather than discard them.
      const connections = parsed.connections.map((c) => ({
        ...c,
        kind: c.kind ?? 'rest',
        mock: c.mock ?? { ...DEFAULT_MOCK },
      }));
      if (!connections.some((c) => c.id === 'local-dev')) connections.push(builtInLocalDev());
      return { connections, activeId: parsed.activeId ?? connections[0].id };
    }
  } catch {
    // A corrupt or unavailable store is not worth failing the app over.
  }
  // The server that served this page, if any, is live by construction — default to it
  // rather than the generator. Otherwise the same defensive default as before: an
  // explorer opened some other way, against a server that may not be running, would
  // report a failure before the user asked for anything.
  const self = selfOrigin();
  const mock = builtInMock();
  if (self) return { connections: [builtInSelf(self), mock, builtInLocalDev()], activeId: 'self' };
  return { connections: [mock, builtInLocalDev()], activeId: mock.id };
}

function persist(state: ConnectionsState): void {
  try {
    const payload: Persisted = { connections: state.connections, activeId: state.activeId };
    localStorage.setItem(STORE_KEY, JSON.stringify(payload));
    const remembered: Record<string, string> = {};
    for (const c of state.connections) {
      if (c.remember) {
        const secret = secrets.get(c.id);
        if (secret) remembered[c.id] = secret;
      }
    }
    localStorage.setItem(SECRET_KEY, JSON.stringify(remembered));
  } catch {
    // Private-mode browsers refuse writes; the session still works in memory.
  }
}

let counter = 0;

const initial = loadPersisted();

export const useConnections = create<ConnectionsState>((set, get) => ({
  connections: initial.connections,
  activeId: initial.activeId,
  probes: {},

  addConnection: (c) => {
    const id = `conn-${Date.now().toString(36)}-${++counter}`;
    set((s) => ({ connections: [...s.connections, { ...c, id }] }));
    persist(get());
    return id;
  },

  updateConnection: (id, patch) => {
    set((s) => ({
      connections: s.connections.map((c) => (c.id === id ? { ...c, ...patch } : c)),
    }));
    persist(get());
  },

  removeConnection: (id) => {
    secrets.delete(id);
    set((s) => ({
      connections: s.connections.filter((c) => c.id !== id),
      activeId: s.activeId === id ? null : s.activeId,
    }));
    persist(get());
  },

  activate: (activeId) => {
    set({ activeId });
    persist(get());
  },

  setSecret: (id, secret) => {
    secrets.set(id, secret);
    persist(get());
  },

  secretOf: (id) => secrets.get(id) ?? '',

  setProbe: (id, probe) => set((s) => ({ probes: { ...s.probes, [id]: probe } })),
}));

/** activeConnection returns the connection in use, falling back to the first. */
export function activeConnection(): Connection | null {
  const { connections, activeId } = useConnections.getState();
  return connections.find((c) => c.id === activeId) ?? connections[0] ?? null;
}

/** authHeaders builds the request headers a connection's auth kind calls for. */
export function authHeaders(connection: Connection, secret: string): Record<string, string> {
  switch (connection.authKind) {
    case 'apikey':
      return secret ? { 'X-API-Key': secret } : {};
    case 'jwt':
      return secret ? { Authorization: `Bearer ${secret}` } : {};
    case 'macaroon':
      return secret ? { Authorization: `Macaroon ${secret}` } : {};
    case 'none':
      return {};
  }
}

/**
 * apiBase is a connection's base URL as a client joins route paths onto it: no trailing
 * slash, since every route in the contract begins with one.
 */
export function apiBase(connection: Connection): string {
  return connection.baseUrl.replace(/\/+$/, '');
}

/**
 * apiFor builds the generated client a connection is read through — one per connection,
 * because a connection *is* a base URL and a credential, and that credential rides in a
 * header on every route. Auth stays the explorer's: which kinds an instance accepts follows
 * from how it was configured, so the client is handed headers rather than asked to negotiate.
 */
export function apiFor(connection: Connection, secret: string): Api<unknown> {
  return new Api<unknown>({
    baseUrl: apiBase(connection),
    baseApiParams: { headers: authHeaders(connection, secret) },
  });
}

/**
 * versionOf reads the build a health body names. The body is handed back unparsed so a
 * probe reports whatever answered, so this parses defensively: an instance too old to
 * carry the field, or a proxy answering with something else entirely, leaves it absent.
 */
function versionOf(body: string): string | undefined {
  try {
    const parsed = JSON.parse(body) as { version?: unknown };
    return typeof parsed.version === 'string' ? parsed.version : undefined;
  } catch {
    return undefined;
  }
}

/**
 * probe checks a connection by asking what the health route reports, which every instance
 * serves. It is the only request the explorer makes before the user asks for data, and it
 * reports what came back rather than interpreting it — hence the response format left unset,
 * which hands back the answer with its body still to read.
 */
export async function probe(connection: Connection, secret: string): Promise<ProbeResult> {
  const t0 = performance.now();
  try {
    const answer = await apiFor(connection, secret).health.health({ format: undefined });
    const body = await answer.text();
    return {
      state: 'ok',
      latencyMs: performance.now() - t0,
      body: body.slice(0, 400),
      version: versionOf(body),
    };
  } catch (thrown) {
    const latencyMs = performance.now() - t0;
    // The client throws the response where the instance refused, and an Error where the
    // request never got an answer at all. A CORS rejection and a dead host are
    // indistinguishable from here; say so rather than guessing, because the fix differs
    // completely.
    if (thrown instanceof Error) {
      return {
        state: 'failed',
        latencyMs,
        detail: `unreachable, or blocked by CORS (${String(thrown)})`,
      };
    }
    const answer = thrown as Response;
    return {
      state: 'failed',
      latencyMs,
      detail: `HTTP ${answer.status}`,
      body: (await answer.text().catch(() => '')).slice(0, 400),
    };
  }
}
