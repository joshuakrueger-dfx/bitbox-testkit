import { FakePairedBitBox, buildPairedBitBox } from '../src/fake/index.js';
import { UnexpectedQueryError, ClosedError, ErrInvalidInput101 } from '../src/errors.js';

describe('FakePairedBitBox', () => {
  it('dispatches to configured handler', async () => {
    const fake = new FakePairedBitBox().on('ethAddress', async () => '0xabc');
    const proxy = fake.asPairedBitBox<{ ethAddress: () => Promise<string> }>();
    await expect(proxy.ethAddress()).resolves.toBe('0xabc');
  });

  it('rejects unknown methods with UnexpectedQueryError', async () => {
    const proxy = new FakePairedBitBox().asPairedBitBox<{ deviceInfo: () => Promise<unknown> }>();
    await expect(proxy.deviceInfo()).rejects.toBeInstanceOf(UnexpectedQueryError);
  });

  it('records every call', async () => {
    const fake = new FakePairedBitBox().on('ethAddress', async (...args) => `addr-${args.join(',')}`);
    const proxy = fake.asPairedBitBox<{ ethAddress: (n: bigint, k: number[]) => Promise<string> }>();
    await proxy.ethAddress(1n, [44, 60, 0]);
    await proxy.ethAddress(137n, [44, 60, 0]);
    expect(fake.calls).toHaveLength(2);
    expect(fake.calls[0]!.method).toBe('ethAddress');
    expect(fake.calls[1]!.args[0]).toBe(137n);
  });

  it('blocks dispatch after close()', async () => {
    const fake = new FakePairedBitBox().on('deviceInfo', async () => ({ name: 'BB' }));
    const proxy = fake.asPairedBitBox<{ deviceInfo: () => Promise<unknown> }>();
    fake.close();
    expect(fake.closed).toBe(true);
    await expect(proxy.deviceInfo()).rejects.toBeInstanceOf(ClosedError);
  });

  it('onClose fires exactly once', () => {
    let count = 0;
    const fake = new FakePairedBitBox().onClose(() => (count += 1));
    fake.close();
    fake.close();
    expect(count).toBe(1);
  });

  it('converts thrown errors to rejections', async () => {
    const fake = new FakePairedBitBox().on('ethSign', () => {
      throw ErrInvalidInput101;
    });
    const proxy = fake.asPairedBitBox<{ ethSign: () => Promise<unknown> }>();
    await expect(proxy.ethSign()).rejects.toBe(ErrInvalidInput101);
  });

  it('Calls snapshot is defensive (mutation does not affect future reads)', async () => {
    const fake = new FakePairedBitBox().on('a', async () => null);
    const proxy = fake.asPairedBitBox<{ a: () => Promise<null> }>();
    await proxy.a();
    const snap = fake.calls as unknown as Array<{ method: string; args: unknown[] }>;
    snap[0]!.method = 'MUTATED';
    expect(fake.calls[0]!.method).toBe('a');
  });

  it('buildPairedBitBox returns a usable proxy', async () => {
    const proxy = buildPairedBitBox<{ deviceInfo: () => Promise<{ name: string }> }>({
      methods: { deviceInfo: async () => ({ name: 'BB' }) },
    });
    await expect(proxy.deviceInfo()).resolves.toEqual({ name: 'BB' });
  });

  it('proxy does NOT pretend to be thenable (avoids awaiter false-positives)', () => {
    const proxy = new FakePairedBitBox().asPairedBitBox<Record<string, unknown>>();
    // `await proxy` would call proxy.then(...) and infect chains otherwise.
    expect(proxy.then).toBeUndefined();
    expect(proxy.catch).toBeUndefined();
    expect(proxy.finally).toBeUndefined();
  });

  it('proxy returns undefined for symbol-keyed lookups', () => {
    const proxy = new FakePairedBitBox().asPairedBitBox<Record<symbol, unknown>>();
    expect((proxy as unknown as { [Symbol.iterator]?: unknown })[Symbol.iterator]).toBeUndefined();
  });

  it('clearCalls drops the recorded log without affecting handlers', async () => {
    const fake = new FakePairedBitBox().on('a', async () => 'x');
    const proxy = fake.asPairedBitBox<{ a: () => Promise<string> }>();
    await proxy.a();
    expect(fake.calls).toHaveLength(1);
    fake.clearCalls();
    expect(fake.calls).toHaveLength(0);
    // handler still works
    await expect(proxy.a()).resolves.toBe('x');
  });
});
