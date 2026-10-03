import type { ReactNode } from 'react';
import { Fragment } from 'react';
import { ChevronDown, Loader2, MoreHorizontal, RotateCcw, Search } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import './table-toolbar.css';

/** TableBatchAction describes a selected-record command without changing its confirmation or execution policy. */
export interface TableBatchAction {
  id: string;
  label: string;
  icon?: ReactNode;
  onSelect: () => void | Promise<void>;
  destructive?: boolean;
  disabled?: boolean;
}

/** TableToolbarProps composes selection, search, view controls, and contextual record actions in one shared layout. */
export interface TableToolbarProps {
  selectionControl?: ReactNode;
  searchControl?: ReactNode;
  onSearchSubmit?: () => void;
  onRefresh?: () => void;
  toolbarActions?: ReactNode;
  batchActions?: TableBatchAction[];
  hasSelection?: boolean;
  batchActionsDisabled?: boolean;
  batchActionsBusy?: boolean;
  loading?: boolean;
}

/** TableToolbar keeps controls in one desktop row and a deliberate two-row layout in narrow containers. */
export function TableToolbar({
  selectionControl,
  searchControl,
  onSearchSubmit,
  onRefresh,
  toolbarActions,
  batchActions = [],
  hasSelection = false,
  batchActionsDisabled = false,
  batchActionsBusy = false,
  loading = false,
}: TableToolbarProps) {
  const { t } = useTranslation();
  const showBatchActions = hasSelection && batchActions.length > 0;
  if (!selectionControl && !searchControl && !onRefresh && !toolbarActions && !showBatchActions) return null;

  return (
    <div className="table-toolbar" role="group" aria-label={t('table_selection.toolbar')}>
      <div className="table-toolbar__layout">
        {selectionControl && <div className="table-toolbar__selection">{selectionControl}</div>}
        {searchControl && (
          <div className="table-toolbar__search">
            <div className="table-toolbar__search-field">{searchControl}</div>
            {onSearchSubmit && (
              <TooltipProvider>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      className="table-toolbar-control"
                      disabled={loading}
                      onClick={onSearchSubmit}
                      aria-label={t('common.search')}
                    >
                      <Search className="h-4 w-4" aria-hidden="true" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>{t('common.search')}</TooltipContent>
                </Tooltip>
              </TooltipProvider>
            )}
          </div>
        )}
        <div className="table-toolbar__actions">
          {onRefresh && (
            <TooltipProvider>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    className="table-toolbar-control"
                    disabled={loading}
                    onClick={onRefresh}
                    aria-label={t('common.refresh')}
                  >
                    <RotateCcw className="h-4 w-4" aria-hidden="true" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>{t('common.refresh')}</TooltipContent>
              </Tooltip>
            </TooltipProvider>
          )}
          {toolbarActions}
          {showBatchActions && (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="table-toolbar-control table-toolbar__batch-trigger gap-2"
                  aria-label={t('table_selection.actions')}
                  title={t('table_selection.selected_actions')}
                  disabled={loading || batchActionsDisabled || batchActionsBusy}
                  aria-busy={batchActionsBusy}
                >
                  {batchActionsBusy && <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />}
                  <span className="table-toolbar__action-label">{t('table_selection.actions')}</span>
                  <ChevronDown className="table-toolbar__action-caret h-4 w-4" aria-hidden="true" />
                  {!batchActionsBusy && <MoreHorizontal className="table-toolbar__action-overflow h-4 w-4" aria-hidden="true" />}
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent
                align="end"
                className="table-toolbar-menu"
                aria-label={t('table_selection.selected_actions')}
                aria-labelledby={undefined}
              >
                {batchActions.map((action, index) => (
                  <Fragment key={action.id}>
                    {action.destructive && index > 0 && !batchActions[index - 1].destructive && <DropdownMenuSeparator />}
                    <DropdownMenuItem
                      className={
                        action.destructive ? 'table-toolbar-menu__item table-toolbar-menu__item--destructive' : 'table-toolbar-menu__item'
                      }
                      disabled={loading || batchActionsDisabled || batchActionsBusy || action.disabled}
                      onSelect={() => {
                        void action.onSelect();
                      }}
                    >
                      {action.icon && (
                        <span className="shrink-0" aria-hidden="true">
                          {action.icon}
                        </span>
                      )}
                      {action.label}
                    </DropdownMenuItem>
                  </Fragment>
                ))}
              </DropdownMenuContent>
            </DropdownMenu>
          )}
        </div>
      </div>
    </div>
  );
}
