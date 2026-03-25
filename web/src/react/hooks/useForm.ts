import { useState, useCallback, useRef, useEffect } from 'react';

type ValidationErrors<T> = Partial<Record<keyof T, string>>;

interface UseFormOptions<T extends object> {
  initialValues: T;
  validate?: (values: T) => ValidationErrors<T>;
  onSubmit: (values: T) => void | Promise<void>;
}

export interface UseFormReturn<T extends object> {
  values: T;
  errors: ValidationErrors<T>;
  touched: Partial<Record<keyof T, boolean>>;
  isSubmitting: boolean;
  isDirty: boolean;
  handleChange: (field: keyof T, value: T[keyof T]) => void;
  handleBlur: (field: keyof T) => void;
  handleSubmit: (e: React.FormEvent) => void;
  reset: (newValues?: T) => void;
  setFieldError: (field: keyof T, error: string) => void;
  fieldProps: (field: keyof T) => {
    value: T[keyof T];
    onChange: (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement>) => void;
    onBlur: () => void;
    error?: string;
  };
}

export function useForm<T extends object>({
  initialValues,
  validate,
  onSubmit,
}: UseFormOptions<T>): UseFormReturn<T> {
  const [values, setValues] = useState<T>(initialValues);
  const [errors, setErrors] = useState<ValidationErrors<T>>({});
  const [touched, setTouched] = useState<Partial<Record<keyof T, boolean>>>({});
  const [isSubmitting, setIsSubmitting] = useState(false);
  const initialRef = useRef(initialValues);
  const mountedRef = useRef(true);
  useEffect(() => { mountedRef.current = true; return () => { mountedRef.current = false; }; }, []);

  const isDirty = JSON.stringify(values) !== JSON.stringify(initialRef.current);

  const handleChange = useCallback((field: keyof T, value: T[keyof T]) => {
    setValues(prev => ({ ...prev, [field]: value }));
    // Clear error when user types
    setErrors(prev => {
      if (prev[field]) {
        const next = { ...prev };
        delete next[field];
        return next;
      }
      return prev;
    });
  }, []);

  const handleBlur = useCallback((field: keyof T) => {
    setTouched(prev => ({ ...prev, [field]: true }));
    if (validate) {
      setValues(current => {
        const fieldErrors = validate(current);
        if (fieldErrors[field]) {
          setErrors(prev => ({ ...prev, [field]: fieldErrors[field] }));
        }
        return current;
      });
    }
  }, [validate]);

  const handleSubmit = useCallback((e: React.FormEvent) => {
    e.preventDefault();

    if (validate) {
      const validationErrors = validate(values);
      if (Object.keys(validationErrors).length > 0) {
        setErrors(validationErrors);
        // Mark all fields with errors as touched
        const allTouched: Partial<Record<keyof T, boolean>> = {};
        for (const key of Object.keys(validationErrors) as (keyof T)[]) {
          allTouched[key] = true;
        }
        setTouched(prev => ({ ...prev, ...allTouched }));
        return;
      }
    }

    const result = onSubmit(values);
    if (result instanceof Promise) {
      setIsSubmitting(true);
      result.finally(() => {
        if (mountedRef.current) setIsSubmitting(false);
      });
    }
  }, [values, validate, onSubmit]);

  const reset = useCallback((newValues?: T) => {
    const v = newValues ?? initialRef.current;
    initialRef.current = v;
    setValues(v);
    setErrors({});
    setTouched({});
    setIsSubmitting(false);
  }, []);

  const setFieldError = useCallback((field: keyof T, error: string) => {
    setErrors(prev => ({ ...prev, [field]: error }));
    setTouched(prev => ({ ...prev, [field]: true }));
  }, []);

  const fieldProps = useCallback((field: keyof T) => ({
    value: values[field],
    onChange: (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement>) => {
      handleChange(field, e.target.value as T[keyof T]);
    },
    onBlur: () => handleBlur(field),
    error: touched[field] ? errors[field] : undefined,
  }), [values, errors, touched, handleChange, handleBlur]);

  return {
    values,
    errors,
    touched,
    isSubmitting,
    isDirty,
    handleChange,
    handleBlur,
    handleSubmit,
    reset,
    setFieldError,
    fieldProps,
  };
}
