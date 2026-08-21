import { cn } from '@/lib/utils';
import * as React from 'react';
import {
  Controller,
  type ControllerProps,
  type FieldPath,
  type FieldValues,
  FormProvider,
  useFormContext,
} from 'react-hook-form';

export const Form = FormProvider;

const FormFieldContext = React.createContext<{ name?: string }>({});

export function FormItem({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn('space-y-2', className)} {...props} />;
}

export function FormLabel({ className, ...props }: React.LabelHTMLAttributes<HTMLLabelElement>) {
  return <label className={cn('text-sm font-medium leading-none', className)} {...props} />;
}

export function FormControl({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn('space-y-2', className)} {...props} />;
}

export function FormMessage({ className, children, ...props }: React.HTMLAttributes<HTMLParagraphElement>) {
  const { name } = React.useContext(FormFieldContext);
  const form = useFormContext();
  const error = name && form ? form.getFieldState(name, form.formState).error : undefined;
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
> = Omit<ControllerProps<TFieldValues, TName>, 'control'> & {
  control?: ControllerProps<TFieldValues, TName>['control'];
};

export function FormField<
  TFieldValues extends FieldValues = FieldValues,
  TName extends FieldPath<TFieldValues> = FieldPath<TFieldValues>,
>({ control, ...props }: FormFieldProps<TFieldValues, TName>) {
  const formContext = useFormContext<TFieldValues>();
  const resolvedControl = control ?? formContext?.control;

  if (!resolvedControl) {
    throw new Error('FormField must be used within a Form provider or be given an explicit control');
  }

  return (
    <FormFieldContext.Provider value={{ name: props.name }}>
      <Controller control={resolvedControl} {...props} />
    </FormFieldContext.Provider>
  );
}
