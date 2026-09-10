/* eslint-disable */
/* tslint:disable */
// @ts-nocheck
/*
 * ---------------------------------------------------------------
 * ## THIS FILE WAS GENERATED VIA SWAGGER-TYPESCRIPT-API        ##
 * ##                                                           ##
 * ## AUTHOR: acacode                                           ##
 * ## SOURCE: https://github.com/acacode/swagger-typescript-api ##
 * ---------------------------------------------------------------
 */

/** A read, evaluated in a fixed logical order: select generates the result set, where filters it, order sorts it, limit truncates it, output shapes and encodes each surviving claim (R-QEVAL). */
export interface Query {
  /** A generator in four independent parts: branch is the scope, head the closure read, claim where the walk starts, path the traversal. Scope and start are independent because a walk runs both ways — a uses step reaches the claims that cite the current one, which lie above it, so the closure decides a reverse step's answer. */
  select: Select;
  /** A boolean tree. Each node is exactly one of the and / or / not combinators over sub-trees, or a leaf naming a field and its test. Within a where, or is boolean; across generators it unions whole result sets. A leaf may name any field a claim carries, height included (V-HEIGHT). */
  where?: Where;
  /** Shapes each result along orthogonal axes. detail: claims with form: original and encoding: cbor reproduces the canonical serialization S(v) a claim's id is computed over, and is the only output form directly verifiable against that id (R-QCANON). */
  output?: Output;
  /** Sort keys applied in priority order. Claims lacking a key's field sort last, and the archive's natural (created_at, id) order breaks any remaining ties, so the sort always resolves to a total order (R-QSORT). */
  order?: Order;
  /** Bounds the read. A read cut short by either bound is a complete answer to the query as bounded, not an error (R-QLIMIT). */
  limit?: Limit;
  /** Where the query runs and how it reports on itself. These controls reach execution and diagnostics, and never the result set. */
  execution?: Execution;
}

/**
 * Diagnostic report emitted as the **final item** of a `POST /query` sequence
 * when `execution.report` names a verbosity. Typed distinctly from result items,
 * so a reader never mistakes it for data, and always last.
 */
export interface QueryReport {
  /**
   * Wall clock at query start.
   * @format date-time
   */
  started_at?: string;
  /**
   * Total execution time in nanoseconds. The unit is in the name because the
   * value is a bare integer, and it is nanoseconds because a `trace` report
   * exists to show steps a millisecond would round away.
   */
  elapsed_ns?: number;
  /** Number of result items emitted before this report. */
  results?: number;
  /** True if `limit.results` or `limit.time` cut the read short. */
  truncated?: boolean;
  /** The ordered, multi-engine execution log, at the requested verbosity. */
  events?: QueryEvent[];
}

/**
 * One entry in a query's execution log — a stage, a routing decision, or a
 * translation.
 */
export interface QueryEvent {
  /** Offset from `started_at` in nanoseconds. */
  at_ns?: number;
  /** Who emitted it (e.g. native, cypher, stack, partition). */
  engine?: string;
  /** What it did (e.g. load-root, step, filter, sort, route, translate-cypher). */
  op?: string;
  /**
   * The entry's own level; a report carries everything at or above the
   * verbosity `execution.report` asked for.
   */
  level?: "error" | "warn" | "info" | "debug" | "trace";
  /** Elapsed time for a timed step, in nanoseconds; 0 for a point event. */
  duration_ns?: number;
  /** Human-readable message, or the translated query text (e.g. the Cypher). */
  detail?: string;
  /** Structured extras — layer or shard name, depth, edge and result counts, … */
  attrs?: Record<string, any>;
}

/** The outcome of a contribution — the new branch-table head and the appended claim ids. */
export interface ContributionResult {
  /** The new branch-table head id after the merge. */
  head: string;
  /** Content-addressed ids of the contributed claims, in order. */
  ids: string[];
}

/**
 * Requests the dev sequencer's clock advance to (at least) this instant. A
 * request older than the clock's current position is accepted as a no-op —
 * the clock only ever moves forward, since a merge's witnessed time
 * regressing would break every guarantee built on it.
 */
export interface DevClockAdvance {
  /**
   * The instant to advance to.
   * @format date-time
   */
  time: string;
}

/**
 * The dev sequencer's clock after the request — the later of what was asked
 * and what it already held.
 */
export interface DevClock {
  /** @format date-time */
  time: string;
}

export interface BranchHead {
  /** The content-addressed head claim id. */
  head: string;
}

export interface BranchInfo {
  /** The branch name, as the branch table holds it. */
  name: string;
  /** The content-addressed head claim id. */
  head: string;
  /**
   * The head claim's generation number (§4.1) — 0 on an initial node, else
   * 1 + max over what it references. The depth of what the branch points at.
   * @format int64
   */
  height: number;
  /**
   * The head claim's `created_at` — when the branch last moved. Soft: a
   * contributor writes it, so it is not the witnessed merge time.
   * @format date-time
   */
  updatedAt: string;
}

export interface ArchiveInfo {
  /**
   * The branch-table head id — the root of every `$archive`-scoped read, and the
   * only place this contract reports it.
   */
  head: string;
  /**
   * The branch-table head claim's generation number.
   * @format int64
   */
  height: number;
  /**
   * The branch-table head's `created_at` — when the archive last moved.
   * @format date-time
   */
  updatedAt: string;
  /** How many branches the table holds. */
  branches: number;
}

export interface BranchEntry {
  /** The branch name, as the branch table holds it. */
  name: string;
  /** That branch's current head claim id. */
  head: string;
}

export interface BranchList {
  /**
   * Every branch the branch table holds, from one archive snapshot — so the
   * heads are consistent with each other. Empty on an archive with no branches.
   */
  branches: BranchEntry[];
}

export interface Health {
  /** @example "ok" */
  status: string;
  /**
   * The server build answering this request. A release names itself exactly;
   * a build from a checkout names its revision, marked when the tree carried
   * uncommitted changes.
   * @example "v1.19.2"
   */
  version: string;
  /**
   * The contributor identity this stack signs merges with.
   * @example "did:key:z6Mk..."
   */
  signer?: string;
}

export interface Subject {
  /**
   * The account the credential resolved to.
   * @example "webapp"
   */
  account: string;
  /**
   * The account's grants, each a `RIGHTS glob` spec as the configuration
   * states it. An account holding none reports an empty array.
   * @example ["CR foo_*","R $archive"]
   */
  grants: string[];
  /**
   * Attenuations carried by this credential, narrowing the grants above.
   * Every caveat must allow a request for it to pass. Empty where the
   * credential carries none.
   * @example ["R foo_bar"]
   */
  caveats: string[];
}

export interface StorageLayer {
  name: string;
  /** Adapter type (e.g. memory, filesystem, s3, redis, neo4j). */
  type: string;
}

export interface StorageLayerList {
  /** Read-through tiers, top (cache) to bottom (authoritative). */
  layers: StorageLayer[];
}

/**
 * Parameters for a verification run — the same shape whether declared in the
 * stack config (scheduled) or posted ad-hoc. Depths are those of
 * core-verification.
 */
export interface VerificationConfig {
  /** Root of the closure to verify — a branch name or a claim id. */
  closure: string;
  /**
   * Storage layer (by name) to read directly. Omit to read the composed
   * read-through view — which may mask the loss of an object on a deeper
   * layer, so name a layer to verify it without blind spots.
   */
  layer?: string;
  /**
   * completeness (a `has` sweep), record-correctness (recanonicalise and
   * recheck the id-chain and signatures), or full-content (also re-hash blobs).
   */
  depth?: "completeness" | "record-correctness" | "full-content";
  /**
   * For full-content, the max content size in bytes to re-read and re-hash
   * per claim; larger content is skipped this run. Omit to verify all content.
   */
  contentThreshold?: number;
}

/** A point-in-time record of a verification run; embeds the config that produced it. */
export interface VerificationReport {
  id: string;
  /**
   * Parameters for a verification run — the same shape whether declared in the
   * stack config (scheduled) or posted ad-hoc. Depths are those of
   * core-verification.
   */
  config: VerificationConfig;
  /**
   * The head id the run verified — the closure it pinned at start. When
   * `config.closure` names a branch, this is the branch's head at start time;
   * the report stays fixed to it even as the branch head moves on.
   */
  head: string;
  /**
   * running (in progress), complete (finished on its own), stopped (cancelled
   * by an operator via the cancel action — partial findings kept), or error
   * (the run itself failed).
   */
  status: "running" | "complete" | "stopped" | "error";
  /** @format date-time */
  startedAt: string;
  /** @format date-time */
  completedAt?: string;
  /** Claims checked so far; advances while `status` is `running`. */
  claimsChecked?: number;
  /** Content bytes re-read so far; advances while `status` is `running`. */
  bytesRead?: number;
  /** True when no failures were found in this run. */
  ok: boolean;
  /**
   * The failures found, accumulating while `status` is `running`. Empty (with
   * `ok: true`) for an intact closure; a healthy archive verifies with none.
   */
  failures?: VerificationFailure[];
}

export interface VerificationReportList {
  reports: VerificationReport[];
}

export interface VerificationFailure {
  /** The claim or object id that failed. */
  id: string;
  /**
   * corrupt-bytes — stored bytes don't match their hash (storage rot/loss;
   * self-heals via read-through if a deeper layer is intact).
   * invalid-content — the claim itself doesn't validate (e.g. bad signature);
   * unrepairable.
   */
  mode: "corrupt-bytes" | "invalid-content";
  /** The layer where the failure was observed. */
  layer: string;
  detail?: string;
}

export interface Error {
  /**
   * Machine-readable failure category, stable across releases — one of
   * unauthenticated, forbidden, not_found, conflict, busy, invalid,
   * unimplemented, internal. Clients branch on this, not on the message.
   */
  code: string;
  /** Human-readable message. Carries no subject id, even on 403. */
  error: string;
}

/**
 * A claim id: id(v) = Sign(H(S(v))) for a node, id(e) = H(S(e)) for an edge, carried as multibase base32 of the self-describing payload. The pattern fixes the multibase framing; whether the payload's multihash or multikey framing parses is the implementation's check.
 * @minLength 2
 * @pattern ^b[a-z2-7]+$
 */
export type Id = string;

/**
 * A glob over class/sub, e.g. derivation/* or entity/person. A leading - excludes. Exclusion decides: a type matching an excluded pattern is refused whatever the included patterns say, and a list of exclusions alone admits every other type (R-QSTEPS).
 * @minLength 1
 */
export type TypeGlob = string;

/** One section of a path. edges bounds the walk: every hop must follow an edge whose type is listed. nodes bounds the answer: a step yields a claim only when its node's type is listed, whatever the nodes it crossed to reach it. min and max bound the hops. A min above a bounded max is refused by the implementation — a JSON Schema cannot compare two sibling values (R-QSTEPS). */
export interface PathStep {
  /** Edge types every hop must match. */
  edges?: TypeGlob[];
  /** provenance follows references outward, uses runs to the claims that cite this one, connections either way. Absent, provenance. */
  dir?: "provenance" | "uses" | "connections";
  /**
   * Fewest hops. Absent, 1 — a step moves at least one hop. 0 also yields the starting set, carrying the frontier through alongside what lies beyond it.
   * @min 0
   */
  min?: number;
  /**
   * Most hops. 0, or absent, leaves the step unbounded: a step of at most zero hops would move nothing, so that reading has no use.
   * @min 0
   */
  max?: number;
  /** Node types the step may yield. */
  nodes?: TypeGlob[];
}

/** A generator in four independent parts: branch is the scope, head the closure read, claim where the walk starts, path the traversal. Scope and start are independent because a walk runs both ways — a uses step reaches the claims that cite the current one, which lie above it, so the closure decides a reverse step's answer. */
export type Select = {
  /**
   * The mandatory scope, and every scope names a graph: a branch name confines to that branch, $archive to the whole Ranke-Archive, $universe applies no confinement and is privileged. An empty value is refused (R-QSCOPE).
   * @minLength 1
   */
  branch: string;
  /** The closure read: the query sees the intersection of the scope's graph and closure(head), so a head narrows a query. Required under $universe, which confines nothing and so offers no head to fall back on, and may name any claim the Universe holds there; optional under every other scope, where the scope's own head serves (R-QHEAD). */
  head?: Id;
  /** Anchors the frontier at the single claim it names, which must lie inside the closure. Absent, the frontier is every claim in the closure and the path is unanchored (R-QANCHOR). */
  claim?: Id;
  /** The traversal: a sequence of steps over frontiers, each frontier a set of claims. Each step's yield is the frontier the next starts from, and the no-repeat rule holds within a step and resets at each boundary, so membership is all a frontier carries (R-QFRONTIER). Absent, the generator returns the full outward closure of the frontier (R-QSTEPS). */
  path?: PathStep[];
};

/** A boolean tree. Each node is exactly one of the and / or / not combinators over sub-trees, or a leaf naming a field and its test. Within a where, or is boolean; across generators it unions whole result sets. A leaf may name any field a claim carries, height included (V-HEIGHT). */
export type Where =
  | {
      /** @minItems 1 */
      and: Where[];
    }
  | {
      /** @minItems 1 */
      or: Where[];
    }
  | {
      /** A boolean tree. Each node is exactly one of the and / or / not combinators over sub-trees, or a leaf naming a field and its test. Within a where, or is boolean; across generators it unions whole result sets. A leaf may name any field a claim carries, height included (V-HEIGHT). */
      not: Where;
    }
  | {
      /**
       * The field tested — any field a claim carries, height included.
       * @minLength 1
       */
      field: string;
      /** One operator applied to one field. eq, ne, lt, le, gt and ge take a value, in a set, glob a shell-style wildcard. Exactly one is present. */
      test: Comparison;
    };

/** A value a comparison tests against. Where it tests a time it MUST be a V-TIME timestamp or an EDTF Level 1 value, and anything else is rejected rather than coerced (R-QTIMEOP); otherwise how two values compare is the engine's. */
export type Value = any;

/** One operator applied to one field. eq, ne, lt, le, gt and ge take a value, in a set, glob a shell-style wildcard. Exactly one is present. */
export interface Comparison {
  /** A value a comparison tests against. Where it tests a time it MUST be a V-TIME timestamp or an EDTF Level 1 value, and anything else is rejected rather than coerced (R-QTIMEOP); otherwise how two values compare is the engine's. */
  eq?: Value;
  /** A value a comparison tests against. Where it tests a time it MUST be a V-TIME timestamp or an EDTF Level 1 value, and anything else is rejected rather than coerced (R-QTIMEOP); otherwise how two values compare is the engine's. */
  ne?: Value;
  /** A value a comparison tests against. Where it tests a time it MUST be a V-TIME timestamp or an EDTF Level 1 value, and anything else is rejected rather than coerced (R-QTIMEOP); otherwise how two values compare is the engine's. */
  lt?: Value;
  /** A value a comparison tests against. Where it tests a time it MUST be a V-TIME timestamp or an EDTF Level 1 value, and anything else is rejected rather than coerced (R-QTIMEOP); otherwise how two values compare is the engine's. */
  le?: Value;
  /** A value a comparison tests against. Where it tests a time it MUST be a V-TIME timestamp or an EDTF Level 1 value, and anything else is rejected rather than coerced (R-QTIMEOP); otherwise how two values compare is the engine's. */
  gt?: Value;
  /** A value a comparison tests against. Where it tests a time it MUST be a V-TIME timestamp or an EDTF Level 1 value, and anything else is rejected rather than coerced (R-QTIMEOP); otherwise how two values compare is the engine's. */
  ge?: Value;
  /** Set membership. */
  in?: Value[];
  /** Shell-style wildcard. */
  glob?: string;
}

/** Inline content per claim. Absent, no content is inlined (R-QCONTENT). */
export interface OutputContent {
  /**
   * Cap in bytes on the content inlined per claim; 0 inlines every claim's content in full.
   * @min 0
   */
  max: number;
  /** What becomes of content past the cap: cutoff inlines the bytes up to it, omit inlines whole values only. Absent, omit. A claim keeps every field it carries either way (R-QCONTENT). */
  overflow?: "cutoff" | "omit";
}

/** Shapes each result along orthogonal axes. detail: claims with form: original and encoding: cbor reproduces the canonical serialization S(v) a claim's id is computed over, and is the only output form directly verifiable against that id (R-QCANON). */
export interface Output {
  /** single yields the reached endpoints, one element each; path yields routes, each running outward from the frontier claim its walk began at (R-QSHAPE). */
  shape?: "single" | "path";
  /** What each element carries: id (the id alone), claims (a record the engine assembles, shaped by form and content), or envelope (the stored bytes copied out whole, the only output a client can hash against an id; form and content do not apply, and both form: materialized and encoding: json are rejected with it). Under shape: path it applies to every claim in the route (R-QDETAIL, R-QCANON). */
  detail?: "id" | "claims" | "envelope";
  /** Which field values a claim carries: original as written, a diff-overlaid claim's delta; materialized with any contribution/diff chain resolved over the predecessor it references, recursively to a base claim. A property of the values, hence orthogonal to detail and encoding (R-QFORM). */
  form?: "original" | "materialized";
  /** Inline content per claim. Absent, no content is inlined (R-QCONTENT). */
  content?: OutputContent;
  /** json is text with content base64-encoded, cbor is binary; the same information either way (R-QENCODING). */
  encoding?: "json" | "cbor";
}

export interface OrderKey {
  /**
   * The field sorted on — any field a claim carries, height included.
   * @minLength 1
   */
  field: string;
  /** How the values compare (R-QSORT). temporal reads each value as the span of time it denotes and orders by that span's midpoint in nanoseconds, so EDTF dates and instants compare on one axis (R-QTEMPORAL). */
  compare?: "numeric" | "lexical" | "temporal";
  /** Sort direction (R-QSORT). */
  dir?: "asc" | "desc";
}

/** Sort keys applied in priority order. Claims lacking a key's field sort last, and the archive's natural (created_at, id) order breaks any remaining ties, so the sort always resolves to a total order (R-QSORT). */
export type Order = OrderKey[];

/**
 * A duration as a decimal sequence with unit suffixes — ns, us, ms, s, m, h — e.g. 5s or 1m30s. The bare 0 means unbounded.
 * @pattern ^(0|([0-9]+(\.[0-9]+)?(ns|us|ms|s|m|h))+)$
 */
export type Duration = string;

/** Bounds the read. A read cut short by either bound is a complete answer to the query as bounded, not an error (R-QLIMIT). */
export interface Limit {
  /**
   * Caps the claim count; 0 is unbounded.
   * @min 0
   */
  results?: number;
  /** The execution budget; 0 is unbounded. */
  time?: Duration;
}

/** Where the query runs and how it reports on itself. These controls reach execution and diagnostics, and never the result set. */
export interface Execution {
  /**
   * Pins the query to one named storage or execution layer; absent, the backend chooses by capability.
   * @minLength 1
   */
  layer?: string;
  /** Report verbosity: info gives high-level stages, debug routing and translation, trace per-claim detail. Set, and only then, the stream carries one final report record after the last element, typed distinctly from result claims (R-QREPORT). */
  report?: "info" | "debug" | "trace";
}

export type QueryParamsType = Record<string | number, any>;
export type ResponseFormat = keyof Omit<Body, "body" | "bodyUsed">;

export interface FullRequestParams extends Omit<RequestInit, "body"> {
  /** set parameter to `true` for call `securityWorker` for this request */
  secure?: boolean;
  /** request path */
  path: string;
  /** content type of request body */
  type?: ContentType;
  /** query params */
  query?: QueryParamsType;
  /** format of response (i.e. response.json() -> format: "json") */
  format?: ResponseFormat;
  /** request body */
  body?: unknown;
  /** base url */
  baseUrl?: string;
  /** request cancellation token */
  cancelToken?: CancelToken;
}

export type RequestParams = Omit<
  FullRequestParams,
  "body" | "method" | "query" | "path"
>;

export interface ApiConfig<SecurityDataType = unknown> {
  baseUrl?: string;
  baseApiParams?: Omit<RequestParams, "baseUrl" | "cancelToken" | "signal">;
  securityWorker?: (
    securityData: SecurityDataType | null,
  ) => Promise<RequestParams | void> | RequestParams | void;
  customFetch?: typeof fetch;
}

export interface HttpResponse<D extends unknown, E extends unknown = unknown>
  extends Response {
  data: D;
  error: E;
}

type CancelToken = Symbol | string | number;

export enum ContentType {
  Json = "application/json",
  JsonApi = "application/vnd.api+json",
  FormData = "multipart/form-data",
  UrlEncoded = "application/x-www-form-urlencoded",
  Text = "text/plain",
}

export class HttpClient<SecurityDataType = unknown> {
  public baseUrl: string = "/";
  private securityData: SecurityDataType | null = null;
  private securityWorker?: ApiConfig<SecurityDataType>["securityWorker"];
  private abortControllers = new Map<CancelToken, AbortController>();
  private customFetch = (...fetchParams: Parameters<typeof fetch>) =>
    fetch(...fetchParams);

  private baseApiParams: RequestParams = {
    credentials: "same-origin",
    headers: {},
    redirect: "follow",
    referrerPolicy: "no-referrer",
  };

  constructor(apiConfig: ApiConfig<SecurityDataType> = {}) {
    Object.assign(this, apiConfig);
  }

  public setSecurityData = (data: SecurityDataType | null) => {
    this.securityData = data;
  };

  protected encodeQueryParam(key: string, value: any) {
    const encodedKey = encodeURIComponent(key);
    return `${encodedKey}=${encodeURIComponent(typeof value === "number" ? value : `${value}`)}`;
  }

  protected addQueryParam(query: QueryParamsType, key: string) {
    return this.encodeQueryParam(key, query[key]);
  }

  protected addArrayQueryParam(query: QueryParamsType, key: string) {
    const value = query[key];
    return value.map((v: any) => this.encodeQueryParam(key, v)).join("&");
  }

  protected toQueryString(rawQuery?: QueryParamsType): string {
    const query = rawQuery || {};
    const keys = Object.keys(query).filter(
      (key) => "undefined" !== typeof query[key],
    );
    return keys
      .map((key) =>
        Array.isArray(query[key])
          ? this.addArrayQueryParam(query, key)
          : this.addQueryParam(query, key),
      )
      .join("&");
  }

  protected addQueryParams(rawQuery?: QueryParamsType): string {
    const queryString = this.toQueryString(rawQuery);
    return queryString ? `?${queryString}` : "";
  }

  private contentFormatters: Record<ContentType, (input: any) => any> = {
    [ContentType.Json]: (input: any) =>
      input !== null && (typeof input === "object" || typeof input === "string")
        ? JSON.stringify(input)
        : input,
    [ContentType.JsonApi]: (input: any) =>
      input !== null && (typeof input === "object" || typeof input === "string")
        ? JSON.stringify(input)
        : input,
    [ContentType.Text]: (input: any) =>
      input !== null && typeof input !== "string"
        ? JSON.stringify(input)
        : input,
    [ContentType.FormData]: (input: any) => {
      if (input instanceof FormData) {
        return input;
      }

      return Object.keys(input || {}).reduce((formData, key) => {
        const property = input[key];
        formData.append(
          key,
          property instanceof Blob
            ? property
            : typeof property === "object" && property !== null
              ? JSON.stringify(property)
              : `${property}`,
        );
        return formData;
      }, new FormData());
    },
    [ContentType.UrlEncoded]: (input: any) => this.toQueryString(input),
  };

  protected mergeRequestParams(
    params1: RequestParams,
    params2?: RequestParams,
  ): RequestParams {
    return {
      ...this.baseApiParams,
      ...params1,
      ...(params2 || {}),
      headers: {
        ...(this.baseApiParams.headers || {}),
        ...(params1.headers || {}),
        ...((params2 && params2.headers) || {}),
      },
    };
  }

  protected createAbortSignal = (
    cancelToken: CancelToken,
  ): AbortSignal | undefined => {
    if (this.abortControllers.has(cancelToken)) {
      const abortController = this.abortControllers.get(cancelToken);
      if (abortController) {
        return abortController.signal;
      }
      return void 0;
    }

    const abortController = new AbortController();
    this.abortControllers.set(cancelToken, abortController);
    return abortController.signal;
  };

  public abortRequest = (cancelToken: CancelToken) => {
    const abortController = this.abortControllers.get(cancelToken);

    if (abortController) {
      abortController.abort();
      this.abortControllers.delete(cancelToken);
    }
  };

  public request = async <T = any, E = any>({
    body,
    secure,
    path,
    type,
    query,
    format,
    baseUrl,
    cancelToken,
    ...params
  }: FullRequestParams): Promise<HttpResponse<T, E>> => {
    const secureParams =
      ((typeof secure === "boolean" ? secure : this.baseApiParams.secure) &&
        this.securityWorker &&
        (await this.securityWorker(this.securityData))) ||
      {};
    const requestParams = this.mergeRequestParams(params, secureParams);
    const queryString = query && this.toQueryString(query);
    const payloadFormatter = this.contentFormatters[type || ContentType.Json];
    const responseFormat = format || requestParams.format;

    return this.customFetch(
      `${baseUrl || this.baseUrl || ""}${path}${queryString ? `?${queryString}` : ""}`,
      {
        ...requestParams,
        headers: {
          ...(requestParams.headers || {}),
          ...(type && type !== ContentType.FormData
            ? { "Content-Type": type }
            : {}),
        },
        signal:
          (cancelToken
            ? this.createAbortSignal(cancelToken)
            : requestParams.signal) || null,
        body:
          typeof body === "undefined" || body === null
            ? null
            : payloadFormatter(body),
      },
    ).then(async (response) => {
      const r = response as HttpResponse<T, E>;
      r.data = null as unknown as T;
      r.error = null as unknown as E;

      const responseToParse = responseFormat ? response.clone() : response;
      const data = !responseFormat
        ? r
        : await responseToParse[responseFormat]()
            .then((data) => {
              if (r.ok) {
                r.data = data;
              } else {
                r.error = data;
              }
              return r;
            })
            .catch((e) => {
              r.error = e;
              return r;
            });

      if (cancelToken) {
        this.abortControllers.delete(cancelToken);
      }

      if (!response.ok) throw data;
      return data;
    });
  };
}

/**
 * @title ranke-db API
 * @version 0.2.0
 * @baseUrl /
 *
 * REST/HTTP binding for a single **ranke-db** stack: read the verifiable graph
 * with a declarative query, and contribute signed claims to it.
 *
 * A server hosts exactly **one stack**, launched from a config file — there is
 * no tenant or archive routing in the paths.
 *
 * ## Reads are queries
 *
 * The full read surface is `POST /query`, carrying a **RankeQL** query — the
 * declarative `Query` type the normative spec fixes (§RankeQL) — as a JSON object:
 * `select` generates the result set, `where` filters it, `order` sorts it, `limit`
 * truncates it, and `output` shapes and encodes each surviving claim. This contract
 * *binds* that type to HTTP and defines none of it; the field names, values and
 * meanings are the spec's. Cypher/GQL is **never** a client route — it is an
 * internal execution engine the planner lowers a query to.
 *
 * A **cacheable GET subset** covers the by-id reads without a query body. Every one
 * of them is a **scope** followed by what is read within it, so the three scopes the
 * access model reserves read alike and differ only in which scope they name:
 *
 *   - `GET /branches` — the branch table's branches, by name and head
 *   - `GET /branches/{branch}/head`
 *   - `GET /branches/{branch}/claims/{id}` and its `/content` form
 *   - `GET /archive/claims/{id}` and its `/content` form — the `$archive` scope
 *   - `GET /universe/claims/{id}` and its `/content` form — the privileged
 *     `$universe` scope
 *
 * `GET /branches` needs no branch name, so a client discovers what it may address.
 * Content is addressed by the **claim** that holds it (not a raw hash), so whether the
 * bytes are inline or a separate blob is hidden and the read stays scoped as its route
 * is scoped. Content also rides **inline** in query results via `output.content`; the
 * content route fetches the bytes for a single claim — including whatever a capped read
 * truncated or dropped, since a claim keeps its `content_hash` either way.
 *
 * No path segment carries a `$`. The reserved names live in grants (`R $universe`) and
 * in a RankeQL body (`select.branch`), and a route names the same scope as a plain
 * segment — which also means these paths survive being typed into a shell.
 *
 * ## Scopes and closures
 *
 * `select.branch` is the mandatory **scope**: a branch name confines the query to
 * that branch, `$archive` to the whole Ranke-Archive, and `$universe` applies no
 * confinement — the privileged by-head-id read, which therefore **requires**
 * `select.head`. Under every other scope `head` is optional and narrows the read to
 * that claim's closure, so it can only narrow, never widen past the grant. A closure
 * is immutable, so pinning `head` gives a paged read a snapshot that cannot shift
 * while the archive advances.
 *
 * ## Encodings and verifiability
 *
 * A query returns a **streaming sequence** of results, one item each, in the query's
 * order. `output.encoding` fixes how each item is serialised and the response media
 * type frames the sequence:
 *
 *   - `json` → `application/json-seq` (RFC 7464) — JSON records, content
 *     base64-encoded when inlined. A **convenience projection**: easy to read and
 *     debug, but **not** independently verifiable.
 *   - `cbor` → `application/cbor-seq` (RFC 8742) — binary.
 *
 * Verifiability is a property of the *shaping*, not of the framing alone:
 * `detail: claims` + `form: original` + `encoding: cbor` + `content: {max: 0}`
 * reproduces the canonical serialization a claim's id is computed over, and is the only
 * combination directly re-hashable and signature-checkable against that id (`R-QCANON`).
 * Every other shaping is a rendering for convenience.
 *
 * **The content axis is part of that combination, not a detail of it.** A query inlines
 * no content unless `output.content` asks (`R-QCONTENT`), so a read shaped for
 * verification states `content: {max: 0}` — a cap of zero meaning *in full*, as a zero
 * bound does everywhere else in a query. Omit it and the claims arrive without their
 * content, which re-hashes to something other than the id. `content_size` is served
 * either way, so a client always sees that content exists and how long it is.
 *
 * A by-id claim GET returns the claim as its signed CBOR (`application/cbor`) whole, so
 * it needs none of this; a content GET streams the blob as raw bytes
 * (`application/octet-stream`). In query results content is instead carried inline
 * (base64 under `json`, byte strings under `cbor`), bounded by `output.content`.
 *
 * ## Credentials and authorization
 *
 * The contract **binds no per-operation rights** and defines **no** read/write/admin
 * ladder — authorization is entirely core-access (system accounts with CRUDA rights
 * over branch globs), decided on the subject the credential resolves to. Every route
 * accepts the same set of credentials; the shape of a route never varies by right.
 *
 * What the contract *does* enumerate is how a credential is **presented**, because
 * the endpoint routes deterministically on the presented scheme to the auth adapter
 * of that type — no parsing a token to guess its kind:
 *
 *   - `Authorization: Bearer <jwt>` → the **JWT** adapter
 *   - `X-API-Key: <key>` → the **API-key** adapter
 *   - `Authorization: Macaroon <token>` → the **macaroon** adapter
 *   - **no** credential header → the **noauth** adapter
 *
 * Which of these adapters is actually mounted is the stack's config; a presentation
 * with no configured adapter is `401`. (Only if two mechanisms were forced onto one
 * scheme would the endpoint fall back to trying each — the schemes above are
 * distinct, so routing stays deterministic.) A `403` body never discloses the
 * subject id.
 *
 * ## Browsers
 *
 * A browser refuses a cross-origin read unless the server admits the origin, so an
 * instance an explorer is meant to reach declares which origins may: the endpoint's
 * `allowedOrigins` (a comma-separated list, `*` for any). Declaring none — the default —
 * leaves the API unreachable from any page but its own origin, which is right for a server
 * nobody browses.
 *
 * Where an origin is admitted, a preflight is answered with the methods and the credential
 * headers above, and `ETag` is **exposed**, without which the conditional reads this
 * contract describes cannot be made from a script. No origin is granted credentialed
 * access: a credential rides in a header here, never in a cookie.
 *
 * ## Closure and the 404 rule
 *
 * Every branch read is bounded by that branch's closure, and every archive read by
 * the current head's closure. A claim or content that exists in the Universe but lies
 * **outside** the closure the route names returns `404` — indistinguishable from one
 * that does not exist. Reads under no closure at all are privileged: they are the
 * `/universe/…` collection, gated by **R** on `$universe`.
 */
export class Api<
  SecurityDataType extends unknown,
> extends HttpClient<SecurityDataType> {
  health = {
    /**
     * @description Reports that the stack is up and the signing/contributor identity it attests merges under. Whether this requires a privileged subject is a core-access decision, not part of this contract.
     *
     * @tags system
     * @name Health
     * @summary Liveness and signer identity
     * @request GET:/health
     * @secure
     */
    health: (params: RequestParams = {}) =>
      this.request<Health, any>({
        path: `/health`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),
  };
  query = {
    /**
     * @description Runs a RankeQL query (§RankeQL) and streams the result set, one item per result, in the serialization chosen by `output.encoding`. Results carry the natural `(created_at, id)` order unless `order` sorts them, with its keys applied in priority order and that natural order breaking any remaining ties; to page, carry the last result's order key into the next request's `where` (pin `select.head` so the closure cannot shift between pages). When `execution.report` names a verbosity, the **final item** in the sequence is a `QueryReport` (see the schema) — typed distinctly from result items and always last — carrying the execution log, timing, and whether a limit truncated the read. `execution.layer` pins which storage layer runs it. The response media type mirrors `output.encoding`: `application/json-seq` or `application/cbor-seq`. A branch named in `select` that does not exist is `404`; claims outside the scope's closure simply do not appear. `$universe` is the privileged unconfined read and requires `select.head`.
     *
     * @tags read
     * @name Query
     * @summary Read the graph with a declarative query
     * @request POST:/query
     * @secure
     */
    query: (data: Query, params: RequestParams = {}) =>
      this.request<File, Error>({
        path: `/query`,
        method: "POST",
        body: data,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),
  };
  contribute = {
    /**
     * @description Contributes one or more signed claims to a branch as a single **atomic** merge — all are absorbed under one new branch-table head, or none are. Content-addressed and therefore **idempotent**: re-contributing yields the same ids with no duplicates. The body is ranke's contribution stream: a **CBOR sequence** (RFC 8742) of records, self-framing so a contribution of any size streams. - `[2, ["<branch>", …]]` — **first**, the branches this contribution touches - `[0, <id>, <claim bytes>, "<branch>"]` — a claim, as the canonical CBOR its contributor signed, and the branch it joins - `[1, <hash>, <content bytes>]` — externalized content, which lives in the Universe unbranched and so names no branch The header comes first so the **C** right is settled on every declared branch before any of the body is read; a claim naming an undeclared branch is refused, so the declaration binds. One contribution may therefore advance **several branches**, and an unauthorized one is answered without its payload being read. Payloads are CBOR byte strings carried through untouched: a claim's id is a signature over exactly those bytes, so the server stores what it was sent. The id is explicit because it cannot be derived — `Sign(H(S(node)))` is made with the contributor's key, and the canonical encoding carries only the node. Content is checked against the hash addressing it. Send every claim the closure needs that the archive may not hold yet — a first-time contributor includes its own `contribution/contributor` claim, since everything it signs references it. The sequencer mints the `contribution/branches` branch-table claim recording the merge (paper 02 §Sequencer). On success the new branch-table head id and the contributed claim ids are returned.
     *
     * @tags write
     * @name Contribute
     * @summary Contribute signed claims (atomic)
     * @request POST:/contribute
     * @secure
     */
    contribute: (data: File, params: RequestParams = {}) =>
      this.request<ContributionResult, Error>({
        path: `/contribute`,
        method: "POST",
        body: data,
        secure: true,
        format: "json",
        ...params,
      }),
  };
  branches = {
    /**
     * @description Returns every branch the branch table holds, each with its name and current head id. Reachable **without knowing any branch name**, so a client discovers what it may address before using the routes that take one; an archive with no branches yet answers with an empty list. Requires the **R** right on the reserved `$branches` target, which is what core-access grants as "enumerates the table". Cacheable **with revalidation** (weak `ETag`, `Cache-Control: no-cache`): every head in the list moves as its branch is contributed to. The listing is answered from **one archive snapshot**, so the heads are consistent with each other.
     *
     * @tags read
     * @name ListBranches
     * @summary List the branch table's branches
     * @request GET:/branches
     * @secure
     */
    listBranches: (params: RequestParams = {}) =>
      this.request<BranchList, Error>({
        path: `/branches`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Returns the branch's current head id — a moving target (it advances on every contribution). Requires the **R** right on that branch. Cacheable **with revalidation** (weak `ETag`, `Cache-Control: no-cache`): a conditional request is cheap when the head has not moved. To inspect the head claim itself, fetch it via `GET /branches/{branch}/claims/{id}`.
     *
     * @tags read
     * @name GetBranchHead
     * @summary Current head id of a branch
     * @request GET:/branches/{branch}/head
     * @secure
     */
    getBranchHead: (branch: string, params: RequestParams = {}) =>
      this.request<BranchHead, Error>({
        path: `/branches/${branch}/head`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Reports a branch beyond its head id: the head's **height** — the generation number of the branch's newest claim, so the depth of what it points at — and **when it last moved**, which is the head claim's `created_at`. Everything here comes from the head claim, so it costs one claim read. A claim count is deliberately absent: counting a branch's claims is a walk of its closure, which is a query (`POST /query`) and not a field. Requires the **R** right on the branch. Cacheable **with revalidation** — every field moves when the branch does.
     *
     * @tags read
     * @name GetBranchInfo
     * @summary What is known about a branch
     * @request GET:/branches/{branch}/info
     * @secure
     */
    getBranchInfo: (branch: string, params: RequestParams = {}) =>
      this.request<BranchInfo, Error>({
        path: `/branches/${branch}/info`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Returns claim `{id}` as its signed CBOR bytes, **only if it lies in branch `{branch}`'s closure**. A claim that is superseded, contradicted, or otherwise outside the closure returns `404`, indistinguishable from one that does not exist. Requires the **R** right on that branch. `{branch}` names an **ordinary branch**. A reserved scope name supplied here names a branch that does not exist and is answered as `404`; each scope has exactly one route (`/archive/…`, `/universe/…`). Immutably **cacheable** by id (strong `ETag`, `Cache-Control: public, immutable`): the id content-addresses the bytes, so they never change.
     *
     * @tags read
     * @name GetBranchClaim
     * @summary Fetch a claim within a branch's closure
     * @request GET:/branches/{branch}/claims/{id}
     * @secure
     */
    getBranchClaim: (branch: string, id: string, params: RequestParams = {}) =>
      this.request<File, Error>({
        path: `/branches/${branch}/claims/${id}`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description Streams the content of claim `{id}` — **only if it lies in branch `{branch}`'s closure** (same closure guarantee as the claim itself; out-of-closure or unknown → `404`). Requires the **R** right on that branch. Content is addressed by the claim that holds it, not by a raw hash: the server resolves whether the bytes live inline in the claim or in a separate blob, so the client can't tell and doesn't need to — and the read stays scoped as the route is scoped. Immutably **cacheable** by the claim id.
     *
     * @tags read
     * @name GetBranchClaimContent
     * @summary Fetch the content of a claim within a branch's closure
     * @request GET:/branches/{branch}/claims/{id}/content
     * @secure
     */
    getBranchClaimContent: (
      branch: string,
      id: string,
      params: RequestParams = {},
    ) =>
      this.request<Blob, Error>({
        path: `/branches/${branch}/claims/${id}/content`,
        method: "GET",
        secure: true,
        ...params,
      }),
  };
  archive = {
    /**
     * @description Reports the Ranke-Archive as a whole: the **branch-table head** — the id every archive-scoped read is rooted at — with its height and when it last moved, and how many branches the table holds. This is the only route that reports the branch-table head, which is what a client needs to name the `$archive` scope in a query or a grant. Reads the `$archive` scope and requires the **R** right on `$archive`. Cacheable **with revalidation**: the head advances on every contribution.
     *
     * @tags read
     * @name GetArchiveInfo
     * @summary What is known about the archive
     * @request GET:/archive/info
     * @secure
     */
    getArchiveInfo: (params: RequestParams = {}) =>
      this.request<ArchiveInfo, Error>({
        path: `/archive/info`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Returns claim `{id}` as its signed CBOR bytes if it lies in the closure of the **whole Ranke-Archive** — the current branch-table head — whichever branch holds it. A client reaches a claim without naming the branch it is on; a claim outside that closure returns `404`. This collection reads the `$archive` scope: the same scope a RankeQL body names as `select.branch: "$archive"`, and the same target a grant is written against. It requires the **R** right on `$archive`. Immutably cacheable by id.
     *
     * @tags read
     * @name GetArchiveClaim
     * @summary Fetch a claim within the archive head's closure
     * @request GET:/archive/claims/{id}
     * @secure
     */
    getArchiveClaim: (id: string, params: RequestParams = {}) =>
      this.request<File, Error>({
        path: `/archive/claims/${id}`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description Streams the content of claim `{id}` if it lies in the archive head's closure, across every branch (same closure guarantee as the claim itself; outside it or unknown → `404`). Reads the `$archive` scope and requires **R** on `$archive`. Content is addressed by the claim; whether the bytes are inline or a separate blob is hidden. Immutably cacheable by the claim id.
     *
     * @tags read
     * @name GetArchiveClaimContent
     * @summary Fetch the content of a claim within the archive head's closure
     * @request GET:/archive/claims/{id}/content
     * @secure
     */
    getArchiveClaimContent: (id: string, params: RequestParams = {}) =>
      this.request<Blob, Error>({
        path: `/archive/claims/${id}/content`,
        method: "GET",
        secure: true,
        ...params,
      }),
  };
  universe = {
    /**
     * @description Returns claim `{id}` as its signed CBOR bytes from the Universe under **no closure at all** — no branch table, and no confinement to the current head. This is what makes it privileged, and what it exists for: reaching an archive from a Universe and a head id alone, as when restoring from a head kept outside the server. A claim the Universe holds is returned even where `GET /archive/claims/{id}` reports it not-found. This collection reads the `$universe` scope: the same scope a RankeQL body names as `select.branch: "$universe"`, and the same target a grant is written against. It requires the **R** right on `$universe`, to which only **R** applies. An ordinary glob confers it by no accident — a `$`-prefixed target needs an exact grant, so `R *` reaches neither reserved scope. Immutably cacheable by id.
     *
     * @tags read
     * @name GetClaim
     * @summary Fetch a claim by id from the Universe (privileged)
     * @request GET:/universe/claims/{id}
     * @secure
     */
    getClaim: (id: string, params: RequestParams = {}) =>
      this.request<File, Error>({
        path: `/universe/claims/${id}`,
        method: "GET",
        secure: true,
        ...params,
      }),

    /**
     * @description Streams the content of claim `{id}` from the Universe under no closure — the privileged read, conferred only through **R** on `$universe`. Content is addressed by the claim; whether the bytes are inline or a separate blob is hidden. Immutably cacheable by the claim id.
     *
     * @tags read
     * @name GetClaimContent
     * @summary Fetch the content of a claim by id from the Universe (privileged)
     * @request GET:/universe/claims/{id}/content
     * @secure
     */
    getClaimContent: (id: string, params: RequestParams = {}) =>
      this.request<Blob, Error>({
        path: `/universe/claims/${id}/content`,
        method: "GET",
        secure: true,
        ...params,
      }),
  };
  dev = {
    /**
     * @description Available **only** when the stack was launched with `--dev` against a `dev` Sequencer: moves the clock the Sequencer mints `created_at` and branch-table timestamps from forward to (at least) the given instant, so a client that knows its own story's schedule — a fixture generator, say — can make the archive's *recorded* history track its *narrated* one, one contribution at a time, rather than every merge landing at the real wall-clock moment the client happened to run. The clock never moves backward: a request older than its current position is accepted and answered with the position unchanged. Absent `--dev`, or against a `concurrent` (production) Sequencer, the route is `501` — the witnessed merge time stays real, which is the whole point of `R-C2DATE`.
     *
     * @tags dev
     * @name AdvanceDevClock
     * @summary Advance the dev sequencer's clock
     * @request POST:/dev/clock
     * @secure
     */
    advanceDevClock: (data: DevClockAdvance, params: RequestParams = {}) =>
      this.request<DevClock, Error>({
        path: `/dev/clock`,
        method: "POST",
        body: data,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),
  };
  system = {
    /**
     * @description Reports the caller back to itself: the account its credential resolved to, that account's grants, and any caveats attenuating them. It answers about **this server's** access policy, never about the graph, which is why it sits under `/system`. Needs no grant — a caller learns only what it already proved by authenticating, and nothing about any other account. A bad credential is still `401`, so this doubles as the side-effect-free way to check one. `grants` and `caveats` are reported separately rather than intersected: a request is allowed when the account's grants permit it **and** every caveat still does, so seeing both is what explains a refusal. A token narrowed by attenuation has no other way to show what survived.
     *
     * @tags system
     * @name Whoami
     * @summary What this credential may do
     * @request GET:/system/whoami
     * @secure
     */
    whoami: (params: RequestParams = {}) =>
      this.request<Subject, Error>({
        path: `/system/whoami`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Lists the stack's storage layers (read-through tiers) by **name and type only** — never connection details or secrets. Naming layers is what lets a verification run target one directly (a read-through view can mask the loss of an object on a deeper layer).
     *
     * @tags system
     * @name ListStorageLayers
     * @summary List storage layers
     * @request GET:/system/layers
     * @secure
     */
    listStorageLayers: (params: RequestParams = {}) =>
      this.request<StorageLayerList, Error>({
        path: `/system/layers`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Verification runs across the whole stack, newest first. Each report is a **point-in-time record**: a layer repaired externally shows clean in a later run, so reports accumulate rather than overwrite — until explicitly removed with `DELETE /system/verifications/{reportId}`. The API lives under `/system/` because a verification run is a ranke-db extension beyond the paper's surface rather than archive content: it is a stack-wide operational resource, rooted at a closure named in the request body and not in the path. (Branch reads no longer occupy the root, so nothing here is avoiding a collision.)
     *
     * @tags system
     * @name ListVerifications
     * @summary List verification runs
     * @request GET:/system/verifications
     * @secure
     */
    listVerifications: (params: RequestParams = {}) =>
      this.request<VerificationReportList, Error>({
        path: `/system/verifications`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Starts a run from a `VerificationConfig`: walk the closure rooted at `closure` (a branch name — resolved to its current head and **pinned** for the life of the run — or a head id directly), reading the named `layer` directly, re-checking each claim to the configured depth. It returns *findings*, not contents, so it never leaks across closures. A run may take a **very long time** (hours to days over a large closure at full-content depth), so it is always **asynchronous**: the call returns `202` immediately with the running report and a `Location` header pointing at the report resource. Poll it with `GET /system/verifications/{reportId}`, pacing by the `Retry-After` hint; stop it with `DELETE`. Starting the same run twice yields two independent point-in-time reports (runs are not deduplicated). Verification is resource-heavy, so the stack caps the number of runs that may execute **concurrently** (configured; default 1). When that cap is already reached the call returns `429` — the server never stops a run to make room. To proceed, either wait (per `Retry-After`) or `GET /system/verifications` to see what is running and free a slot deliberately: `cancel` a run to stop it while keeping its report, or `DELETE` it to remove it entirely.
     *
     * @tags system
     * @name StartVerification
     * @summary Start a verification run
     * @request POST:/system/verifications
     * @secure
     */
    startVerification: (data: VerificationConfig, params: RequestParams = {}) =>
      this.request<VerificationReport, Error>({
        path: `/system/verifications`,
        method: "POST",
        body: data,
        secure: true,
        type: ContentType.Json,
        format: "json",
        ...params,
      }),

    /**
     * @description The report for a run. Poll until `status` leaves `running`; while it is still running the response carries a `Retry-After` hint and the progress counters (`claimsChecked`, `bytesRead`) advance between polls.
     *
     * @tags system
     * @name GetVerification
     * @summary Show a verification run
     * @request GET:/system/verifications/{reportId}
     * @secure
     */
    getVerification: (reportId: string, params: RequestParams = {}) =>
      this.request<VerificationReport, Error>({
        path: `/system/verifications/${reportId}`,
        method: "GET",
        secure: true,
        format: "json",
        ...params,
      }),

    /**
     * @description Really deletes the run and its report. If the run is still `running` it is stopped first, then the record is removed — so this both aborts a run and cleans up finished history in one step. Unlike the cancel action, nothing survives: a subsequent `GET` is `404`. Either verb frees a concurrency slot; cancel keeps the report, delete removes it.
     *
     * @tags system
     * @name DeleteVerification
     * @summary Delete a verification run
     * @request DELETE:/system/verifications/{reportId}
     * @secure
     */
    deleteVerification: (reportId: string, params: RequestParams = {}) =>
      this.request<void, Error>({
        path: `/system/verifications/${reportId}`,
        method: "DELETE",
        secure: true,
        ...params,
      }),

    /**
     * @description Stops a `running` run but **keeps** its report: the record stays in history with `status` `stopped` and whatever partial findings it had gathered, and the concurrency slot is freed. Use this to abort a run you want to keep a record of; use `DELETE` to remove it entirely. Idempotent — cancelling a run that has already finished or stopped returns its current report unchanged.
     *
     * @tags system
     * @name CancelVerification
     * @summary Cancel a running verification run
     * @request POST:/system/verifications/{reportId}/cancel
     * @secure
     */
    cancelVerification: (reportId: string, params: RequestParams = {}) =>
      this.request<VerificationReport, Error>({
        path: `/system/verifications/${reportId}/cancel`,
        method: "POST",
        secure: true,
        format: "json",
        ...params,
      }),
  };
}
