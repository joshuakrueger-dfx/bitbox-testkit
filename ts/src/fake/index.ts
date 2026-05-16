/**
 * In-memory fake of the bitbox-api PairedBitBox surface.
 *
 * Most tests want one of three flavours:
 *   1. Plug a hand-crafted handler map into FakePairedBitBox directly.
 *   2. Use a pre-built Scenario from /ts/src/scenarios to drive a specific
 *      firmware reaction (e.g. "umlaut → ErrInvalidInput101").
 *   3. Use a Quirk's `.scenario()` from /ts/src/quirks for regression suites.
 *
 * The fake records every call (`.calls`) so tests can assert what the
 * consumer attempted to send.
 */

import { ClosedError, UnexpectedQueryError } from '../errors.js';
import type { FakeSetup, Handler } from './types.js';

export type { FakeSetup, Handler } from './types.js';

export interface RecordedCall {
  readonly method: string;
  readonly args: readonly unknown[];
}

/**
 * FakePairedBitBox replaces a real `bitbox-api` PairedBitBox in tests.
 * It exposes a Proxy-based dispatch: any method invocation looks up a
 * handler in the configured map. Unknown methods throw UnexpectedQueryError.
 */
export class FakePairedBitBox {
  private handlers: Record<string, Handler> = {};
  private _calls: RecordedCall[] = [];
  private _closed = false;
  private _onClose: (() => void) | null = null;

  /** Construct a fake from a setup descriptor (typically from a Scenario). */
  static from(setup: FakeSetup): FakePairedBitBox {
    const f = new FakePairedBitBox();
    f.handlers = { ...setup.methods };
    return f;
  }

  /** Override (or add) a handler for one method. */
  on<TArgs extends readonly unknown[] = unknown[], TResult = unknown>(
    method: string,
    handler: Handler<TArgs, TResult>,
  ): this {
    this.handlers[method] = handler as unknown as Handler;
    return this;
  }

  /** Register a one-shot callback when close() is first called. */
  onClose(fn: () => void): this {
    this._onClose = fn;
    return this;
  }

  /** Mark the fake closed. Subsequent dispatch throws ClosedError. */
  close(): void {
    if (this._closed) return;
    this._closed = true;
    this._onClose?.();
  }

  get closed(): boolean {
    return this._closed;
  }

  /** Defensive snapshot of every dispatched call. */
  get calls(): readonly RecordedCall[] {
    return this._calls.map((c) => ({ method: c.method, args: [...c.args] }));
  }

  /** Clear the recorded call log without releasing the fake. */
  clearCalls(): this {
    this._calls = [];
    return this;
  }

  /**
   * Returns a Proxy that routes any property access into the handler
   * map. Use this as a drop-in replacement for `PairedBitBox`.
   *
   * The generic parameter is the wallet-API shape you expect (e.g. an
   * import from `bitbox-api`). It is a pure type cast; the proxy does
   * no runtime check against the type's structure.
   */
  asPairedBitBox<T = unknown>(): T {
    const self = this;
    return new Proxy(
      {},
      {
        get(_target, prop) {
          // Symbol-keyed property lookups (Symbol.toPrimitive,
          // Symbol.asyncIterator, then/catch probes from awaiters)
          // must NOT be treated as dispatched methods — returning a
          // function for `then` would make every proxy access look
          // thenable and infect await chains.
          if (typeof prop === 'symbol') return undefined;
          if (prop === 'then' || prop === 'catch' || prop === 'finally') return undefined;

          // Synthetic helpers exposed directly on the proxy for
          // introspection and cleanup paths.
          if (prop === '__fake__') return self;
          if (prop === 'close' || prop === 'free') return () => self.close();

          const method = prop;
          return (...args: unknown[]) => {
            if (self._closed) {
              return Promise.reject(new ClosedError());
            }
            self._calls.push({ method, args });
            const handler = self.handlers[method];
            if (!handler) {
              return Promise.reject(new UnexpectedQueryError(method));
            }
            try {
              const result = handler(...args);
              return Promise.resolve(result);
            } catch (err) {
              return Promise.reject(err);
            }
          };
        },
      },
    ) as T;
  }
}

/**
 * Convenience: create a FakePairedBitBox from a FakeSetup and return its
 * proxy form, ready to drop into a `jest.mock` factory.
 */
export function buildPairedBitBox<T = unknown>(setup: FakeSetup): T {
  return FakePairedBitBox.from(setup).asPairedBitBox<T>();
}
