/**
 * Loads the canonical quirks JSON (shared with the Go side) into typed
 * Quirk objects. Callbacks (detect/scenario) are attached by ID in
 * callbacks.ts.
 */

import rawJson from './quirks.json' with { type: 'json' };
import type { Quirk, Category, Severity, FirmwareRange, Filter, DetectRule } from './types.js';
import { attachCallbacks } from './callbacks.js';

interface RawQuirk {
  id: string;
  name: string;
  category: string;
  severity: string;
  description: string;
  source: string;
  firmware: FirmwareRange;
  match_regex?: string;
  detect?: DetectRule[];
}

interface RawRegistry {
  schema_version: string;
  description: string;
  quirks: RawQuirk[];
}

function buildRegistry(): readonly Quirk[] {
  const raw = rawJson as unknown as RawRegistry;
  const out: Quirk[] = [];
  const seen = new Set<string>();

  for (const rq of raw.quirks) {
    if (seen.has(rq.id)) {
      throw new Error(`bitbox-testkit/quirks: duplicate ID ${rq.id} in quirks.json`);
    }
    seen.add(rq.id);

    const q: Quirk = {
      id: rq.id,
      name: rq.name,
      category: rq.category as Category,
      severity: rq.severity as Severity,
      description: rq.description,
      source: rq.source,
      firmware: rq.firmware,
      matchRegex: rq.match_regex,
      patterns: rq.detect,
    };
    attachCallbacks(q);
    out.push(q);
  }
  return out;
}

/** All known quirks. Frozen at module-load time. */
export const Registry: readonly Quirk[] = buildRegistry();

/** Severity rank used by Filter.minSeverity. */
export function severityRank(s: Severity): number {
  switch (s) {
    case 'hint':
      return 0;
    case 'warning':
      return 1;
    case 'critical':
      return 2;
  }
}

/** True if range covers version v. Empty v always passes. */
export function firmwareApplies(range: FirmwareRange, v: string): boolean {
  if (!v) return true;
  if (range.min && compareVersion(v, range.min) < 0) return false;
  if (range.max && compareVersion(v, range.max) >= 0) return false;
  return true;
}

/** Returns the subset of quirks matching the filter. */
export function subset(filter: Filter): Quirk[] {
  return Registry.filter((q) => {
    if (filter.category && q.category !== filter.category) return false;
    if (filter.minSeverity && severityRank(q.severity) < severityRank(filter.minSeverity)) {
      return false;
    }
    if (filter.firmware && !firmwareApplies(q.firmware, filter.firmware)) return false;
    return true;
  });
}

function compareVersion(a: string, b: string): number {
  const pa = splitVersion(a);
  const pb = splitVersion(b);
  const max = Math.max(pa.length, pb.length);
  for (let i = 0; i < max; i++) {
    const ai = pa[i] ?? 0;
    const bi = pb[i] ?? 0;
    if (ai < bi) return -1;
    if (ai > bi) return 1;
  }
  return 0;
}

function splitVersion(v: string): number[] {
  const stripped = v.startsWith('v') ? v.slice(1) : v;
  return stripped.split('.').map((p) => {
    const cut = Math.min(...['-', '+'].map((c) => (p.indexOf(c) === -1 ? Infinity : p.indexOf(c))));
    const numeric = Number.isFinite(cut) ? p.slice(0, cut) : p;
    const n = parseInt(numeric, 10);
    return Number.isNaN(n) ? 0 : n;
  });
}
