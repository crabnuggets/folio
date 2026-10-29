import { useId, useState, type ReactNode } from 'react';
import { useForm } from 'react-hook-form';
import { ApiError, errorMessage } from '../lib/api';

export type Values = Record<string, string | boolean>;
export type Field = {
  name: string; label: string; type?: 'text' | 'password' | 'date' | 'month' | 'datetime-local' | 'checkbox' | 'textarea' | 'select' | 'money';
  required?: boolean; hint?: string; options?: { value: string; label: string }[];
  visible?: (values: Values) => boolean;
};

export function Form({ fields, defaults = {}, submit, onSubmit, children, disabled = false }: {
  fields: Field[]; defaults?: Values; submit: string; onSubmit: (values: Values) => Promise<void>;
  children?: ReactNode; disabled?: boolean;
}) {
  const id = useId();
  const { register, handleSubmit, formState: { errors, isSubmitting }, setError, watch } = useForm<Values>({ defaultValues: defaults });
  const values = watch();
  const [error, setFailure] = useState<unknown>();
  return <form onSubmit={handleSubmit(async values => {
    setFailure(undefined);
    try { await onSubmit(values); } catch (failure) {
      setFailure(failure);
      if (failure instanceof ApiError) Object.entries(failure.fields).forEach(([name, message]) => setError(name, { message }));
    }
  })}>
    {children}
    <fieldset disabled={disabled || isSubmitting} className="form-fields">
      {fields.filter(field => !field.visible || field.visible(values)).map(field => {
        const controlId = `${id}-${field.name}`;
        const errorText = errors[field.name]?.message;
        const props = {
          id: controlId,
          'aria-invalid': Boolean(errorText) || undefined,
          'aria-describedby': `${controlId}-hint${errorText ? ` ${controlId}-error` : ''}`,
          ...register(field.name, { required: field.required ? `${field.label} is required.` : false }),
        };
        return <div className={`field ${field.type === 'checkbox' ? 'check-field' : ''}`} key={field.name}>
          <label htmlFor={controlId}>{field.label}{field.required ? <span aria-hidden="true"> *</span> : null}</label>
          {field.type === 'select' ? <select {...props}><option value="">Select…</option>{field.options?.map(option => <option key={option.value} value={option.value}>{option.label}</option>)}</select>
            : field.type === 'textarea' ? <textarea {...props} rows={3} />
            : <input {...props} type={field.type === 'money' ? 'text' : field.type ?? 'text'} inputMode={field.type === 'money' ? 'decimal' : undefined} autoComplete={field.type === 'password' ? 'off' : undefined} spellCheck={field.type === 'password' ? false : undefined} />}
          <small id={`${controlId}-hint`}>{field.hint}</small>
          {errorText && <small className="critical" id={`${controlId}-error`} role="alert">{String(errorText)}</small>}
        </div>;
      })}
    </fieldset>
    {error != null && <div className="notice error" role="alert">{errorMessage(error)}{error instanceof ApiError && error.requestId && <small>Reference: {error.requestId}</small>}</div>}
    <div className="form-actions"><button type="submit" className="primary" disabled={disabled || isSubmitting}>{isSubmitting ? 'Saving…' : submit}</button></div>
  </form>;
}

export const text = (v: Values, key: string) => String(v[key] ?? '').trim();
export const nullable = (v: Values, key: string) => text(v, key) || null;
