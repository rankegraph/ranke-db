/**
 * package: core / content
 * type:    data
 * job:     hold the claim content this session has read
 * limits:  storage only; reading it is the port's, rendering it the UI's (-> core/data, ui)
 *
 * A claim is content-addressed, so its bytes can never change: the id *is* the hash of what
 * they are. That makes this the one cache in the client needing no invalidation — a claim
 * read once is read for the session.
 *
 * Module scope, like the graph: bytes are data, and data does not go into React state. The
 * cache is bounded by what the reader actually opened, since content is fetched on selection
 * rather than in bulk.
 */

const cache = new Map<string, Uint8Array>();

/** contentOf returns bytes already read, or null. */
export function contentOf(id: string): Uint8Array | null {
  return cache.get(id) ?? null;
}

/** rememberContent keeps a claim's bytes for the session. */
export function rememberContent(id: string, bytes: Uint8Array): void {
  cache.set(id, bytes);
}

/** isTextual reports whether an encoding says the bytes are meant to be read as characters. */
export function isTextual(encoding: string | undefined): boolean {
  if (!encoding) return false;
  return (
    encoding.startsWith('text/') ||
    encoding === 'application/json' ||
    encoding === 'application/xml' ||
    encoding.endsWith('+json') ||
    encoding.endsWith('+xml')
  );
}

/** asText decodes bytes as UTF-8, replacing anything that is not. */
export function asText(bytes: Uint8Array): string {
  return new TextDecoder('utf-8', { fatal: false }).decode(bytes);
}

/** forgetContent empties the cache — a session reset, not something a read does. */
export function forgetContent(): void {
  cache.clear();
}

/** cachedContentCount is how many claims' bytes are held, for a test to pin the cache. */
export function cachedContentCount(): number {
  return cache.size;
}
