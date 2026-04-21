import { Button } from '@/components/ui/button';
import { FormControl, FormField, FormItem, FormMessage } from '@/components/ui/form';
import { SelectionListManager } from '@/components/ui/selection-list-manager';
import { Textarea } from '@/components/ui/textarea';
import { AlertCircle } from 'lucide-react';
import { useMemo } from 'react';
import type { UseFormReturn } from 'react-hook-form';
import { MODEL_CONFIGS_EXAMPLE, MODEL_MAPPING_EXAMPLE } from '../constants';
import { formatJSON } from '../helpers';
import type { ChannelForm } from '../schemas';
import { LabelWithHelp } from './LabelWithHelp';

interface ChannelModelSettingsProps {
  form: UseFormReturn<ChannelForm>;
  availableModels: { id: string; name: string }[];
  currentCatalogModels: string[];
  defaultPricing: string;
  tr: (key: string, defaultValue: string, options?: Record<string, unknown>) => string;
  notify: (options: any) => void;
}

export const ChannelModelSettings = ({
  form,
  availableModels,
  currentCatalogModels,
  defaultPricing,
  tr,
  notify,
}: ChannelModelSettingsProps) => {
  const fieldHasError = (name: string) => !!(form.formState.errors as any)?.[name];
  const errorClass = (name: string) => (fieldHasError(name) ? 'border-destructive focus-visible:ring-destructive' : '');
  const selectedModels = form.watch('models');
  const hiddenModels = form.watch('hidden_models');
  const modelMapping = form.watch('model_mapping') || '';

  const selectedModelSet = useMemo(() => {
    return new Set(selectedModels.map((model) => model.trim().toLowerCase()).filter((model) => model.length > 0));
  }, [selectedModels]);

  const mappingSources = useMemo(() => {
    if (!modelMapping.trim()) {
      return new Set<string>();
    }
    try {
      const parsed = JSON.parse(modelMapping) as Record<string, unknown>;
      if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') {
        return new Set<string>();
      }
      return new Set(Object.keys(parsed).map((model) => model.trim().toLowerCase()).filter((model) => model.length > 0));
    } catch (_error) {
      return new Set<string>();
    }
  }, [modelMapping]);

  const hiddenModelOptions = useMemo(
    () =>
      selectedModels.map((model) => ({
        value: model,
        label: model,
      })),
    [selectedModels]
  );

  const hiddenModelsOutsideSupported = useMemo(
    () => hiddenModels.filter((model) => !selectedModelSet.has(model.trim().toLowerCase())),
    [hiddenModels, selectedModelSet]
  );

  const hiddenMappingSources = useMemo(
    () => hiddenModels.filter((model) => mappingSources.has(model.trim().toLowerCase())),
    [hiddenModels, mappingSources]
  );

  const fillRelatedModels = () => {
    if (currentCatalogModels.length === 0) {
      return;
    }
    const currentModels = form.getValues('models');
    const uniqueModels = [...new Set([...currentModels, ...currentCatalogModels])];
    form.setValue('models', uniqueModels);
  };

  const fillAllModels = () => {
    const currentModels = form.getValues('models');
    const allModelIds = availableModels.map((m) => m.id);
    const uniqueModels = [...new Set([...currentModels, ...allModelIds])];
    form.setValue('models', uniqueModels);
  };

  const clearModels = () => {
    form.setValue('models', []);
  };

  const formatModelMapping = () => {
    const current = form.getValues('model_mapping');
    const formatted = formatJSON(current);
    form.setValue('model_mapping', formatted);
  };

  /**
   * formatModelConfigs formats the model_configs JSON for readability and updates the form value.
   * @returns void
   */
  const formatModelConfigs = () => {
    const current = form.getValues('model_configs');
    const formatted = formatJSON(current);
    form.setValue('model_configs', formatted);
  };

  /**
   * loadDefaultModelConfigs applies the default pricing config to the model_configs field.
   * @returns void
   */
  const loadDefaultModelConfigs = () => {
    console.debug('[ChannelModelSettings] Load default model configs', {
      hasDefaultPricing: Boolean(defaultPricing),
    });
    if (!defaultPricing) {
      return;
    }
    form.setValue('model_configs', defaultPricing);
  };

  return (
    <div className="space-y-6">
      <FormField
        control={form.control}
        name="models"
        render={() => (
          <FormItem>
            <SelectionListManager
              label={tr('models.label', 'Models *')}
              help={tr('models.help', 'Select the models supported by this channel.')}
              options={availableModels.map((model) => ({
                value: model.id,
                label: model.name,
              }))}
              selected={form.watch('models')}
              onChange={(next) => form.setValue('models', next)}
              searchPlaceholder={tr('models.search_placeholder', 'Search models...')}
              customPlaceholder={tr('models.custom_placeholder', 'Add custom model...')}
              addLabel={tr('common.add', 'Add')}
              selectedSummaryLabel={(count) =>
                tr('models.selected_count', 'Selected Models ({{count}})', {
                  count,
                })
              }
              emptySelectedLabel={tr('models.no_selection', 'No models selected')}
              noOptionsLabel={tr('models.no_match', 'No models found')}
              actions={
                <>
                  <Button type="button" variant="outline" size="sm" onClick={fillRelatedModels}>
                    {tr('models.fill_related', 'Fill Related ({{count}})', {
                      count: currentCatalogModels.length,
                    })}
                  </Button>
                  <Button type="button" variant="outline" size="sm" onClick={fillAllModels}>
                    {tr('models.fill_all', 'Fill All ({{count}})', {
                      count: availableModels.length,
                    })}
                  </Button>
                  <Button type="button" variant="outline" onClick={clearModels} className="text-destructive hover:text-destructive">
                    {tr('models.clear', 'Clear')}
                  </Button>
                </>
              }
            />
            <FormMessage />
          </FormItem>
        )}
      />

      <FormField
        control={form.control}
        name="hidden_models"
        render={() => (
          <FormItem>
            <SelectionListManager
              label={tr('hidden_models.label', 'Hidden Models')}
              help={tr(
                'hidden_models.help',
                'Models listed here are served by this channel but not returned from /v1/models and rejected from direct user requests. Useful for exposing a unified alias via Model Mapping.'
              )}
              options={hiddenModelOptions}
              selected={hiddenModels}
              onChange={(next) => form.setValue('hidden_models', next)}
              searchPlaceholder={tr('hidden_models.search_placeholder', 'Search hidden models...')}
              customPlaceholder={tr('hidden_models.custom_placeholder', 'Add hidden model...')}
              addLabel={tr('common.add', 'Add')}
              selectedSummaryLabel={(count) =>
                tr('hidden_models.selected_count', 'Hidden Models ({{count}})', {
                  count,
                })
              }
              emptySelectedLabel={tr('hidden_models.no_selection', 'No hidden models selected')}
              noOptionsLabel={tr('hidden_models.no_match', 'No models found')}
            />
            <FormMessage />

            {hiddenModelsOutsideSupported.length > 0 && (
              <div className="mt-3 flex items-start gap-2 rounded-lg border border-warning-border bg-warning-muted p-3">
                <AlertCircle className="mt-0.5 h-4 w-4 shrink-0 text-warning" />
                <span className="text-sm text-warning-foreground">
                  {tr('validation.hidden_models_not_supported', 'These hidden models are not currently supported by the channel: {{models}}', {
                    models: hiddenModelsOutsideSupported.join(', '),
                  })}
                </span>
              </div>
            )}

            {hiddenMappingSources.length > 0 && (
              <div className="mt-3 flex items-start gap-2 rounded-lg border border-warning-border bg-warning-muted p-3">
                <AlertCircle className="mt-0.5 h-4 w-4 shrink-0 text-warning" />
                <span className="text-sm text-warning-foreground">
                  {tr(
                    'validation.hidden_models_unreachable_alias',
                    'These hidden models are used as Model Mapping sources, so the public aliases will become unreachable: {{models}}',
                    {
                      models: hiddenMappingSources.join(', '),
                    }
                  )}
                </span>
              </div>
            )}
          </FormItem>
        )}
      />

      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
        <FormField
          control={form.control}
          name="model_mapping"
          render={({ field }) => (
            <FormItem>
              <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2">
                <LabelWithHelp
                  label={tr('model_mapping.label', 'Model Mapping')}
                  help={tr('model_mapping.help', 'Map request model names to upstream model names (JSON).')}
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="h-6 text-xs self-start sm:self-auto"
                  onClick={formatModelMapping}
                >
                  {tr('common.format_json', 'Format JSON')}
                </Button>
              </div>
              <FormControl>
                <Textarea
                  placeholder={tr('model_mapping.placeholder', '{"gpt-3.5-turbo-0301": "gpt-3.5-turbo"}', {
                    example: JSON.stringify(MODEL_MAPPING_EXAMPLE, null, 2),
                  })}
                  className={`font-mono text-xs min-h-[150px] ${errorClass('model_mapping')}`}
                  {...field}
                  value={field.value || ''}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        <FormField
          control={form.control}
          name="model_configs"
          render={({ field }) => (
            <FormItem>
              <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2">
                <LabelWithHelp
                  label={tr('model_configs.label', 'Model Configs')}
                  help={tr('model_configs.help', 'Custom pricing and limits for specific models (JSON).')}
                />
                <div className="flex flex-wrap gap-2 self-start sm:self-auto">
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className="h-6 text-xs"
                    onClick={loadDefaultModelConfigs}
                    disabled={!defaultPricing}
                  >
                    {tr('model_configs.load_default', 'Load Default')}
                  </Button>
                  <Button type="button" variant="ghost" size="sm" className="h-6 text-xs" onClick={formatModelConfigs}>
                    {tr('common.format_json', 'Format JSON')}
                  </Button>
                </div>
              </div>
              <FormControl>
                <Textarea
                  placeholder={tr('model_configs.placeholder', '{"gpt-4": {"ratio": 0.03, "completion_ratio": 2.0}}', {
                    example: JSON.stringify(MODEL_CONFIGS_EXAMPLE, null, 2),
                  })}
                  className={`font-mono text-xs min-h-[150px] ${errorClass('model_configs')}`}
                  {...field}
                  value={field.value || ''}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      </div>
    </div>
  );
};
