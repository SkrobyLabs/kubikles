import { z } from 'zod';
import { urlSchema, nonNegativeIntSchema, requiredStringSchema } from './common';

/**
 * Helm repository add form schema
 */
export const helmRepoSchema = z.object({
  name: requiredStringSchema.regex(
    /^[a-zA-Z0-9]([a-zA-Z0-9_-]*[a-zA-Z0-9])?$/,
    'Name must start and end with alphanumeric, can contain hyphens and underscores'
  ),
  url: urlSchema,
  priority: nonNegativeIntSchema.default(100),
});

export type HelmRepoFormValues = z.infer<typeof helmRepoSchema>;

/**
 * OCI registry login form schema
 */
export const ociLoginSchema = z.object({
  registry: requiredStringSchema.refine(
    (val) => {
      // Basic registry validation - hostname with optional port
      const registryRegex =
        /^(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}(?::\d{1,5})?$/;
      return registryRegex.test(val);
    },
    { message: 'Invalid registry format (e.g., myregistry.azurecr.io)' }
  ),
  username: z.string().optional(),
  password: z.string().optional(),
});

// Schema for basic login (requires username and password)
export const ociBasicLoginSchema = ociLoginSchema.extend({
  username: requiredStringSchema,
  password: requiredStringSchema,
});

export type OCILoginFormValues = z.infer<typeof ociLoginSchema>;
export type OCIBasicLoginFormValues = z.infer<typeof ociBasicLoginSchema>;
