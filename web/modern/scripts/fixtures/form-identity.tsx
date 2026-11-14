import React, { useState } from 'react';
import { createRoot } from 'react-dom/client';
import { useForm } from 'react-hook-form';
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from '@/components/ui/form';
import { Input } from '@/components/ui/input';

/** IdentityField renders one actual form field with configurable explicit identity and optional matching label. */
export function IdentityField({ label, id, matched = false }: { label: string; id?: string; matched?: boolean }) {
  const form = useForm<{ name: string }>({ defaultValues: { name: '' } });
  return <Form {...form}>
    <form onSubmit={form.handleSubmit(() => undefined)}>
      <FormField name="name" rules={{ required: label + ' is required' }} render={({ field }) => <FormItem>
        <FormLabel htmlFor={matched ? id : undefined}>{label}</FormLabel>
        <FormControl><Input id={id} aria-describedby="name-hint" {...field} /></FormControl>
        <FormMessage />
      </FormItem>} />
      <button type="submit">Save {label}</button>
    </form>
  </Form>;
}

/** FormIdentityFixture exposes generated, explicit, matching and changing ids with real React Hook Form refs. */
export function FormIdentityFixture() {
  const [id, setId] = useState<string | undefined>('custom-name');
  Object.assign(window, { formIdentityFixture: { setId } });
  return <>
    <p id="name-hint">Existing description</p>
    <IdentityField label="Matched" id="matching-name" matched />
    <IdentityField label="Generated" />
    <IdentityField label="Name" id={id} />
  </>;
}
createRoot(document.getElementById('root')!).render(<FormIdentityFixture />);
