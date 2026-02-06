import { z } from 'zod';

/**
 * Common Zod schemas for form validation
 */

// Port number: 1-65535
export const portSchema = z
  .number()
  .int('Port must be an integer')
  .min(1, 'Port must be at least 1')
  .max(65535, 'Port must be at most 65535');

// Port as string input (for form fields)
export const portStringSchema = z
  .string()
  .min(1, 'Port is required')
  .refine((val) => /^\d+$/.test(val), 'Port must be a number')
  .transform((val) => parseInt(val, 10))
  .pipe(portSchema);

// Optional port as string
export const optionalPortStringSchema = z
  .string()
  .optional()
  .refine((val) => !val || /^\d+$/.test(val), 'Port must be a number')
  .transform((val) => (val ? parseInt(val, 10) : undefined));

// URL validation
export const urlSchema = z
  .string()
  .min(1, 'URL is required')
  .refine((val) => {
    try {
      new URL(val);
      return true;
    } catch {
      return false;
    }
  }, 'Invalid URL format');

// Optional URL
export const optionalUrlSchema = z
  .string()
  .optional()
  .refine(
    (val) => {
      if (!val) return true;
      try {
        new URL(val);
        return true;
      } catch {
        return false;
      }
    },
    { message: 'Invalid URL format' }
  );

// Kubernetes resource name (RFC 1123 DNS subdomain)
// - max 253 characters
// - lowercase alphanumeric, hyphens, dots
// - must start and end with alphanumeric
export const k8sNameSchema = z
  .string()
  .min(1, 'Name is required')
  .max(253, 'Name must be at most 253 characters')
  .regex(
    /^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$/,
    'Name must be lowercase alphanumeric with hyphens/dots, starting and ending with alphanumeric'
  );

// Kubernetes namespace name (RFC 1123 DNS label)
// - max 63 characters
// - lowercase alphanumeric and hyphens
// - must start with letter, end with alphanumeric
export const namespaceSchema = z
  .string()
  .min(1, 'Namespace is required')
  .max(63, 'Namespace must be at most 63 characters')
  .regex(
    /^[a-z]([-a-z0-9]*[a-z0-9])?$/,
    'Namespace must be lowercase, start with a letter, and contain only alphanumeric characters and hyphens'
  );

// Simple non-empty string
export const requiredStringSchema = z.string().min(1, 'This field is required');

// Optional string that trims whitespace
export const optionalStringSchema = z.string().optional();

// Positive integer
export const positiveIntSchema = z
  .number()
  .int('Must be an integer')
  .positive('Must be a positive number');

// Non-negative integer
export const nonNegativeIntSchema = z
  .number()
  .int('Must be an integer')
  .min(0, 'Must be 0 or greater');

// Email validation
export const emailSchema = z.string().email('Invalid email address');

// IP address (v4)
const ipv4Regex =
  /^(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$/;
export const ipv4Schema = z.string().refine((val) => ipv4Regex.test(val), 'Invalid IPv4 address');

// Hostname or IP
export const hostSchema = z
  .string()
  .min(1, 'Host is required')
  .refine((val) => {
    // Check if valid IPv4
    const ipv4Regex =
      /^(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$/;
    if (ipv4Regex.test(val)) return true;

    // Check if valid hostname
    const hostnameRegex =
      /^(?=.{1,253}$)(?:(?!-)[a-zA-Z0-9-]{1,63}(?<!-)\.)*(?!-)[a-zA-Z0-9-]{1,63}(?<!-)$/;
    return hostnameRegex.test(val);
  }, 'Must be a valid hostname or IP address');

// Type exports for inference
export type Port = z.infer<typeof portSchema>;
export type K8sName = z.infer<typeof k8sNameSchema>;
export type Namespace = z.infer<typeof namespaceSchema>;
