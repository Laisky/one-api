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

const FormFieldContext = React.createContext<{ error?: FieldError; name?: string }>({});

/** FormItem renders a consistently spaced container from the supplied div attributes and returns it. */
export function FormItem({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn('space-y-2', className)} {...props} />;
}

/** FormLabel renders a styled label from the supplied label attributes and returns it. */
export function FormLabel({ className, ...props }: React.LabelHTMLAttributes<HTMLLabelElement>) {
  return <label className={cn('text-sm font-medium leading-none', className)} {...props} />;
}

/** FormControl renders a styled control container from the supplied div attributes and returns it. */
export function FormControl({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn('space-y-2', className)} {...props} />;
}

/** FormMessage renders the current field error or supplied children from the paragraph attributes and returns it. */
export function FormMessage({ className, children, ...props }: React.HTMLAttributes<HTMLParagraphElement>) {
  const { error } = React.useContext(FormFieldContext);
  const body = error?.message !== undefined ? String(error.message) : children;

  if (!body) return null;

  return (
    <p role="alert" className={cn('text-xs text-destructive', className)} {...props}>
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

/** FormField binds the supplied field props to an explicit or provider control and returns the controlled field. */
export function FormField<
  TFieldValues extends FieldValues = FieldValues,
  TName extends FieldPath<TFieldValues> = FieldPath<TFieldValues>,
  TTransformedValues = TFieldValues,
>({ control, render, ...props }: FormFieldProps<TFieldValues, TName, TTransformedValues>) {
  const formContext = useFormContext<TFieldValues, unknown, TTransformedValues>();
  const resolvedControl = control ?? formContext?.control;

  if (!resolvedControl) {
    throw new Error('FormField must be used within a Form provider or be given an explicit control');
  }

  return (
    <Controller<TFieldValues, TName, TTransformedValues>
      control={resolvedControl}
      {...props}
      render={(fieldProps) => (
        <FormFieldContext.Provider value={{ error: fieldProps.fieldState.error, name: props.name }}>
          {render(fieldProps)}
        </FormFieldContext.Provider>
      )}
    />
  );
}
