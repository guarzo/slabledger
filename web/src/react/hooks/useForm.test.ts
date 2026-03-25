import { renderHook, act } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import { useForm } from './useForm';

describe('useForm', () => {
  const initialValues = { name: '', email: '', age: 0 };

  it('initializes with given values', () => {
    const { result } = renderHook(() =>
      useForm({ initialValues, onSubmit: vi.fn() })
    );
    expect(result.current.values).toEqual(initialValues);
    expect(result.current.errors).toEqual({});
    expect(result.current.touched).toEqual({});
    expect(result.current.isSubmitting).toBe(false);
    expect(result.current.isDirty).toBe(false);
  });

  it('updates values on handleChange', () => {
    const { result } = renderHook(() =>
      useForm({ initialValues, onSubmit: vi.fn() })
    );
    act(() => result.current.handleChange('name', 'Alice'));
    expect(result.current.values.name).toBe('Alice');
    expect(result.current.isDirty).toBe(true);
  });

  it('marks field as touched on blur', () => {
    const { result } = renderHook(() =>
      useForm({ initialValues, onSubmit: vi.fn() })
    );
    act(() => result.current.handleBlur('name'));
    expect(result.current.touched.name).toBe(true);
  });

  it('validates on submit and blocks when errors exist', () => {
    const onSubmit = vi.fn();
    const validate = (values: typeof initialValues) => {
      const errors: Partial<Record<keyof typeof initialValues, string>> = {};
      if (!values.name) errors.name = 'Required';
      return errors;
    };
    const { result } = renderHook(() =>
      useForm({ initialValues, validate, onSubmit })
    );

    act(() => {
      result.current.handleSubmit({ preventDefault: vi.fn() } as unknown as React.FormEvent);
    });

    expect(onSubmit).not.toHaveBeenCalled();
    expect(result.current.errors.name).toBe('Required');
    expect(result.current.touched.name).toBe(true);
  });

  it('calls onSubmit when validation passes', () => {
    const onSubmit = vi.fn();
    const { result } = renderHook(() =>
      useForm({
        initialValues: { ...initialValues, name: 'Alice' },
        validate: () => ({}),
        onSubmit,
      })
    );

    act(() => {
      result.current.handleSubmit({ preventDefault: vi.fn() } as unknown as React.FormEvent);
    });

    expect(onSubmit).toHaveBeenCalledWith({ ...initialValues, name: 'Alice' });
  });

  it('clears field error when user types', () => {
    const { result } = renderHook(() =>
      useForm({
        initialValues,
        validate: (v) => (!v.name ? { name: 'Required' } : {}),
        onSubmit: vi.fn(),
      })
    );

    // Trigger validation
    act(() => {
      result.current.handleSubmit({ preventDefault: vi.fn() } as unknown as React.FormEvent);
    });
    expect(result.current.errors.name).toBe('Required');

    // Type something
    act(() => result.current.handleChange('name', 'Alice'));
    expect(result.current.errors.name).toBeUndefined();
  });

  it('resets form state', () => {
    const { result } = renderHook(() =>
      useForm({ initialValues, onSubmit: vi.fn() })
    );

    act(() => {
      result.current.handleChange('name', 'Alice');
      result.current.handleBlur('name');
    });
    expect(result.current.isDirty).toBe(true);

    act(() => result.current.reset());
    expect(result.current.values).toEqual(initialValues);
    expect(result.current.touched).toEqual({});
    expect(result.current.isDirty).toBe(false);
  });

  it('resets to new values when provided', () => {
    const { result } = renderHook(() =>
      useForm({ initialValues, onSubmit: vi.fn() })
    );

    const newValues = { name: 'Bob', email: 'bob@test.com', age: 30 };
    act(() => result.current.reset(newValues));
    expect(result.current.values).toEqual(newValues);
    expect(result.current.isDirty).toBe(false);
  });

  it('sets field-level errors', () => {
    const { result } = renderHook(() =>
      useForm({ initialValues, onSubmit: vi.fn() })
    );

    act(() => result.current.setFieldError('email', 'Already taken'));
    expect(result.current.errors.email).toBe('Already taken');
    expect(result.current.touched.email).toBe(true);
  });

  it('handles async onSubmit', async () => {
    const onSubmit = vi.fn(() => Promise.resolve());
    const { result } = renderHook(() =>
      useForm({ initialValues: { ...initialValues, name: 'Alice' }, onSubmit })
    );

    await act(async () => {
      result.current.handleSubmit({ preventDefault: vi.fn() } as unknown as React.FormEvent);
    });

    expect(onSubmit).toHaveBeenCalled();
    expect(result.current.isSubmitting).toBe(false);
  });

  it('validates on blur for touched fields', () => {
    const { result } = renderHook(() =>
      useForm({
        initialValues,
        validate: (v) => (!v.name ? { name: 'Required' } : {}),
        onSubmit: vi.fn(),
      })
    );

    act(() => result.current.handleBlur('name'));
    expect(result.current.errors.name).toBe('Required');
  });

  it('fieldProps returns correct shape', () => {
    const { result } = renderHook(() =>
      useForm({ initialValues, onSubmit: vi.fn() })
    );

    const props = result.current.fieldProps('name');
    expect(props).toHaveProperty('value', '');
    expect(props).toHaveProperty('onChange');
    expect(props).toHaveProperty('onBlur');
    expect(props.error).toBeUndefined();
  });
});
