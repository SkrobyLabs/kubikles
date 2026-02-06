import { useState, useCallback, useMemo } from 'react';
import { z, ZodError, ZodSchema } from 'zod';

type FieldErrors<T> = Partial<Record<keyof T, string>>;
type TouchedFields<T> = Partial<Record<keyof T, boolean>>;

interface UseFormOptions<T extends z.ZodType> {
  schema: T;
  initialValues: z.infer<T>;
  onSubmit: (values: z.infer<T>) => Promise<void> | void;
  validateOnChange?: boolean;
  validateOnBlur?: boolean;
}

interface FieldProps {
  value: any;
  onChange: (
    e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement>
  ) => void;
  onBlur: () => void;
  name: string;
  [key: string]: any;
}

interface UseFormReturn<T extends z.ZodType> {
  values: z.infer<T>;
  errors: FieldErrors<z.infer<T>>;
  touched: TouchedFields<z.infer<T>>;
  isSubmitting: boolean;
  isValid: boolean;
  submitError: string | null;

  // Field helpers
  setValue: <K extends keyof z.infer<T>>(field: K, value: z.infer<T>[K]) => void;
  setValues: (values: Partial<z.infer<T>>) => void;
  setFieldTouched: (field: keyof z.infer<T>, touched?: boolean) => void;
  setFieldError: (field: keyof z.infer<T>, error: string | undefined) => void;
  getFieldProps: (field: keyof z.infer<T>) => FieldProps;

  // Form actions
  handleSubmit: (e?: React.FormEvent) => Promise<void>;
  reset: (values?: z.infer<T>) => void;
  validate: () => boolean;
  validateField: (field: keyof z.infer<T>) => string | undefined;

  // Error helpers
  setSubmitError: (error: string | null) => void;
  clearErrors: () => void;
}

function getZodErrors<T>(error: ZodError, values: T): FieldErrors<T> {
  const errors: FieldErrors<T> = {};
  for (const issue of error.issues) {
    const path = issue.path[0] as keyof T;
    if (path && !errors[path]) {
      errors[path] = issue.message;
    }
  }
  return errors;
}

export function useForm<T extends ZodSchema>({
  schema,
  initialValues,
  onSubmit,
  validateOnChange = true,
  validateOnBlur = true,
}: UseFormOptions<T>): UseFormReturn<T> {
  type FormValues = z.infer<T>;

  const [values, setValuesState] = useState<FormValues>(initialValues);
  const [errors, setErrors] = useState<FieldErrors<FormValues>>({});
  const [touched, setTouched] = useState<TouchedFields<FormValues>>({});
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);

  // Validate all fields
  const validate = useCallback((): boolean => {
    const result = schema.safeParse(values);
    if (!result.success) {
      setErrors(getZodErrors(result.error, values));
      return false;
    }
    setErrors({});
    return true;
  }, [schema, values]);

  // Validate a single field
  const validateField = useCallback(
    (field: keyof FormValues): string | undefined => {
      const result = schema.safeParse(values);
      if (!result.success) {
        const fieldError = result.error.issues.find((issue) => issue.path[0] === field);
        return fieldError?.message;
      }
      return undefined;
    },
    [schema, values]
  );

  // Set a single value
  const setValue = useCallback(
    <K extends keyof FormValues>(field: K, value: FormValues[K]) => {
      setValuesState((prev) => {
        const newValues = { ...(prev as any), [field]: value };

        if (validateOnChange) {
          const result = schema.safeParse(newValues);
          if (!result.success) {
            const fieldError = result.error.issues.find((issue) => issue.path[0] === field);
            setErrors((prevErrors) => ({
              ...prevErrors,
              [field]: fieldError?.message,
            }));
          } else {
            setErrors((prevErrors) => {
              const { [field]: _, ...rest } = prevErrors;
              return rest as FieldErrors<FormValues>;
            });
          }
        }

        return newValues;
      });
      setSubmitError(null);
    },
    [schema, validateOnChange]
  );

  // Set multiple values
  const setValues = useCallback(
    (newValues: Partial<FormValues>) => {
      setValuesState((prev) => ({ ...(prev as any), ...newValues }));
      if (validateOnChange) {
        // Re-validate after setting values
        setTimeout(() => validate(), 0);
      }
      setSubmitError(null);
    },
    [validate, validateOnChange]
  );

  // Set field touched state
  const setFieldTouched = useCallback(
    (field: keyof FormValues, isTouched = true) => {
      setTouched((prev) => ({ ...prev, [field]: isTouched }));

      if (validateOnBlur && isTouched) {
        const error = validateField(field);
        setErrors((prev) => ({
          ...prev,
          [field]: error,
        }));
      }
    },
    [validateField, validateOnBlur]
  );

  // Set a specific field error manually
  const setFieldError = useCallback((field: keyof FormValues, error: string | undefined) => {
    setErrors((prev) => ({
      ...prev,
      [field]: error,
    }));
  }, []);

  // Get props for a field (for easy binding)
  const getFieldProps = useCallback(
    (field: keyof FormValues): FieldProps => ({
      name: field as string,
      value: values[field] as string | number | boolean,
      onChange: (
        e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement>
      ) => {
        const target = e.target;
        let newValue: unknown;

        if (target instanceof HTMLInputElement) {
          if (target.type === 'checkbox') {
            newValue = target.checked;
          } else if (target.type === 'number') {
            newValue = target.value === '' ? '' : target.value;
          } else {
            newValue = target.value;
          }
        } else {
          newValue = target.value;
        }

        setValue(field, newValue as FormValues[keyof FormValues]);
      },
      onBlur: () => setFieldTouched(field, true),
    }),
    [values, setValue, setFieldTouched]
  );

  // Handle form submission
  const handleSubmit = useCallback(
    async (e?: React.FormEvent) => {
      e?.preventDefault();

      // Mark all fields as touched
      const allTouched = Object.keys(values as any).reduce(
        (acc: any, key: string) => ({ ...acc, [key]: true }),
        {} as TouchedFields<FormValues>
      );
      setTouched(allTouched);

      // Validate
      const result = schema.safeParse(values);
      if (!result.success) {
        setErrors(getZodErrors(result.error, values));
        return;
      }

      setErrors({});
      setIsSubmitting(true);
      setSubmitError(null);

      try {
        await onSubmit(result.data);
      } catch (err) {
        const message = err instanceof Error ? err.message : 'An error occurred';
        setSubmitError(message);
      } finally {
        setIsSubmitting(false);
      }
    },
    [schema, values, onSubmit]
  );

  // Reset form to initial or provided values
  const reset = useCallback(
    (newValues?: FormValues) => {
      setValuesState(newValues ?? initialValues);
      setErrors({});
      setTouched({});
      setSubmitError(null);
    },
    [initialValues]
  );

  // Clear all errors
  const clearErrors = useCallback(() => {
    setErrors({});
    setSubmitError(null);
  }, []);

  // Check if form is valid
  const isValid = useMemo(() => {
    const result = schema.safeParse(values);
    return result.success;
  }, [schema, values]);

  return {
    values,
    errors,
    touched,
    isSubmitting,
    isValid,
    submitError,

    setValue,
    setValues,
    setFieldTouched,
    setFieldError,
    getFieldProps,

    handleSubmit,
    reset,
    validate,
    validateField,

    setSubmitError,
    clearErrors,
  };
}

// Re-export z for convenience
export { z } from 'zod';
