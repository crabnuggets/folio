import { useId, useState, type ReactNode } from 'react';
import { useForm } from 'react-hook-form';
import { ApiError, errorMessage } from '../lib/api';

export type Values = Record<string, string | boolean>;
export type Field = {
  name: string; label: string; type?: 'text' | 'password' | 'date' | 'month' | 'datetime-local' | 'checkbox' | 'textarea' | 'select' | 'money';
  required?: boolean; hint?: string; options?: { value: string; label: string }[];
  visible?: (values: Values) => boolean;
};

export function Form({ fields, defaults = {}, submit, onSubmit, children, disabled = false, formId }: {
  fields: Field[]; defaults?: Values; submit: string; onSubmit: (values: Values) => Promise<void>;
  children?: ReactNode; disabled?: boolean;
  /** Submitted by a button elsewhere (`form={formId}`), so the form renders no action row. */
  formId?: string;
}) {
  const id = useId();
  const { register, handleSubmit, formState: { errors, isSubmitting }, setError, watch } = useForm<Values>({ defaultValues: defaults });
  const values = watch();
  const [error, setFailure] = useState<unknown>();
  return <form id={formId} onSubmit={handleSubmit(async values => {
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
        const hint = <small id={`${controlId}-hint`}>{field.hint}</small>;
        const failure = errorText && <small className="critical" id={`${controlId}-error`} role="alert">{String(errorText)}</small>;
        // Checkbox first, then its label with the hint beneath, so reading and visual order agree.
        if (field.type === 'checkbox') return <div className="field check-field" key={field.name}>
          <input {...props} type="checkbox" /><div className="check-text"><label htmlFor={controlId}>{field.label}</label>{hint}{failure}</div>
        </div>;
        return <div className={field.type === 'money' ? 'field money-field' : 'field'} key={field.name}>
          <label htmlFor={controlId}>{field.label}</label>
          {field.type === 'select' ? <select {...props}><option value="">Select…</option>{field.options?.map(option => <option key={option.value} value={option.value}>{option.label}</option>)}</select>
            : field.type === 'textarea' ? <textarea {...props} rows={3} />
            : <input {...props} className={field.type === 'money' ? 'amount-input' : undefined} type={field.type === 'money' ? 'text' : field.type ?? 'text'} inputMode={field.type === 'money' ? 'decimal' : undefined} autoComplete={field.type === 'password' ? 'off' : undefined} spellCheck={field.type === 'password' ? false : undefined} />}
          {hint}{failure}
        </div>;
      })}
    </fieldset>
    {error != null && <div className="notice error" role="alert">{errorMessage(error)}{error instanceof ApiError && error.requestId && <small>Reference: {error.requestId}</small>}</div>}
    {!formId && <div className="form-actions"><button type="submit" className="primary" disabled={disabled || isSubmitting}>{isSubmitting ? 'Saving…' : submit}</button></div>}
  </form>;
}

export const text = (v: Values, key: string) => String(v[key] ?? '').trim();
export const nullable = (v: Values, key: string) => text(v, key) || null;
