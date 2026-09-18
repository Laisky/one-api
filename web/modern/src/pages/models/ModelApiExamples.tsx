import { CopyButton } from '@/components/ui/copy-button';
import { useId, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { buildModelApiExamples, type ModelApiMetadata } from './model-api-examples';

/** ModelApiExamplesProps supplies the selected catalog model without access to any credentials. */
interface ModelApiExamplesProps {
  modelName: string;
  data: ModelApiMetadata;
}

/** ExampleCodeBlock displays a scrollable code sample and copies exactly the displayed text. */
function ExampleCodeBlock({ title, text, copyLabel }: { title: string; text: string; copyLabel: string }) {
  return (
    <div className="min-w-0 space-y-2">
      <div className="flex items-center justify-between gap-2">
        <h4 className="text-xs font-semibold">{title}</h4>
        <CopyButton key={text} text={text} label={copyLabel} className="h-7 w-7 shrink-0 p-0" />
      </div>
      <pre aria-label={title} tabIndex={0} className="max-h-64 max-w-full overflow-auto rounded-lg border bg-muted/30 p-3 text-xs leading-relaxed">
        <code>{text}</code>
      </pre>
    </div>
  );
}

/** ModelApiExamples renders endpoint, curl request, and response examples in either modal layout. */
export function ModelApiExamples({ modelName, data }: ModelApiExamplesProps) {
  const { t } = useTranslation();
  const id = useId();
  const examples = useMemo(
    () => buildModelApiExamples(modelName, data, typeof window === 'undefined' ? '' : window.location.origin),
    [modelName, data]
  );
  const [selectedId, setSelectedId] = useState<string>();
  // A newly selected model can expose a different API family. Never retain an
  // example from the previous model when its selected format is unavailable.
  const example = examples.find((item) => item.id === selectedId) ?? examples[0];

  return (
    <section aria-labelledby={`${id}-title`} className="min-w-0 max-w-full space-y-3">
      <h3 id={`${id}-title`} className="text-sm font-semibold tracking-wide">{t('modelApi.title')}</h3>
      <div className="min-w-0 space-y-4 rounded-xl border bg-card p-4">
        <p className="text-xs leading-relaxed text-muted-foreground">{t('modelApi.description')}</p>
        {examples.length > 1 ? (
          <div className="space-y-1.5">
            <label htmlFor={`${id}-format`} className="text-xs font-semibold">{t('modelApi.format')}</label>
            <select
              id={`${id}-format`}
              value={example.id}
              onChange={(event) => setSelectedId(event.target.value)}
              className="w-full rounded-md border bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              {examples.map((item) => <option key={item.id} value={item.id}>{t(`modelApi.formats.${item.id}`)}</option>)}
            </select>
          </div>
        ) : (
          <p className="text-xs font-medium">{t(`modelApi.formats.${example.id}`)}</p>
        )}
        <div className="space-y-1.5">
          <h4 className="text-xs font-semibold">{t('modelApi.endpoint')}</h4>
          <div className="flex min-w-0 items-start gap-2 rounded-lg border bg-muted/30 p-3">
            <span className="shrink-0 font-mono text-xs font-semibold">{example.method}</span>
            <code aria-label={t('modelApi.endpoint')} className="min-w-0 flex-1 break-all text-xs">{example.endpoint}</code>
            <CopyButton key={example.endpoint} text={example.endpoint} label={t('modelApi.copyEndpoint')} className="h-6 w-6 shrink-0 p-0" />
          </div>
        </div>
        {example.note && <p className="text-xs leading-relaxed text-muted-foreground">{t(`modelApi.notes.${example.note}`)}</p>}
        <ExampleCodeBlock title={t('modelApi.request')} text={example.request} copyLabel={t('modelApi.copyRequest')} />
        <ExampleCodeBlock title={t('modelApi.response')} text={example.response} copyLabel={t('modelApi.copyResponse')} />
        <p className="text-xs leading-relaxed text-muted-foreground">{t('modelApi.illustrative')}</p>
      </div>
    </section>
  );
}
