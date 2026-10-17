import { CopyButton } from '@/components/ui/copy-button';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { useResponsive } from '@/hooks/useResponsive';
import { X } from 'lucide-react';
import { useCallback, useEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';
import type { AudioInputTariff } from './AudioTariffDetails';
import { ModelApiExamples } from './ModelApiExamples';
import { PricingContent } from './ModelPricingContent';
import type { PerPagePricingData } from './PerPagePricing';

// ---- Types matching the backend ModelDisplayInfo ----

export interface ModelDisplayData {
  input_price: number;
  cached_input_price?: number;
  cache_write_5m_price?: number;
  cache_write_1h_price?: number;
  output_price: number;
  max_tokens?: number;
  context_length?: number;
  max_output_tokens?: number;
  input_modalities?: string[];
  output_modalities?: string[];
  supported_features?: string[];
  supported_sampling_parameters?: string[];
  max_reasoning_tokens?: number;
  supported_reasoning_efforts?: string[];
  default_reasoning_effort?: string;
  quantization?: string;
  hugging_face_id?: string;
  description?: string;
  image_price?: number;
  tiers?: TierData[];
  video_pricing?: VideoPricingData;
  audio_pricing?: AudioPricingData;
  image_pricing?: ImagePricingData;
  embedding_pricing?: EmbeddingPricingData;
  per_call_pricing?: PerCallPricingData;
  per_page_pricing?: PerPagePricingData;
  time_windows?: TimeWindowData[];
  active_time_window?: string;
}

interface TierData {
  input_price: number;
  output_price: number;
  cached_input_price?: number;
  cache_write_5m_price?: number;
  cache_write_1h_price?: number;
  input_token_threshold: number;
  output_token_threshold?: number;
}

interface VideoPricingData {
  input_image_usd?: number;
  per_second_usd: number;
  base_resolution?: string;
  resolution_multipliers?: Record<string, number>;
}

interface AudioPricingData extends AudioInputTariff {
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

interface PerCallPricingData {
  usd_per_thousand_calls?: number;
  usd_per_call?: number;
}

/** TimeWindowData retains the source schedule and its complete pricing overlay. */
export interface TimeWindowData {
  name?: string;
  timezone?: string;
  ranges: { start: string; end: string }[];
  days_of_week?: number[];
  date_from?: string;
  date_to?: string;
  overlay: TimeWindowOverlayData;
}

interface TimeWindowOverlayData {
  input_price?: number;
  cached_input_price?: number;
  cache_write_5m_price?: number;
  cache_write_1h_price?: number;
  output_price?: number;
  tiers?: TierData[];
  video_pricing?: VideoPricingData;
  audio_pricing?: AudioPricingData;
  image_pricing?: ImagePricingData;
  embedding_pricing?: EmbeddingPricingData;
  per_call_pricing?: PerCallPricingData;
  per_page_pricing?: PerPagePricingData;
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

/**
 * ModelPricingModal renders detailed pricing and metadata for a selected model
 * and exposes a copy action for the model name in desktop and mobile headers.
 */
export function ModelPricingModal({ open, onOpenChange, modelName, data, channelName }: ModelPricingModalProps) {
  const { isMobile } = useResponsive();
  const { t, i18n } = useTranslation();
  const tr = useCallback(
    (key: string, defaultValue: string, options?: Record<string, unknown>) => t(`models.detail.${key}`, { defaultValue, ...options }),
    [t]
  );
  const closeLabel = tr('close', 'Close');
  const copyModelLabel = tr('copy_model_name', 'Copy model name');
  const copyModelSuccess = tr('model_name_copied', 'Model name copied');
  const modelTitle = (
    <span className="inline-flex min-w-0 max-w-full items-center gap-2">
      <span className="min-w-0 truncate font-mono">{modelName}</span>
      <CopyButton text={modelName} label={copyModelLabel} successMessage={copyModelSuccess} className="h-7 w-7 shrink-0 p-0" />
    </span>
  );

  const content = (
    <div className="min-w-0 space-y-5">
      <PricingContent modelName={modelName} data={data} channelName={channelName} tr={tr} locale={i18n.language} />
      <ModelApiExamples key={modelName} modelName={modelName} data={data} />
    </div>
  );

  if (isMobile) {
    return (
      <MobileBottomSheet open={open} onClose={() => onOpenChange(false)} title={modelTitle} subtitle={channelName} closeLabel={closeLabel}>
        {content}
      </MobileBottomSheet>
    );
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl max-h-[85vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="text-base">{modelTitle}</DialogTitle>
          <DialogDescription>{channelName}</DialogDescription>
        </DialogHeader>
        {content}
      </DialogContent>
    </Dialog>
  );
}

// ---- Mobile bottom sheet ----

/**
 * MobileBottomSheet renders the mobile pricing sheet with drag-to-close and
 * keyboard-close interactions, returning its content through a portal.
 */
function MobileBottomSheet({
  open,
  onClose,
  title,
  subtitle,
  closeLabel,
  children,
}: {
  open: boolean;
  onClose: () => void;
  title: React.ReactNode;
  subtitle?: string;
  closeLabel: string;
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
      return () => {
        document.body.style.overflow = prev;
      };
    }
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
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

  const onTouchStart = useCallback(
    (e: React.TouchEvent) => {
      handleDragStart(e.touches[0].clientY);
    },
    [handleDragStart]
  );

  const onTouchMove = useCallback(
    (e: React.TouchEvent) => {
      handleDragMove(e.touches[0].clientY);
    },
    [handleDragMove]
  );

  const onTouchEnd = useCallback(() => {
    handleDragEnd();
  }, [handleDragEnd]);

  const onMouseDown = useCallback(
    (e: React.MouseEvent) => {
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
    },
    [handleDragStart, handleDragMove, handleDragEnd]
  );

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
              <h2 id="mobile-sheet-title" className="text-sm font-semibold">
                {title}
              </h2>
              {subtitle && <p className="text-xs text-muted-foreground truncate">{subtitle}</p>}
            </div>
            <button onClick={onClose} className="shrink-0 rounded-full p-1.5 hover:bg-muted transition-colors" aria-label={closeLabel}>
              <X className="h-4 w-4" />
            </button>
          </div>
        </div>
        {/* Scrollable content */}
        <div className="flex-1 overflow-y-auto overscroll-contain px-4 py-4">{children}</div>
      </div>
    </>,
    document.body
  );
}
