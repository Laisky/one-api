import { useId, useRef, useState, type ReactNode } from 'react';
import { ChevronDown, MoreHorizontal, ArrowUp, ArrowDown, ArrowUpRight } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import './mobile-table.css';

/** MobileColumnMeta lets a column opt into a mobile role without depending on translated column IDs. */
export interface MobileColumnMeta {
  mobileLabel?: string;
  mobileRole?: 'primary' | 'summary' | 'detail' | 'actions';
}

/** MobileColumnDescription is the structural subset shared by accessor and display column definitions. */
interface MobileColumnDescription {
  header?: unknown;
  accessorKey?: unknown;
  accessorFn?: unknown;
  meta?: unknown;
}

/** MobileField preserves one original cell renderer and its accessible, localized label. */
export interface MobileField {
  id: string;
  label: string;
  content: ReactNode;
  role?: MobileColumnMeta['mobileRole'] | 'selection';
}

/** MobileRecord describes a record using presentation data, never a second copy of its business logic. */
export interface MobileRecord {
  id: string;
  fields: MobileField[];
  selected: boolean;
  onOpen?: () => void;
  extraActions?: ReactNode;
}

/** getMobileColumnLabel preserves an explicit label, then a string header, before humanizing an identifier. */
export function getMobileColumnLabel(column: MobileColumnDescription, id: string): string {
  const meta = column.meta as MobileColumnMeta | undefined;
  return meta?.mobileLabel ?? (typeof column.header === 'string' ? column.header : id.replace(/[_.]+/g, ' '));
}

/** getMobileField describes a cell and puts a trailing display column in its own labeled disclosure. */
export function getMobileField(
  column: MobileColumnDescription,
  id: string,
  content: ReactNode,
  trailing: boolean
): MobileField {
  const meta = column.meta as MobileColumnMeta | undefined;
  // A trailing display column is the existing tables' action slot. Its own
  // label is retained; computed data columns can explicitly use summary/detail.
  const trailingDisplay = trailing && !column.accessorKey && !column.accessorFn;
  const role = id === '__selection__' ? 'selection' : (meta?.mobileRole ?? (id === 'actions' || trailingDisplay ? 'actions' : undefined));
  return { id, label: getMobileColumnLabel(column, id), content, role };
}

const INTERACTIVE_TARGETS =
  'button, a[href], input, select, textarea, label, summary, [role="button"], [role="link"], [role="checkbox"], [data-mobile-controls]';

/** MobileFields renders labeled values without duplicating their original interactive children. */
function MobileFields({ fields }: { fields: MobileField[] }) {
  return (
    <dl className="mobile-record__fields">
      {fields.map((field) => (
        <div key={field.id} className="mobile-record__field">
          <dt>{field.label}</dt>
          <dd>{field.content}</dd>
        </div>
      ))}
    </dl>
  );
}

/** MobileRecordCard keeps a readable summary visible and exposes details and existing actions on demand. */
function MobileRecordCard({ record, loading }: { record: MobileRecord; loading: boolean }) {
  const { t } = useTranslation();
  const titleId = useId();
  const detailsId = useId();
  const actionsId = useId();
  const [detailsOpen, setDetailsOpen] = useState(false);
  const [actionsOpen, setActionsOpen] = useState(false);
  const actionsTrigger = useRef<HTMLButtonElement>(null);
  const selection = record.fields.find((field) => field.role === 'selection');
  const actions = record.fields.filter((field) => field.role === 'actions');
  const dataFields = record.fields.filter((field) => field.role !== 'selection' && field.role !== 'actions');
  const primary = dataFields.find((field) => field.role === 'primary') ?? dataFields[0];
  const remaining = dataFields.filter((field) => field !== primary);
  const explicitSummary = remaining.filter((field) => field.role === 'summary');
  const summary = explicitSummary.length ? explicitSummary : remaining.filter((field) => field.role !== 'detail').slice(0, 4);
  const details = remaining.filter((field) => !summary.includes(field));
  const actionLabel = actions[0]?.label || t('mobile_table.actions', 'Actions');
  const hasActions = actions.length > 0 || !!record.extraActions;

  return (
    <article
      className={cn('mobile-record', record.selected && 'mobile-record--selected')}
      aria-labelledby={primary ? titleId : undefined}
      data-mobile-record=""
      onClick={(event) => {
        if (loading || !(event.target instanceof Element) || event.target.closest(INTERACTIVE_TARGETS)) return;
        if (window.getSelection()?.toString()) return;
        record.onOpen?.();
      }}
    >
      <div className="mobile-record__header">
        {selection && <label className="mobile-record__selection">{selection.content}</label>}
        {primary && (
          <dl className="mobile-record__identity">
            <dt className="sr-only">{primary.label}</dt>
            <dd id={titleId}>{primary.content}</dd>
          </dl>
        )}
        {hasActions && (
          <Button
            ref={actionsTrigger}
            type="button"
            variant="ghost"
            size="icon"
            className="mobile-record__action-trigger"
            aria-label={actionLabel}
            aria-describedby={primary ? titleId : undefined}
            aria-expanded={actionsOpen}
            aria-controls={actionsId}
            disabled={loading}
            onClick={() => setActionsOpen((open) => !open)}
          >
            <MoreHorizontal className="h-5 w-5" aria-hidden="true" />
          </Button>
        )}
      </div>
      {summary.length > 0 && <MobileFields fields={summary} />}
      {hasActions && (
        <div
          id={actionsId}
          className="mobile-record__actions"
          hidden={!actionsOpen}
          role="group"
          aria-label={actionLabel}
          data-mobile-controls=""
          onKeyDown={(event) => {
            if (event.key !== 'Escape' || event.defaultPrevented) return;
            event.stopPropagation();
            setActionsOpen(false);
            actionsTrigger.current?.focus();
          }}
        >
          {actions.map((field) => <div key={field.id} className="mobile-record__action-content">{field.content}</div>)}
          {actions.length === 0 && record.extraActions}
        </div>
      )}
      {details.length > 0 && (
        <div id={detailsId} className="mobile-record__details" hidden={!detailsOpen}>
          <MobileFields fields={details} />
        </div>
      )}
      {(details.length > 0 || record.onOpen) && (
        <div className="mobile-record__footer" data-mobile-controls="">
          {details.length > 0 && (
            <Button
              type="button"
              variant="ghost"
              className="mobile-record__detail-trigger"
              aria-expanded={detailsOpen}
              aria-controls={detailsId}
              disabled={loading}
              onClick={() => setDetailsOpen((open) => !open)}
            >
              {detailsOpen ? t('mobile_table.less', 'Less') : t('mobile_table.details', 'Details')}
              <ChevronDown className={cn('h-4 w-4', detailsOpen && 'rotate-180')} aria-hidden="true" />
            </Button>
          )}
          {record.onOpen && (
            <Button type="button" variant="ghost" className="mobile-record__open" onClick={record.onOpen} disabled={loading}>
              {t('mobile_table.open_record', 'Open record')}
              <ArrowUpRight className="h-4 w-4" aria-hidden="true" />
            </Button>
          )}
        </div>
      )}
    </article>
  );
}

/** MobileTable renders one semantic list; stable record keys keep disclosures attached to their records. */
export function MobileTable({ records, loading = false, emptyMessage }: { records: MobileRecord[]; loading?: boolean; emptyMessage: ReactNode }) {
  return records.length ? (
    <ul className="mobile-data-list" inert={loading}>
      {records.map((record) => <li key={record.id}><MobileRecordCard record={record} loading={loading} /></li>)}
    </ul>
  ) : (
    <div className="mobile-table-empty">{emptyMessage}</div>
  );
}

/** MobileSortOption describes one supported server-side sort field. */
export interface MobileSortOption {
  value: string;
  label: string;
}

/** MobileTableSort restores sorting when desktop column headers are replaced by mobile records. */
export function MobileTableSort({
  options,
  sortBy,
  sortOrder,
  onSortChange,
  loading = false,
  defaultOrder = 'asc',
}: {
  options: MobileSortOption[];
  sortBy: string;
  sortOrder: 'asc' | 'desc';
  onSortChange?: (key: string, order: 'asc' | 'desc') => void;
  loading?: boolean;
  defaultOrder?: 'asc' | 'desc';
}) {
  const { t } = useTranslation();
  const id = useId();
  if (!onSortChange || !options.length) return null;
  const selected = options.some((option) => option.value === sortBy) ? sortBy : '';
  return (
    <div className="mobile-table-sort">
      <label htmlFor={id}>{t('mobile_table.sort_by', 'Sort by')}</label>
      <select
        id={id}
        value={selected}
        disabled={loading}
        onChange={(event) => onSortChange(event.target.value, event.target.value === sortBy ? sortOrder : defaultOrder)}
      >
        <option value="">{t('mobile_table.default_order', 'Default order')}</option>
        {options.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
      </select>
      <Button
        type="button"
        variant="outline"
        size="icon"
        disabled={loading || !selected}
        aria-label={sortOrder === 'asc' ? t('mobile_table.sort_descending', 'Sort descending') : t('mobile_table.sort_ascending', 'Sort ascending')}
        onClick={() => onSortChange(selected, sortOrder === 'asc' ? 'desc' : 'asc')}
      >
        {sortOrder === 'asc' ? <ArrowUp className="h-4 w-4" aria-hidden="true" /> : <ArrowDown className="h-4 w-4" aria-hidden="true" />}
      </Button>
    </div>
  );
}
