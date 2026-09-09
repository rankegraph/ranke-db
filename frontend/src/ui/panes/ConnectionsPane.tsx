/**
 * package: ui / panes
 * type:    view
 * job:     show and edit where claims come from
 * limits:  presentation; it reads state and dispatches actions (-> core/session)
 *
 * A source is a mock archive (generator parameters standing in for server details) or a
 * ranke-db instance, both read through the same port. Several of either can be
 * configured: the explorer is a client, not an installation.
 */

import { useState } from 'react';
import {
  AUTH_LABELS,
  AUTH_SECRET_LABELS,
  DEFAULT_MOCK,
  useConnections,
} from '../../core/connections.ts';
import type { AuthKind, Connection } from '../../core/connections.ts';
import { sourceFor } from '../../core/data/source.ts';
import { Button, Empty, Field, Select, TextInput, Toggle } from '../components/Field.tsx';

const AUTH_OPTIONS: { value: AuthKind; label: string }[] = (
  ['none', 'apikey', 'jwt', 'macaroon'] as AuthKind[]
).map((kind) => ({ value: kind, label: AUTH_LABELS[kind] }));

function ConnectionRow({ connection }: { connection: Connection }) {
  const { activeId, activate, updateConnection, removeConnection, setSecret, secretOf, probes, setProbe } =
    useConnections();
  const [secret, setSecretLocal] = useState(() => secretOf(connection.id));
  const [open, setOpen] = useState(false);
  const result = probes[connection.id];
  const active = activeId === connection.id;
  const secretLabel = AUTH_SECRET_LABELS[connection.authKind];

  const test = async () => {
    setProbe(connection.id, { state: 'probing' });
    setProbe(connection.id, await sourceFor(connection, secret).health());
  };

  return (
    <div className={`connection${active ? ' is-active' : ''}`}>
      <div className="connection-head">
        <button type="button" className="connection-name" onClick={() => setOpen(!open)}>
          <span className={`dot state-${result?.state ?? 'unknown'}`} aria-hidden="true" />
          {connection.name || connection.baseUrl || 'unnamed'}
        </button>
        <span className="connection-auth">
          {connection.kind === 'mock'
            ? `seed ${connection.mock.seed} · ~${connection.mock.claimsPerContribution}/contribution`
            : AUTH_LABELS[connection.authKind]}
        </span>
        {active ? (
          <span className="badge">active</span>
        ) : (
          <Button onClick={() => activate(connection.id)}>use</Button>
        )}
      </div>

      {result?.state === 'ok' ? (
        <p className="connection-status ok">
          healthy · {result.latencyMs?.toFixed(0)} ms
          {result.version ? ` · ranke-db ${result.version}` : null}
        </p>
      ) : null}
      {result?.state === 'failed' ? <p className="connection-status bad">{result.detail}</p> : null}
      {result?.state === 'probing' ? <p className="connection-status">probing…</p> : null}

      {open && connection.kind === 'mock' ? (
        <div className="connection-body">
          <Field label="name">
            <TextInput
              value={connection.name}
              onChange={(name) => updateConnection(connection.id, { name })}
              placeholder="mock archive"
            />
          </Field>
          <Field label="claims" hint="the archive's ceiling, ~0.12 ms each to build">
            <Select
              value={connection.mock.claims}
              options={[1000, 10000, 100000, 300000].map((n) => ({
                value: n,
                label: n.toLocaleString('en-US'),
              }))}
              onChange={(claims) =>
                updateConnection(connection.id, { mock: { ...connection.mock, claims } })
              }
            />
          </Field>
          <Field label="claims per contribution" hint="sets the height">
            <Select
              value={connection.mock.claimsPerContribution}
              options={[3, 10, 30, 100, 1000].map((n) => ({ value: n, label: String(n) }))}
              onChange={(claimsPerContribution) =>
                updateConnection(connection.id, {
                  mock: { ...connection.mock, claimsPerContribution },
                })
              }
            />
          </Field>
          <Field label="seed" hint="same seed, same archive">
            <TextInput
              value={String(connection.mock.seed)}
              onChange={(raw) => {
                const seed = Number.parseInt(raw, 10);
                if (Number.isFinite(seed)) {
                  updateConnection(connection.id, { mock: { ...connection.mock, seed } });
                }
              }}
            />
          </Field>
          <p className="note">
            Seed and granularity define <em>which archive this is</em>; how much of it you
            read is a query, set in the Query tab. A generated archive goes through the same
            port as a real instance, so neither path is a special case — and its claims are
            built by the ADT library, so their ids are real content addresses. That is what
            the ceiling costs: raising it makes the first read wait for the archive.
          </p>
          <div className="row">
            <Button variant="danger" onClick={() => removeConnection(connection.id)}>
              remove
            </Button>
          </div>
        </div>
      ) : null}

      {open && connection.kind === 'rest' ? (
        <div className="connection-body">
          <Field label="name">
            <TextInput
              value={connection.name}
              onChange={(name) => updateConnection(connection.id, { name })}
              placeholder="local"
            />
          </Field>
          <Field label="base URL">
            <TextInput
              type="url"
              value={connection.baseUrl}
              onChange={(baseUrl) => updateConnection(connection.id, { baseUrl })}
              placeholder="http://localhost:8080"
            />
          </Field>
          <Field label="auth">
            <Select
              value={connection.authKind}
              options={AUTH_OPTIONS}
              onChange={(authKind) => updateConnection(connection.id, { authKind })}
            />
          </Field>
          {secretLabel ? (
            <>
              <Field label={secretLabel}>
                <TextInput
                  type="password"
                  value={secret}
                  onChange={(value) => {
                    setSecretLocal(value);
                    setSecret(connection.id, value);
                  }}
                  placeholder="held in memory only"
                />
              </Field>
              <Toggle
                label="remember on this device"
                checked={connection.remember}
                onChange={(remember) => updateConnection(connection.id, { remember })}
              />
              <p className="note">
                Secrets stay in memory for the session unless remembered. Remembering writes
                them to <code>localStorage</code>, where any script on the page can read them.
              </p>
            </>
          ) : null}
          <div className="row">
            <Button variant="primary" onClick={() => void test()}>
              test
            </Button>
            <Button variant="danger" onClick={() => removeConnection(connection.id)}>
              remove
            </Button>
          </div>
        </div>
      ) : null}
    </div>
  );
}

export function ConnectionsPane() {
  const { connections, addConnection } = useConnections();

  return (
    <div className="pane">
      <p className="lede">
        The explorer is a pure client. It runs from a static bundle, keeps everything in the
        browser, works with no server at all, and can hold several instances at once.
      </p>

      {connections.map((connection) => (
        <ConnectionRow key={connection.id} connection={connection} />
      ))}

      {connections.length === 0 ? <Empty>No instances configured yet.</Empty> : null}

      <div className="row">
        <Button
          onClick={() =>
            addConnection({
              name: `mock ${connections.length + 1}`,
              kind: 'mock',
              baseUrl: '',
              authKind: 'none',
              remember: false,
              mock: { ...DEFAULT_MOCK },
            })
          }
        >
          add mock archive
        </Button>
        <Button
          variant="primary"
          onClick={() =>
            addConnection({
              name: `instance ${connections.length + 1}`,
              kind: 'rest',
              baseUrl: 'http://localhost:8080',
              authKind: 'none',
              remember: false,
              mock: { ...DEFAULT_MOCK },
            })
          }
        >
          add server
        </Button>
      </div>

      <p className="note">
        A server is read through the client generated from <code>openapi.yaml</code>, so every
        route and response type is the contract's. Content bytes come from a server only: a
        generated archive declares its content addresses and sizes, and has no bytes behind
        them.
      </p>
    </div>
  );
}
