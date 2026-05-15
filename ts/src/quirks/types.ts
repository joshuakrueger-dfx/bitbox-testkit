/**
 * Types mirroring /go/bitbox/quirks/registry.go. Keep field names in sync
 * with quirks.json schema; see /quirks/SCHEMA.md for canonical semantics.
 */

export type Category =
  | 'eth'
  | 'btc'
  | 'cardano'
  | 'mnemonic'
  | 'protocol'
  | 'app';

export type Severity = 'hint' | 'warning' | 'critical';

export interface FirmwareRange {
  /** inclusive lower bound; empty means "from the beginning" */
  min: string;
  /** exclusive upper bound; empty means "forever" */
  max: string;
}

/**
 * Data-driven static-detection rule, loaded verbatim from quirks.json.
 * Mirrors Go's quirks.DetectRule.
 */
export interface DetectRule {
  /** "regex" | "regex_in_context" | "ordered_pair" */
  readonly kind: string;
  readonly regex?: string;
  readonly context_regex?: string;
  readonly before_regex?: string;
  readonly after_regex?: string;
  readonly file_globs?: readonly string[];
  readonly reason: string;
  readonly fix_hint?: string;
}

/**
 * One documented BitBox firmware constraint.
 */
export interface Quirk {
  readonly id: string;
  readonly name: string;
  readonly category: Category;
  readonly severity: Severity;
  readonly description: string;
  readonly source: string;
  readonly firmware: FirmwareRange;
  /** regex (as a string) matching Jest output that could indicate this quirk */
  readonly matchRegex?: string;

  /** Data-driven static-detection rules. Empty means no static signature. */
  readonly patterns?: readonly DetectRule[];

  /**
   * Source-level static check. Consumer passes a glob of files to scan.
   * Higher-level than `patterns` — handles complex/code-driven detection.
   */
  detect?: (sourcePaths: string[]) => readonly DetectFinding[];

  /**
   * Scenario factory: returns a mock setup function. Pass it to
   * `installMocks()` (or call inside `jest.mock()`) to make the bitbox-api
   * surface return the documented firmware response.
   */
  scenario?: () => unknown;
}

export interface DetectFinding {
  readonly file: string;
  readonly line: number;
  readonly snippet: string;
  readonly reason: string;
}

/**
 * Filter applied to the global registry.
 */
export interface Filter {
  category?: Category;
  /** Lower bound on severity ranking: hint < warning < critical. */
  minSeverity?: Severity;
  /** Firmware version (e.g. "9.23.0"). When provided, only quirks whose
   *  FirmwareRange applies to this version are returned. */
  firmware?: string;
}
