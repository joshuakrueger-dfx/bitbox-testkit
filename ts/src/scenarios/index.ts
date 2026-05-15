/**
 * Scenario factories: produce FakeSetup descriptors for known firmware
 * behaviours. Used directly in tests or attached to Quirks for regression
 * suites.
 *
 * Mirrors /go/bitbox/scenarios. The TS scenarios target the bitbox-api
 * PairedBitBox surface rather than the raw firmware.Communication.
 */

import {
  ErrInvalidInput101,
  ErrUserAbort,
  AwaitingUserConfirmError,
} from '../errors.js';
import type { FakeSetup, Handler } from '../fake/types.js';

/**
 * For methods we don't model individually, every call resolves to a wire-level
 * firmware "invalid input" error. Useful as a default scenario for quirks
 * that only differ in the input that triggers them.
 */
export function scenarioErrInvalidInput(): FakeSetup {
  const reject: Handler = () => Promise.reject(ErrInvalidInput101);
  return {
    description: 'every PairedBitBox call rejects with ErrInvalidInput101',
    methods: methodCatchAll(reject),
  };
}

/**
 * The umlaut/EIP-712 quirk. Any ethSignMessage / ethSignTypedMessage call
 * carrying a non-ASCII payload rejects with ErrInvalidInput101. ASCII
 * payloads succeed with a dummy signature.
 *
 * The real bitbox-api types use `Uint8Array` for msg; we treat byte >= 0x80
 * as non-ASCII.
 */
export function scenarioRegressionUmlautEIP712(): FakeSetup {
  const checkAscii: Handler = (..._args) => {
    const msg = findFirstBytes(_args);
    if (msg && containsNonAscii(msg)) {
      return Promise.reject(ErrInvalidInput101);
    }
    // dummy 65-byte signature for ethSignMessage; consumers usually don't inspect it
    return Promise.resolve({ r: new Uint8Array(32), s: new Uint8Array(32), v: 0 });
  };
  return {
    description: 'non-ASCII payload rejected with ErrInvalidInput101',
    methods: {
      ethSignMessage: checkAscii,
      ethSignTypedMessage: checkAscii,
    },
  };
}

/**
 * Simulates user aborting on-device mid-flow. After `before` successful
 * calls, every subsequent call rejects with ErrUserAbort.
 */
export function scenarioDeviceDisconnect(before = 2): FakeSetup {
  let seen = 0;
  const handler: Handler = () => {
    if (seen++ >= before) {
      return Promise.reject(ErrUserAbort);
    }
    return Promise.resolve(undefined);
  };
  return {
    description: `closes after ${before} successful calls`,
    methods: methodCatchAll(handler),
  };
}

/**
 * Panic-mid-query: the n-th call throws synchronously. Use to verify the
 * consumer's WebView/WASM bridge wraps thrown errors into rejected promises
 * rather than stalling the bridge silently.
 */
export function scenarioPanicMidQuery(n = 1, value: unknown = 'simulated panic'): FakeSetup {
  let count = 0;
  const handler: Handler = () => {
    count++;
    if (count === n) {
      throw value;
    }
    return Promise.resolve(undefined);
  };
  return {
    description: `throws synchronously on call ${n}`,
    methods: methodCatchAll(handler),
  };
}

/**
 * Slow response: every call resolves only after `delayMs`. Use this to
 * prove the consumer's transport timeouts are context-driven (long
 * user-confirm flows must succeed).
 */
export function scenarioSlowResponse(delayMs = 15_000, payload: unknown = undefined): FakeSetup {
  const handler: Handler = () => new Promise((resolve) => setTimeout(() => resolve(payload), delayMs));
  return {
    description: `every call delayed by ${delayMs}ms`,
    methods: methodCatchAll(handler),
  };
}

/**
 * Quirk E10: firmware on older versions doesn't recognise newer chain IDs.
 * Calls whose first argument is a chain ID in `unknownChainIds` reject with
 * ErrInvalidInput101; calls on other chains succeed. Use the
 * `bigintChainIds` flag if your bitbox-api wrapper uses bigint chain IDs.
 */
export function scenarioUnknownNetwork(unknownChainIds: readonly (number | bigint)[] = [999, 146]): FakeSetup {
  const numSet = new Set<number>();
  const bigSet = new Set<bigint>();
  for (const id of unknownChainIds) {
    if (typeof id === 'bigint') bigSet.add(id);
    else numSet.add(id);
  }
  const handler: Handler = async (...args) => {
    const head = args[0];
    if (typeof head === 'number' && numSet.has(head)) {
      return Promise.reject(ErrInvalidInput101);
    }
    if (typeof head === 'bigint' && bigSet.has(head)) {
      return Promise.reject(ErrInvalidInput101);
    }
    return undefined;
  };
  return {
    description: `rejects calls whose first arg is one of ${[...numSet, ...bigSet].join(',')}`,
    methods: methodCatchAll(handler),
  };
}

/**
 * Pairing race scenario: the first n calls succeed (channel hash available),
 * subsequent calls reject with AwaitingUserConfirmError until
 * `signalConfirm()` is invoked.
 */
export function scenarioChannelHashEarly(hashRepeats = 2): FakeSetup & { signalConfirm: () => void } {
  let hashCount = 0;
  let confirmed = false;
  const handler: Handler = () => {
    if (hashCount < hashRepeats) {
      hashCount++;
      return Promise.resolve({ channelHash: new Uint8Array([0xde, 0xad, 0xbe, 0xef]) });
    }
    if (!confirmed) {
      return Promise.reject(new AwaitingUserConfirmError());
    }
    return Promise.resolve(undefined);
  };
  return {
    description: 'channel hash available before user confirm; reject until signalConfirm()',
    methods: methodCatchAll(handler),
    signalConfirm: () => {
      confirmed = true;
    },
  };
}

// helpers

function methodCatchAll(handler: Handler): Record<string, Handler> {
  // Common bitbox-api PairedBitBox methods we typically need to intercept.
  // Methods not listed fall through to UnexpectedQueryError unless caller
  // adds them.
  const methods: Record<string, Handler> = {};
  for (const m of [
    'btcAddress',
    'btcSignMessage',
    'btcSignPSBT',
    'ethAddress',
    'ethSign',
    'ethSignEIP1559',
    'ethSignMessage',
    'ethSignTypedMessage',
    'cardanoAddress',
    'cardanoXpubs',
    'cardanoSignTransaction',
    'deviceInfo',
    'rootFingerprint',
    'showMnemonic',
    'restoreFromMnemonic',
  ]) {
    methods[m] = handler;
  }
  return methods;
}

function findFirstBytes(args: readonly unknown[]): Uint8Array | undefined {
  for (const a of args) {
    if (a instanceof Uint8Array) return a;
  }
  return undefined;
}

function containsNonAscii(b: Uint8Array): boolean {
  for (let i = 0; i < b.length; i++) {
    if (b[i]! > 0x7f) return true;
  }
  return false;
}
