import { useTranslation } from 'react-i18next';

/** AudioInputTariff describes exact input units independently of token estimates. */
export interface AudioInputTariff {
  input_unit?: string;
  input_price_usd?: number;
  input_price_quantity?: number;
  minimum_billable_seconds?: number;
  billing_increment_seconds?: number;
}

/** AudioTariffDetails renders direct provider tariffs, including explicit free inputs. */
export function AudioTariffDetails({ pricing }: { pricing?: AudioInputTariff }) {
  const { t } = useTranslation();
  if (!pricing || !['characters', 'utf8_bytes', 'seconds'].includes(pricing.input_unit ?? '')) return null;
  const quantity = pricing.input_price_quantity ?? 1;
  const price = pricing.input_price_usd ?? 0; // Zero is omitted by the backend serializer.
  if (!Number.isFinite(price) || price < 0 || !Number.isFinite(quantity) || quantity <= 0) return null;
  return (
    <div className="rounded-lg border bg-muted/30 p-3" aria-label={t('modelApi.audioTariff.title')}>
      <div className="text-xs font-semibold">{t('modelApi.audioTariff.title')}</div>
      <div className="mt-1 font-mono text-sm">{t('modelApi.audioTariff.rate', { price, quantity, unit: t(`modelApi.audioTariff.${pricing.input_unit}`) })}</div>
      {!!pricing.minimum_billable_seconds && <p className="mt-1 text-xs text-muted-foreground">{t('modelApi.audioTariff.minimum', { seconds: pricing.minimum_billable_seconds })}</p>}
      {!!pricing.billing_increment_seconds && <p className="mt-1 text-xs text-muted-foreground">{t('modelApi.audioTariff.increment', { seconds: pricing.billing_increment_seconds })}</p>}
    </div>
  );
}
