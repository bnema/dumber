// GENERATED FROM GO FILES: SEE THE GENERATOR SCRIPT
// Auto-generated TypeScript types from Go structs
// Generated on: 2025-09-22T12:41:27.395Z
// Generator script: scripts/generate-types.js
// Source files: internal/db/models.go, internal/parser/types.go
// Do not edit manually - run 'node scripts/generate-types.js' to regenerate

export interface CertificateValidation {
  id: number;
  hostname: string;
  certificate_hash: string;
  user_decision: string;
  created_at: string | null;
  expires_at: string | null;
}

export interface History {
  id: number;
  url: string;
  title: string | null;
  visit_count: number | null;
  last_visited: string | null;
  created_at: string | null;
  favicon_url: string | null;
}

export interface Shortcut {
  id: number;
  shortcut: string;
  url_template: string;
  description: string | null;
  created_at: string | null;
}

export interface ZoomLevel {
  domain: string;
  zoom_factor: number;
  updated_at: string | null;
}

export interface ParseResult {
  type: InputType;
  url: string;
  query: string;
  confidence: number;
  fuzzy_matches?: FuzzyMatch[];
  shortcut?: DetectedShortcut;
  processing_time: number;
}

export interface DetectedShortcut {
  key: string;
  query: string;
  url: string;
  description: string;
}

export interface FuzzyMatch {
  history_entry?: History;
  score: number;
  url_score: number;
  title_score: number;
  recency_score: number;
  visit_score: number;
  matched_field: string;
}

export interface FuzzyConfig {
  min_similarity_threshold: number;
  max_results: number;
  url_weight: number;
  title_weight: number;
  recency_weight: number;
  visit_weight: number;
  recency_decay_days: number;
}

// Manual types and enums

// Type aliases for frontend usage
export type HistoryEntry = History;
export type SearchShortcut = Shortcut;
export type ZoomEntry = ZoomSetting;
