import { z } from 'zod';
import { portSchema, requiredStringSchema, optionalStringSchema } from './common';

/**
 * Port forward form schema for pod-specific dialog
 */
export const podPortForwardSchema = z.object({
  localPort: portSchema,
  label: optionalStringSchema.default(''),
  https: z.boolean().default(false),
  favorite: z.boolean().default(false),
  autoStart: z.boolean().default(false),
  keepAlive: z.boolean().default(false),
  startNow: z.boolean().default(true),
  openInBrowser: z.boolean().default(false),
});

export type PodPortForwardFormValues = z.infer<typeof podPortForwardSchema>;

/**
 * Full port forward config schema
 */
export const portForwardConfigSchema = z.object({
  id: optionalStringSchema.default(''),
  context: requiredStringSchema,
  namespace: requiredStringSchema,
  resourceType: z.enum(['pod', 'service']).default('pod'),
  resourceName: requiredStringSchema,
  localPort: portSchema,
  remotePort: portSchema,
  label: optionalStringSchema.default(''),
  favorite: z.boolean().default(false),
  https: z.boolean().default(false),
  autoStart: z.boolean().default(false),
  keepAlive: z.boolean().default(false),
  startNow: z.boolean().default(true),
  openInBrowser: z.boolean().default(false),
});

export type PortForwardConfigFormValues = z.infer<typeof portForwardConfigSchema>;

/**
 * Edit mode schema (less strict - some fields are read-only)
 */
export const portForwardEditSchema = z.object({
  id: requiredStringSchema,
  localPort: portSchema,
  label: optionalStringSchema.default(''),
  favorite: z.boolean().default(false),
  https: z.boolean().default(false),
  autoStart: z.boolean().default(false),
  keepAlive: z.boolean().default(false),
});

export type PortForwardEditFormValues = z.infer<typeof portForwardEditSchema>;
