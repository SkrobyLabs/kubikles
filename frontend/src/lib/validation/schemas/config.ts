import { z } from 'zod';

/**
 * Zod schema for app configuration validation
 * Mirrors the structure in ConfigContext.tsx but provides runtime validation
 */

// Log search config
const logSearchConfigSchema = z.object({
  debounceMs: z.number().int().min(0).max(2000).optional(),
  searchOnEnter: z.boolean().optional(),
  useRegex: z.boolean().optional(),
  filterOnly: z.boolean().optional(),
  contextLinesBefore: z.number().int().min(0).max(20).optional(),
  contextLinesAfter: z.number().int().min(0).max(20).optional(),
});

// Logs config
const logsConfigSchema = z.object({
  lineWrap: z.boolean().optional(),
  showTimestamps: z.boolean().optional(),
  position: z.enum(['start', 'end', 'all']).optional(),
  search: logSearchConfigSchema.optional(),
});

// Port forwards config
const portForwardsConfigSchema = z.object({
  autoStartMode: z.enum(['all', 'favorites', 'none']).optional(),
});

// AI config
const aiConfigSchema = z.object({
  model: z.string().optional(),
  panelWidth: z.number().int().min(280).max(800).optional(),
  requestTimeout: z.number().int().min(1).max(60).optional(),
  allowedTools: z.array(z.string()).optional(),
});

// Sidebar layout section
const sidebarLayoutSectionSchema = z.object({
  id: z.string(),
  title: z.string(),
  items: z.array(z.string()),
  isCustom: z.boolean().optional(),
  itemLabels: z.record(z.string()).optional(),
});

// Sidebar config
const sidebarConfigSchema = z.object({
  layout: z.array(sidebarLayoutSectionSchema).optional(),
});

// UI config
const uiConfigSchema = z.object({
  searchDebounceMs: z.number().int().min(0).max(1000).optional(),
  copyFeedbackMs: z.number().int().min(500).max(5000).optional(),
  scrollZoomEnabled: z.boolean().optional(),
  showTabIcons: z.boolean().optional(),
  sidebar: sidebarConfigSchema.optional(),
});

// Kubernetes config
const kubernetesConfigSchema = z.object({
  apiTimeoutMs: z.number().int().min(10000).max(300000).optional(),
  metricsPollIntervalMs: z.number().int().min(5000).max(300000).optional(),
  connectionTestTimeoutSeconds: z.number().int().min(1).max(30).optional(),
  nodeDebugImage: z.string().optional(),
});

// Metrics config
const metricsConfigSchema = z.object({
  preferredSource: z.enum(['auto', 'k8s', 'prometheus']).optional(),
});

// Performance config
const performanceConfigSchema = z.object({
  pollIntervalMs: z.number().int().min(500).max(10000).optional(),
  eventCoalescerMs: z.number().int().min(1).max(100).optional(),
  enableRequestCancellation: z.boolean().optional(),
  forceHttp1: z.boolean().optional(),
  clientPoolSize: z.number().int().min(0).max(10).optional(),
});

// Debug config
const debugConfigSchema = z.object({
  showLogSourceMarkers: z.boolean().optional(),
});

/**
 * Complete app config schema
 * All fields are optional since we deep-merge with defaults
 */
export const appConfigSchema = z.object({
  logs: logsConfigSchema.optional(),
  portForwards: portForwardsConfigSchema.optional(),
  ai: aiConfigSchema.optional(),
  ui: uiConfigSchema.optional(),
  kubernetes: kubernetesConfigSchema.optional(),
  metrics: metricsConfigSchema.optional(),
  performance: performanceConfigSchema.optional(),
  debug: debugConfigSchema.optional(),
});

export type AppConfigValidated = z.infer<typeof appConfigSchema>;

/**
 * Validate config with graceful degradation
 * Returns validated config or null if validation fails
 */
export function validateConfig(config: unknown): {
  valid: boolean;
  data: AppConfigValidated | null;
  issues: z.ZodIssue[];
} {
  const result = appConfigSchema.safeParse(config);
  if (result.success) {
    return { valid: true, data: result.data, issues: [] };
  }
  return { valid: false, data: null, issues: result.error.issues };
}
