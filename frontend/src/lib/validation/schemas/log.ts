import { z } from 'zod';

/**
 * Datetime string validation for log time picker.
 * Accepts: YYYY-MM-DDTHH:MM:SSZ, YYYY-MM-DDTHH:MM:SS, YYYY-MM-DD HH:MM:SS, YYYY-MM-DDTHH:MM, YYYY-MM-DD HH:MM
 */
const dateTimePatterns = [
  /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z?$/,
  /^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/,
  /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/,
  /^\d{4}-\d{2}-\d{2} \d{2}:\d{2}$/,
];

export const timePickerSchema = z.object({
  inputTime: z
    .string()
    .min(1, 'Please enter a date/time')
    .refine(
      (val) => dateTimePatterns.some((p) => p.test(val)),
      'Invalid format. Use: YYYY-MM-DD HH:MM:SS'
    ),
});

export type TimePickerFormValues = z.infer<typeof timePickerSchema>;
