import { readFileSync } from 'fs';
import { fileURLToPath } from 'url';
import { dirname, resolve } from 'path';
import { Registry, subset, firmwareApplies } from '../src/quirks/index.js';

const __dirname = dirname(fileURLToPath(import.meta.url));
const quirksJsonPath = resolve(__dirname, '../src/quirks/quirks.json');
const rawQuirks: { quirks: unknown[] } = JSON.parse(readFileSync(quirksJsonPath, 'utf8'));

describe('quirks registry', () => {
  // Self-consistent count: the Registry MUST expose exactly the number
  // of quirks documented in quirks.json. Hardcoded numbers go stale
  // every release; reading the source-of-truth keeps the assertion
  // load-bearing without needing a manual bump.
  it('loads every quirk from quirks.json into the Registry', () => {
    expect(Registry.length).toBe(rawQuirks.quirks.length);
    expect(Registry.length).toBeGreaterThanOrEqual(31);
  });

  it('has no duplicate IDs', () => {
    const seen = new Set<string>();
    for (const q of Registry) {
      expect(seen.has(q.id)).toBe(false);
      seen.add(q.id);
    }
  });

  it('every quirk has the required metadata', () => {
    for (const q of Registry) {
      expect(q.id).toBeTruthy();
      expect(q.name).toBeTruthy();
      expect(q.category).toBeTruthy();
      expect(q.severity).toBeTruthy();
      expect(q.description.length).toBeGreaterThan(20);
      expect(q.source).toBeTruthy();
    }
  });

  it('every quirk has at least a scenario or detect', () => {
    for (const q of Registry) {
      expect(q.scenario || q.detect).toBeDefined();
    }
  });

  it('subset filters by category', () => {
    const eth = subset({ category: 'eth' });
    expect(eth.length).toBeGreaterThan(0);
    for (const q of eth) {
      expect(q.category).toBe('eth');
    }
  });

  it('subset filters by minSeverity', () => {
    const crit = subset({ minSeverity: 'critical' });
    for (const q of crit) {
      expect(q.severity).toBe('critical');
    }
  });

  it('subset filters by firmware', () => {
    const v9_10 = subset({ firmware: '9.10.0' });
    for (const q of v9_10) {
      expect(firmwareApplies(q.firmware, '9.10.0')).toBe(true);
    }
  });

  it('subset for old firmware excludes quirks introduced later', () => {
    // M1 (18-words removed) applies only from 9.24.0+
    const before = subset({ firmware: '9.20.0' });
    expect(before.find((q) => q.id === 'M1')).toBeUndefined();
    const after = subset({ firmware: '9.24.0' });
    expect(after.find((q) => q.id === 'M1')).toBeDefined();
  });
});

describe('firmwareApplies', () => {
  it('empty range covers everything', () => {
    expect(firmwareApplies({ min: '', max: '' }, '9.23.0')).toBe(true);
  });

  it('min boundary is inclusive', () => {
    expect(firmwareApplies({ min: '9.20.0', max: '' }, '9.20.0')).toBe(true);
    expect(firmwareApplies({ min: '9.20.0', max: '' }, '9.19.9')).toBe(false);
  });

  it('max boundary is exclusive', () => {
    expect(firmwareApplies({ min: '', max: '9.20.0' }, '9.19.9')).toBe(true);
    expect(firmwareApplies({ min: '', max: '9.20.0' }, '9.20.0')).toBe(false);
  });

  it('strips v prefix', () => {
    expect(firmwareApplies({ min: '9.20.0', max: '' }, 'v9.21.0')).toBe(true);
  });
});
