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

// Simple non-empty string
export const requiredStringSchema = z.string().min(1, 'This field is required');

// Optional string that trims whitespace
export const optionalStringSchema = z.string().optional();

// Non-negative integer
export const nonNegativeIntSchema = z
  .number()
  .int('Must be an integer')
  .min(0, 'Must be 0 or greater');
