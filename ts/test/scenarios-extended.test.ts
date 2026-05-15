import { buildPairedBitBox } from '../src/fake/index.js';
import { scenarioUnknownNetwork } from '../src/scenarios/index.js';
import { ErrInvalidInput101 } from '../src/errors.js';

describe('scenarioUnknownNetwork', () => {
  it('rejects calls with a number chain ID in the unknown set', async () => {
    const proxy = buildPairedBitBox<{ ethAddress: (chainId: number) => Promise<unknown> }>(
      scenarioUnknownNetwork([999]),
    );
    await expect(proxy.ethAddress(999)).rejects.toBe(ErrInvalidInput101);
  });

  it('accepts calls with a known chain ID', async () => {
    const proxy = buildPairedBitBox<{ ethAddress: (chainId: number) => Promise<unknown> }>(
      scenarioUnknownNetwork([999]),
    );
    await expect(proxy.ethAddress(1)).resolves.toBeUndefined();
  });

  it('handles bigint chain IDs', async () => {
    const proxy = buildPairedBitBox<{ ethAddress: (chainId: bigint) => Promise<unknown> }>(
      scenarioUnknownNetwork([999n]),
    );
    await expect(proxy.ethAddress(999n)).rejects.toBe(ErrInvalidInput101);
    await expect(proxy.ethAddress(1n)).resolves.toBeUndefined();
  });
});
