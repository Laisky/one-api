import { cn } from '@/lib/utils';
import * as React from 'react';
import {
  Controller,
  type ControllerProps,
  type FieldError,
  type FieldPath,
  type FieldValues,
  FormProvider,
  useFormContext,
} from 'react-hook-form';

export const Form = FormProvider;

type FormFieldContextValue = {
  error?: FieldError;
  name?: string;
  id?: string;
  messageId?: string;
  registerControlId?: (id: string | undefined) => () => void;
};

const FormFieldContext = React.createContext<FormFieldContextValue>({});

/** NATIVE_CONTROL_TAGS lists DOM elements FormControl may wire directly; other DOM children are layout wrappers. */
const NATIVE_CONTROL_TAGS = new Set(['input', 'textarea', 'select', 'button']);

/**
 * useFormField reads the surrounding FormField context.
 * It returns the effective control identity, stable error-message identity, current error, field name and registrar;
 * the ids are undefined outside a FormField so standalone labels and controls keep their explicit wiring.
 */
function useFormField() {
  return React.useContext(FormFieldContext);
}

/**
 * mergeIdRefs joins whitespace-separated id references while dropping empty and duplicate entries.
 * It takes any number of optional reference lists and returns the merged list, or undefined when none remain.
 */
function mergeIdRefs(...refs: Array<string | undefined>): string | undefined {
  const ids = new Set(refs.flatMap((ref) => (ref ? ref.split(/\s+/).filter(Boolean) : [])));
  return ids.size > 0 ? [...ids].join(' ') : undefined;
}

/** FormItem renders a consistently spaced container from the supplied div attributes and returns it. */
export function FormItem({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn('space-y-2', className)} {...props} />;
}

/**
 * FormLabel renders a styled label from the supplied label attributes and returns it.
 * Inside a FormField it uses the control's effective id unless an explicit `htmlFor` is given.
 */
export function FormLabel({ className, htmlFor, ...props }: React.LabelHTMLAttributes<HTMLLabelElement>) {
  const { id } = useFormField();
  return <label className={cn('text-sm font-medium leading-none', className)} htmlFor={htmlFor ?? id} {...props} />;
}

/**
 * FormControl renders a styled control container from the supplied div attributes and returns it.
 * Inside a FormField it registers an explicit child id with the shared identity owner before paint, or uses its
 * generated fallback. It adds `aria-invalid` and appends the stable error-message id to `aria-describedby` only while
 * an error exists. Layout wrappers such as a `div` are left untouched so they keep their explicit wiring.
 */
export function FormControl({ className, children, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  const { id, messageId, error, registerControlId } = useFormField();
  const child = React.isValidElement<Record<string, unknown>>(children) ? children : null;
  const isWireable = Boolean(child && (typeof child.type !== 'string' || NATIVE_CONTROL_TAGS.has(child.type)));
  const explicitId = child?.props.id as string | undefined;

  React.useLayoutEffect(() => {
    if (isWireable) return registerControlId?.(explicitId);
  }, [isWireable, explicitId, registerControlId]);

  const content = id && child && isWireable
    ? React.cloneElement(child, {
      id: explicitId ?? id,
      'aria-invalid': (child.props['aria-invalid'] as boolean | undefined) ?? Boolean(error),
      'aria-describedby': mergeIdRefs(child.props['aria-describedby'] as string | undefined, error ? messageId : undefined),
    })
    : children;

  return (
    <div className={cn('space-y-2', className)} {...props}>
      {content}
    </div>
  );
}

/**
 * FormMessage renders the current field error or supplied children from the paragraph attributes and returns it.
 * It is not a live region: react-hook-form focuses the first invalid field on submit, and FormControl links this
 * message through `aria-describedby`, so screen readers announce it with the focused field instead of all at once.
 */
export function FormMessage({ className, children, ...props }: React.HTMLAttributes<HTMLParagraphElement>) {
  const { error, messageId } = useFormField();
  const body = error?.message !== undefined ? String(error.message) : children;

  if (!body) return null;

  return (
    <p id={messageId} className={cn('text-xs text-destructive', className)} {...props}>
      {body}
    </p>
  );
}

type FormFieldProps<
  TFieldValues extends FieldValues = FieldValues,
  TName extends FieldPath<TFieldValues> = FieldPath<TFieldValues>,
  TTransformedValues = TFieldValues,
> = Omit<ControllerProps<TFieldValues, TName, TTransformedValues>, 'control'> & {
  control?: ControllerProps<TFieldValues, TName, TTransformedValues>['control'];
};

/**
 * FormField binds the supplied field props to an explicit or provider control and returns the controlled field.
 * It owns the effective control id shared by label and control, plus a stable error-message id.
 * FormControl may register an explicit id without replacing its child ref or changing the error identity.
 */
export function FormField<
  TFieldValues extends FieldValues = FieldValues,
  TName extends FieldPath<TFieldValues> = FieldPath<TFieldValues>,
  TTransformedValues = TFieldValues,
>({ control, render, ...props }: FormFieldProps<TFieldValues, TName, TTransformedValues>) {
  const formContext = useFormContext<TFieldValues, unknown, TTransformedValues>();
  const resolvedControl = control ?? formContext?.control;
  const id = React.useId();
  const [controlId, setControlId] = React.useState<string | undefined>();
  const activeControlRef = React.useRef<symbol | null>(null);
  /** registerControlId adopts a mounted control's explicit id and returns an ownership-safe cleanup. */
  const registerControlId = React.useCallback((explicitId: string | undefined) => {
    const owner = Symbol('form-control');
    activeControlRef.current = owner;
    setControlId(explicitId);
    return () => {
      if (activeControlRef.current === owner) {
        activeControlRef.current = null;
        setControlId(undefined);
      }
    };
  }, []);

  if (!resolvedControl) {
    throw new Error('FormField must be used within a Form provider or be given an explicit control');
  }

  return (
    <Controller<TFieldValues, TName, TTransformedValues>
      control={resolvedControl}
      {...props}
      render={(fieldProps) => (
        <FormFieldContext.Provider value={{ error: fieldProps.fieldState.error, name: props.name, id: controlId ?? id, messageId: `${id}-message`, registerControlId }}>
          {render(fieldProps)}
        </FormFieldContext.Provider>
      )}
    />
  );
}
