import { z } from 'zod';

/**
 * Scale replica count schema
 */
export const scaleSchema = z.object({
  replicas: z.number().int('Replicas must be an integer').min(0, 'Minimum 0 replicas').max(100, 'Maximum 100 replicas'),
});

export type ScaleFormValues = z.infer<typeof scaleSchema>;
