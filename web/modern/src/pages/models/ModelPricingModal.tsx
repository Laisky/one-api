import { Badge } from '@/components/ui/badge';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Separator } from '@/components/ui/separator';
import { useResponsive } from '@/hooks/useResponsive';
import { X } from 'lucide-react';
import { useCallback, useEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';

// ---- Types matching the backend ModelDisplayInfo ----

export interface ModelDisplayData {
  input_price: number;
  cached_input_price?: number;
  cache_write_5m_price?: number;
  cache_write_1h_price?: number;
  output_price: number;
  max_tokens?: number;
  image_price?: number;
  tiers?: TierData[];
  video_pricing?: VideoPricingData;
  audio_pricing?: AudioPricingData;
  image_pricing?: ImagePricingData;
  embedding_pricing?: EmbeddingPricingData;
}

interface TierData {
  input_price: number;
  output_price: number;
  cached_input_price?: number;
  cache_write_5m_price?: number;
  cache_write_1h_price?: number;
  input_token_threshold: number;
}

interface VideoPricingData {
  per_second_usd: number;
  base_resolution?: string;
  resolution_multipliers?: Record<string, number>;
}

interface AudioPricingData {
  prompt_token_ratio?: number;
  completion_token_ratio?: number;
  prompt_tokens_per_second?: number;
  completion_tokens_per_second?: number;
  usd_per_second?: number;
}

interface ImagePricingData {
  price_per_image_usd?: number;
  default_size?: string;
  default_quality?: string;
  min_images?: number;
  max_images?: number;
  size_multipliers?: Record<string, number>;
  quality_multipliers?: Record<string, number>;
  quality_size_multipliers?: Record<string, Record<string, number>>;
}

interface EmbeddingPricingData {
  text_token_price?: number;
  image_token_price?: number;
  audio_token_price?: number;
  video_token_price?: number;
  document_token_price?: number;
  usd_per_image?: number;
  usd_per_audio_second?: number;
  usd_per_video_frame?: number;
  usd_per_document_page?: number;
}

// ---- Props ----

interface ModelPricingModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  modelName: string;
  data: ModelDisplayData;
  channelName: string;
}

// ---- Component ----

export function ModelPricingModal({ open, onOpenChange, modelName, data, channelName }: ModelPricingModalProps) {
  const { isMobile } = useResponsive();
  const { t } = useTranslation();
  const tr = useCallback(
    (key: string, defaultValue: string, options?: Record<string, unknown>) =>
      t(`models.detail.${key}`, { defaultValue, ...options }),
    [t]
  );

  const content = <PricingContent modelName={modelName} data={data} channelName={channelName} tr={tr} />;

  if (isMobile) {
    return (
      <MobileBottomSheet open={open} onClose={() => onOpenChange(false)} title={modelName} subtitle={channelName}>
        {content}
      </MobileBottomSheet>
    );
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl max-h-[85vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="font-mono text-base">{modelName}</DialogTitle>
          <DialogDescription>{channelName}</DialogDescription>
        </DialogHeader>
        {content}
      </DialogContent>
    </Dialog>
  );
}

// ---- Mobile bottom sheet ----

function MobileBottomSheet({
  open,
  onClose,
  title,
  subtitle,
  children,
}: {
  open: boolean;
  onClose: () => void;
  title: string;
  subtitle?: string;
  children: React.ReactNode;
}) {
  const sheetRef = useRef<HTMLDivElement>(null);
  const [dragOffset, setDragOffset] = useState(0);
  const dragStartY = useRef<number | null>(null);
  const isDragging = useRef(false);

  useEffect(() => {
    if (open) {
      setDragOffset(0);
      const prev = document.body.style.overflow;
      document.body.style.overflow = 'hidden';
      return () => { document.body.style.overflow = prev; };
    }
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const handler = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose(); };
    document.addEventListener('keydown', handler);
    return () => document.removeEventListener('keydown', handler);
  }, [open, onClose]);

  // Swipe-down-to-close handlers
  const handleDragStart = useCallback((clientY: number) => {
    dragStartY.current = clientY;
    isDragging.current = true;
  }, []);

  const handleDragMove = useCallback((clientY: number) => {
    if (dragStartY.current === null) return;
    const delta = clientY - dragStartY.current;
    // Only allow downward drag
    setDragOffset(Math.max(0, delta));
  }, []);

  const handleDragEnd = useCallback(() => {
    if (!isDragging.current) return;
    isDragging.current = false;
    dragStartY.current = null;
    // Close if dragged down more than 120px
    if (dragOffset > 120) {
      onClose();
    } else {
      setDragOffset(0);
    }
  }, [dragOffset, onClose]);

  const onTouchStart = useCallback((e: React.TouchEvent) => {
    handleDragStart(e.touches[0].clientY);
  }, [handleDragStart]);

  const onTouchMove = useCallback((e: React.TouchEvent) => {
    handleDragMove(e.touches[0].clientY);
  }, [handleDragMove]);

  const onTouchEnd = useCallback(() => {
    handleDragEnd();
  }, [handleDragEnd]);

  const onMouseDown = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    handleDragStart(e.clientY);

    const onMouseMove = (ev: MouseEvent) => handleDragMove(ev.clientY);
    const onMouseUp = () => {
      handleDragEnd();
      document.removeEventListener('mousemove', onMouseMove);
      document.removeEventListener('mouseup', onMouseUp);
    };
    document.addEventListener('mousemove', onMouseMove);
    document.addEventListener('mouseup', onMouseUp);
  }, [handleDragStart, handleDragMove, handleDragEnd]);

  if (!open) return null;

  return createPortal(
    <>
      <div
        className="fixed inset-0 z-50 bg-black/50"
        style={{ opacity: dragOffset > 0 ? Math.max(0.2, 1 - dragOffset / 300) : undefined }}
        onClick={onClose}
        aria-hidden="true"
      />
      <div
        ref={sheetRef}
        className="fixed inset-x-0 bottom-0 z-50 flex flex-col bg-background rounded-t-2xl shadow-2xl animate-in slide-in-from-bottom duration-300"
        style={{
          maxHeight: '92vh',
          transform: dragOffset > 0 ? `translateY(${dragOffset}px)` : undefined,
          transition: isDragging.current ? 'none' : 'transform 0.3s ease-out',
        }}
        role="dialog"
        aria-modal="true"
        aria-labelledby="mobile-sheet-title"
      >
        {/* Draggable header area — swipe down to close */}
        <div
          className="shrink-0 cursor-grab active:cursor-grabbing touch-none select-none"
          onTouchStart={onTouchStart}
          onTouchMove={onTouchMove}
          onTouchEnd={onTouchEnd}
          onMouseDown={onMouseDown}
        >
          {/* Drag indicator */}
          <div className="flex justify-center pt-2 pb-1">
            <div className="h-1 w-10 rounded-full bg-muted-foreground/30" />
          </div>
          {/* Header */}
          <div className="flex items-start justify-between px-4 pb-3 border-b">
            <div className="min-w-0 flex-1 pr-3">
              <h2 id="mobile-sheet-title" className="font-mono text-sm font-semibold truncate">{title}</h2>
              {subtitle && <p className="text-xs text-muted-foreground truncate">{subtitle}</p>}
            </div>
            <button
              onClick={onClose}
              className="shrink-0 rounded-full p-1.5 hover:bg-muted transition-colors"
              aria-label="Close"
            >
              <X className="h-4 w-4" />
            </button>
          </div>
        </div>
        {/* Scrollable content */}
        <div className="flex-1 overflow-y-auto overscroll-contain px-4 py-4">
          {children}
        </div>
      </div>
    </>,
    document.body
  );
}

// ---- Internal components ----

type TrFn = (key: string, defaultValue: string, options?: Record<string, unknown>) => string;

function PricingContent({
  modelName: _modelName,
  data,
  channelName: _channelName,
  tr,
}: {
  modelName: string;
  data: ModelDisplayData;
  channelName: string;
  tr: TrFn;
}) {
  const hasCache = (data.cached_input_price !== undefined && data.cached_input_price !== data.input_price) ||
    (data.cache_write_5m_price !== undefined && data.cache_write_5m_price > 0) ||
    (data.cache_write_1h_price !== undefined && data.cache_write_1h_price > 0);

  return (
    <div className="space-y-5">
      {/* Base text token pricing */}
      <PricingSection title={tr('text_tokens', 'Text Token Pricing')} icon="text">
        <PriceGrid>
          <PriceCell label={tr('input', 'Input')} sublabel={tr('per_1m', 'per 1M tokens')} value={data.input_price} tr={tr} />
          <PriceCell label={tr('output', 'Output')} sublabel={tr('per_1m', 'per 1M tokens')} value={data.output_price} tr={tr} />
        </PriceGrid>
      </PricingSection>

      {/* Cache pricing */}
      {hasCache && (
        <PricingSection title={tr('cache_pricing', 'Cache Pricing')} icon="cache">
          <PriceGrid>
            {data.cached_input_price !== undefined && data.cached_input_price !== data.input_price && (
              <PriceCell label={tr('cached_read', 'Cache Read')} sublabel={tr('per_1m', 'per 1M tokens')} value={data.cached_input_price} tr={tr} />
            )}
            {data.cache_write_5m_price !== undefined && data.cache_write_5m_price > 0 && (
              <PriceCell label={tr('cache_write_5m', '5-min Cache Write')} sublabel={tr('per_1m', 'per 1M tokens')} value={data.cache_write_5m_price} tr={tr} />
            )}
            {data.cache_write_1h_price !== undefined && data.cache_write_1h_price > 0 && (
              <PriceCell label={tr('cache_write_1h', '1-hour Cache Write')} sublabel={tr('per_1m', 'per 1M tokens')} value={data.cache_write_1h_price} tr={tr} />
            )}
          </PriceGrid>
        </PricingSection>
      )}

      {/* Tiered pricing */}
      {data.tiers && data.tiers.length > 0 && (
        <PricingSection title={tr('tiered_pricing', 'Tiered Pricing')} icon="tiers">
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b text-muted-foreground">
                  <th className="text-left py-2 pr-3 font-medium">{tr('tier_threshold', 'Threshold')}</th>
                  <th className="text-left py-2 px-3 font-medium">{tr('input', 'Input')}</th>
                  <th className="text-left py-2 px-3 font-medium">{tr('output', 'Output')}</th>
                  {data.tiers.some(t => t.cached_input_price && t.cached_input_price > 0) && (
                    <th className="text-left py-2 pl-3 font-medium">{tr('cached_read', 'Cache Read')}</th>
                  )}
                </tr>
              </thead>
              <tbody>
                {/* Base tier */}
                <tr className="border-b border-dashed">
                  <td className="py-2 pr-3">
                    <Badge variant="secondary" className="text-xs">{tr('base_tier', 'Base')}</Badge>
                  </td>
                  <td className="py-2 px-3 font-mono text-sm">{formatUsd(data.input_price)}</td>
                  <td className="py-2 px-3 font-mono text-sm">{formatUsd(data.output_price)}</td>
                  {data.tiers.some(t => t.cached_input_price && t.cached_input_price > 0) && (
                    <td className="py-2 pl-3 font-mono text-sm">{formatUsd(data.cached_input_price ?? data.input_price)}</td>
                  )}
                </tr>
                {data.tiers.map((tier, i) => (
                  <tr key={i} className="border-b border-dashed last:border-0">
                    <td className="py-2 pr-3">
                      <Badge variant="outline" className="text-xs">&ge; {formatTokenCount(tier.input_token_threshold)}</Badge>
                    </td>
                    <td className="py-2 px-3 font-mono text-sm">{formatUsd(tier.input_price)}</td>
                    <td className="py-2 px-3 font-mono text-sm">{formatUsd(tier.output_price)}</td>
                    {data.tiers!.some(t => t.cached_input_price && t.cached_input_price > 0) && (
                      <td className="py-2 pl-3 font-mono text-sm">{tier.cached_input_price ? formatUsd(tier.cached_input_price) : '-'}</td>
                    )}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </PricingSection>
      )}

      {/* Image pricing */}
      {data.image_pricing && (
        <PricingSection title={tr('image_pricing', 'Image Pricing')} icon="image">
          {data.image_pricing.price_per_image_usd !== undefined && data.image_pricing.price_per_image_usd > 0 && (
            <div className="mb-3">
              <PriceGrid>
                <PriceCell label={tr('base_price', 'Base Price')} sublabel={tr('per_image', 'per image')} value={data.image_pricing.price_per_image_usd} tr={tr} raw />
              </PriceGrid>
            </div>
          )}
          <div className="flex flex-wrap gap-2 mb-3">
            {data.image_pricing.default_size && (
              <Badge variant="secondary" className="text-xs">
                {tr('default_size', 'Default')}: {data.image_pricing.default_size}
              </Badge>
            )}
            {data.image_pricing.default_quality && (
              <Badge variant="secondary" className="text-xs">
                {tr('default_quality', 'Quality')}: {data.image_pricing.default_quality}
              </Badge>
            )}
            {data.image_pricing.min_images !== undefined && data.image_pricing.min_images > 0 && (
              <Badge variant="outline" className="text-xs">
                {tr('min_images', 'Min')}: {data.image_pricing.min_images}
              </Badge>
            )}
            {data.image_pricing.max_images !== undefined && data.image_pricing.max_images > 0 && (
              <Badge variant="outline" className="text-xs">
                {tr('max_images', 'Max')}: {data.image_pricing.max_images}
              </Badge>
            )}
          </div>

          {/* Quality x Size multiplier matrix */}
          {data.image_pricing.quality_size_multipliers && Object.keys(data.image_pricing.quality_size_multipliers).length > 0 && (
            <MultiplierMatrix
              data={data.image_pricing.quality_size_multipliers}
              basePrice={data.image_pricing.price_per_image_usd}
              rowLabel={tr('quality', 'Quality')}
              colLabel={tr('size', 'Size')}
            />
          )}

          {/* Simple size multipliers (when no quality matrix) */}
          {!data.image_pricing.quality_size_multipliers && data.image_pricing.size_multipliers && Object.keys(data.image_pricing.size_multipliers).length > 0 && (
            <SimpleMultiplierTable
              data={data.image_pricing.size_multipliers}
              basePrice={data.image_pricing.price_per_image_usd}
              label={tr('size', 'Size')}
            />
          )}

          {/* Simple quality multipliers */}
          {!data.image_pricing.quality_size_multipliers && data.image_pricing.quality_multipliers && Object.keys(data.image_pricing.quality_multipliers).length > 0 && (
            <SimpleMultiplierTable
              data={data.image_pricing.quality_multipliers}
              basePrice={data.image_pricing.price_per_image_usd}
              label={tr('quality', 'Quality')}
            />
          )}
        </PricingSection>
      )}

      {/* Video pricing */}
      {data.video_pricing && (
        <PricingSection title={tr('video_pricing', 'Video Pricing')} icon="video">
          <PriceGrid>
            <PriceCell label={tr('base_rate', 'Base Rate')} sublabel={tr('per_second', 'per second')} value={data.video_pricing.per_second_usd} tr={tr} raw />
          </PriceGrid>
          {data.video_pricing.base_resolution && (
            <div className="mt-2">
              <Badge variant="secondary" className="text-xs">
                {tr('base_resolution', 'Base Resolution')}: {data.video_pricing.base_resolution}
              </Badge>
            </div>
          )}
          {data.video_pricing.resolution_multipliers && Object.keys(data.video_pricing.resolution_multipliers).length > 0 && (
            <div className="mt-3">
              <SimpleMultiplierTable
                data={data.video_pricing.resolution_multipliers}
                basePrice={data.video_pricing.per_second_usd}
                label={tr('resolution', 'Resolution')}
                unit={tr('per_second', 'per second')}
              />
            </div>
          )}
        </PricingSection>
      )}

      {/* Audio pricing */}
      {data.audio_pricing && (
        <PricingSection title={tr('audio_pricing', 'Audio Pricing')} icon="audio">
          <PriceGrid>
            {data.audio_pricing.usd_per_second !== undefined && data.audio_pricing.usd_per_second > 0 && (
              <PriceCell label={tr('base_rate', 'Base Rate')} sublabel={tr('per_second', 'per second')} value={data.audio_pricing.usd_per_second} tr={tr} raw />
            )}
            {data.audio_pricing.prompt_token_ratio !== undefined && data.audio_pricing.prompt_token_ratio > 0 && (
              <div className="rounded-lg border bg-muted/30 p-3">
                <div className="text-[11px] font-semibold uppercase tracking-widest text-muted-foreground">{tr('prompt_ratio', 'Prompt Ratio')}</div>
                <div className="mt-1 text-lg font-semibold tabular-nums">{fmtNum(data.audio_pricing.prompt_token_ratio)}x</div>
              </div>
            )}
            {data.audio_pricing.completion_token_ratio !== undefined && data.audio_pricing.completion_token_ratio > 0 && (
              <div className="rounded-lg border bg-muted/30 p-3">
                <div className="text-[11px] font-semibold uppercase tracking-widest text-muted-foreground">{tr('completion_ratio', 'Completion Ratio')}</div>
                <div className="mt-1 text-lg font-semibold tabular-nums">{fmtNum(data.audio_pricing.completion_token_ratio)}x</div>
              </div>
            )}
          </PriceGrid>
          {(data.audio_pricing.prompt_tokens_per_second || data.audio_pricing.completion_tokens_per_second) && (
            <div className="mt-2 flex flex-wrap gap-2">
              {data.audio_pricing.prompt_tokens_per_second !== undefined && data.audio_pricing.prompt_tokens_per_second > 0 && (
                <Badge variant="outline" className="text-xs">
                  {tr('prompt_tps', 'Prompt')}: {fmtNum(data.audio_pricing.prompt_tokens_per_second)} tok/s
                </Badge>
              )}
              {data.audio_pricing.completion_tokens_per_second !== undefined && data.audio_pricing.completion_tokens_per_second > 0 && (
                <Badge variant="outline" className="text-xs">
                  {tr('completion_tps', 'Completion')}: {fmtNum(data.audio_pricing.completion_tokens_per_second)} tok/s
                </Badge>
              )}
            </div>
          )}
        </PricingSection>
      )}

      {/* Embedding pricing */}
      {data.embedding_pricing && (
        <PricingSection title={tr('embedding_pricing', 'Embedding Pricing')} icon="embedding">
          <PriceGrid>
            {data.embedding_pricing.text_token_price !== undefined && data.embedding_pricing.text_token_price > 0 && (
              <PriceCell label={tr('text', 'Text')} sublabel={tr('per_1m', 'per 1M tokens')} value={data.embedding_pricing.text_token_price} tr={tr} />
            )}
            {data.embedding_pricing.image_token_price !== undefined && data.embedding_pricing.image_token_price > 0 && (
              <PriceCell label={tr('image', 'Image')} sublabel={tr('per_1m', 'per 1M tokens')} value={data.embedding_pricing.image_token_price} tr={tr} />
            )}
            {data.embedding_pricing.audio_token_price !== undefined && data.embedding_pricing.audio_token_price > 0 && (
              <PriceCell label={tr('audio', 'Audio')} sublabel={tr('per_1m', 'per 1M tokens')} value={data.embedding_pricing.audio_token_price} tr={tr} />
            )}
            {data.embedding_pricing.video_token_price !== undefined && data.embedding_pricing.video_token_price > 0 && (
              <PriceCell label={tr('video', 'Video')} sublabel={tr('per_1m', 'per 1M tokens')} value={data.embedding_pricing.video_token_price} tr={tr} />
            )}
            {data.embedding_pricing.document_token_price !== undefined && data.embedding_pricing.document_token_price > 0 && (
              <PriceCell label={tr('document', 'Document')} sublabel={tr('per_1m', 'per 1M tokens')} value={data.embedding_pricing.document_token_price} tr={tr} />
            )}
          </PriceGrid>
          {/* Direct per-unit pricing */}
          {(data.embedding_pricing.usd_per_image || data.embedding_pricing.usd_per_audio_second ||
            data.embedding_pricing.usd_per_video_frame || data.embedding_pricing.usd_per_document_page) && (
            <>
              <Separator className="my-3" />
              <PriceGrid>
                {data.embedding_pricing.usd_per_image !== undefined && data.embedding_pricing.usd_per_image > 0 && (
                  <PriceCell label={tr('per_image_unit', 'Per Image')} value={data.embedding_pricing.usd_per_image} tr={tr} raw />
                )}
                {data.embedding_pricing.usd_per_audio_second !== undefined && data.embedding_pricing.usd_per_audio_second > 0 && (
                  <PriceCell label={tr('per_audio_sec', 'Per Audio Sec')} value={data.embedding_pricing.usd_per_audio_second} tr={tr} raw />
                )}
                {data.embedding_pricing.usd_per_video_frame !== undefined && data.embedding_pricing.usd_per_video_frame > 0 && (
                  <PriceCell label={tr('per_video_frame', 'Per Video Frame')} value={data.embedding_pricing.usd_per_video_frame} tr={tr} raw />
                )}
                {data.embedding_pricing.usd_per_document_page !== undefined && data.embedding_pricing.usd_per_document_page > 0 && (
                  <PriceCell label={tr('per_doc_page', 'Per Doc Page')} value={data.embedding_pricing.usd_per_document_page} tr={tr} raw />
                )}
              </PriceGrid>
            </>
          )}
        </PricingSection>
      )}
    </div>
  );
}

// ---- Reusable sub-components ----

const sectionIcons: Record<string, string> = {
  text: '\u{1F4DD}',
  cache: '\u{1F4BE}',
  tiers: '\u{1F4CA}',
  image: '\u{1F5BC}',
  video: '\u{1F3AC}',
  audio: '\u{1F3B5}',
  embedding: '\u{1F9E9}',
};

function PricingSection({ title, icon, children }: { title: string; icon: string; children: React.ReactNode }) {
  return (
    <div>
      <div className="flex items-center gap-2 mb-3">
        <span className="text-base" role="img">{sectionIcons[icon] || ''}</span>
        <h3 className="text-sm font-semibold tracking-wide text-foreground">{title}</h3>
      </div>
      <div className="rounded-xl border bg-card p-4">{children}</div>
    </div>
  );
}

function PriceGrid({ children }: { children: React.ReactNode }) {
  return <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">{children}</div>;
}

function PriceCell({
  label,
  sublabel,
  value,
  tr,
  raw,
}: {
  label: string;
  sublabel?: string;
  value: number;
  tr: TrFn;
  raw?: boolean;
}) {
  return (
    <div className="rounded-lg border bg-muted/30 p-3">
      <div className="text-[11px] font-semibold uppercase tracking-widest text-muted-foreground">{label}</div>
      {sublabel && <div className="text-[10px] text-muted-foreground/70">{sublabel}</div>}
      <div className="mt-1 text-lg font-semibold tabular-nums">
        {raw ? formatUsdRaw(value) : formatUsdForTokens(value, tr)}
      </div>
    </div>
  );
}

function MultiplierMatrix({
  data,
  basePrice,
  rowLabel,
  colLabel,
}: {
  data: Record<string, Record<string, number>>;
  basePrice?: number;
  rowLabel: string;
  colLabel: string;
}) {
  const qualities = Object.keys(data);
  const allSizes = new Set<string>();
  qualities.forEach((q) => Object.keys(data[q]).forEach((s) => allSizes.add(s)));
  const sizes = Array.from(allSizes);

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b text-muted-foreground">
            <th className="text-left py-2 pr-3 font-medium">{rowLabel} \ {colLabel}</th>
            {sizes.map((s) => (
              <th key={s} className="text-center py-2 px-2 font-medium font-mono text-xs">{s}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {qualities.map((q) => (
            <tr key={q} className="border-b border-dashed last:border-0">
              <td className="py-2 pr-3 capitalize font-medium">{q}</td>
              {sizes.map((s) => {
                const multiplier = data[q]?.[s];
                const price = basePrice && multiplier ? basePrice * multiplier : undefined;
                return (
                  <td key={s} className="text-center py-2 px-2 font-mono text-sm">
                    {multiplier !== undefined ? (
                      <div>
                        <div>{price !== undefined ? formatUsdRaw(price) : `${fmtNum(multiplier)}x`}</div>
                        {multiplier !== 1 && (
                          <div className="text-[10px] text-muted-foreground">{fmtNum(multiplier)}x</div>
                        )}
                      </div>
                    ) : (
                      <span className="text-muted-foreground">-</span>
                    )}
                  </td>
                );
              })}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function SimpleMultiplierTable({
  data,
  basePrice,
  label,
  unit: _unit,
}: {
  data: Record<string, number>;
  basePrice?: number;
  label: string;
  unit?: string;
}) {
  const entries = Object.entries(data).sort(([, a], [, b]) => a - b);
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b text-muted-foreground">
            <th className="text-left py-1.5 pr-3 font-medium">{label}</th>
            <th className="text-right py-1.5 px-3 font-medium">Multiplier</th>
            {basePrice !== undefined && basePrice > 0 && (
              <th className="text-right py-1.5 pl-3 font-medium">Price</th>
            )}
          </tr>
        </thead>
        <tbody>
          {entries.map(([key, multiplier]) => (
            <tr key={key} className="border-b border-dashed last:border-0">
              <td className="py-1.5 pr-3 font-mono text-xs">{key}</td>
              <td className="text-right py-1.5 px-3 font-mono text-sm">{fmtNum(multiplier)}x</td>
              {basePrice !== undefined && basePrice > 0 && (
                <td className="text-right py-1.5 pl-3 font-mono text-sm">{formatUsdRaw(basePrice * multiplier)}</td>
              )}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// ---- Formatting helpers ----

/** Round any number to at most 4 decimal places, stripping trailing zeros. */
function fmtNum(n: number): string {
  return parseFloat(n.toFixed(4)).toString();
}

function formatUsd(price: number): string {
  if (price === 0) return '$0';
  if (price < 0.001) return `$${parseFloat(price.toFixed(6))}`;
  if (price < 1) return `$${parseFloat(price.toFixed(4))}`;
  return `$${price.toFixed(2)}`;
}

function formatUsdRaw(price: number): string {
  if (price === 0) return '$0';
  if (price < 0.0001) return `$${parseFloat(price.toFixed(6))}`;
  if (price < 0.01) return `$${parseFloat(price.toFixed(4))}`;
  return `$${price.toFixed(2)}`;
}

function formatUsdForTokens(price: number, tr: TrFn): string {
  if (price === 0) return tr('free', 'Free');
  return formatUsd(price);
}

function formatTokenCount(count: number): string {
  if (count >= 1_000_000) return `${(count / 1_000_000).toFixed(1)}M`;
  if (count >= 1_000) return `${(count / 1_000).toFixed(0)}K`;
  return count.toString();
}
