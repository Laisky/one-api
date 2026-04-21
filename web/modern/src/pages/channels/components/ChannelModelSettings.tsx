import { Button } from '@/components/ui/button';
import { FormControl, FormField, FormItem, FormMessage } from '@/components/ui/form';
import { SelectionListManager } from '@/components/ui/selection-list-manager';
import { Textarea } from '@/components/ui/textarea';
import { AlertCircle } from 'lucide-react';
import { useMemo } from 'react';
import type { UseFormReturn } from 'react-hook-form';
import { MODEL_CONFIGS_EXAMPLE, MODEL_MAPPING_EXAMPLE } from '../constants';
import { formatJSON, sanitizeJsonInput } from '../helpers';
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
      const parsed = JSON.parse(sanitizeJsonInput(modelMapping)) as Record<string, unknown>;
      if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') {
        return new Set<string>();
      }
      return new Set(
        Object.keys(parsed)
          .map((model) => model.trim().toLowerCase())
          .filter((model) => model.length > 0)
      );
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

  const mappingJsonIssue = useMemo<
    { type: 'parse'; message: string } | { type: 'shape' } | { type: 'invalid-entries'; entries: string[] } | null
  >(() => {
    if (!modelMapping.trim()) {
      return null;
    }
    try {
      const parsed = JSON.parse(sanitizeJsonInput(modelMapping));
      if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') {
        return { type: 'shape' };
      }
      const invalid: string[] = [];
      for (const [rawKey, rawValue] of Object.entries(parsed as Record<string, unknown>)) {
        const keyLabel = rawKey.trim().length > 0 ? rawKey : '(empty key)';
        if (rawKey.trim().length === 0) {
          invalid.push(keyLabel);
          continue;
        }
        if (typeof rawValue !== 'string' || rawValue.trim().length === 0) {
          invalid.push(keyLabel);
        }
      }
      if (invalid.length > 0) {
        return { type: 'invalid-entries', entries: invalid };
      }
      return null;
    } catch (error) {
      return { type: 'parse', message: error instanceof Error ? error.message : 'Invalid JSON' };
    }
  }, [modelMapping]);

  const hasMappingJsonIssue = mappingJsonIssue !== null;

  const availableModelSet = useMemo(
    () => new Set(availableModels.map((model) => model.id.trim().toLowerCase()).filter((model) => model.length > 0)),
    [availableModels]
  );

  const knownTargetSet = useMemo(() => {
    const combined = new Set<string>();
    availableModelSet.forEach((model) => combined.add(model));
    selectedModelSet.forEach((model) => combined.add(model));
    return combined;
  }, [availableModelSet, selectedModelSet]);

  const mappingKeysOutsideSupported = useMemo(() => {
    if (hasMappingJsonIssue || !modelMapping.trim()) {
      return [] as string[];
    }
    try {
      const parsed = JSON.parse(sanitizeJsonInput(modelMapping)) as Record<string, unknown>;
      return Object.keys(parsed)
        .map((key) => key.trim())
        .filter((key) => key.length > 0 && !selectedModelSet.has(key.toLowerCase()));
    } catch (_error) {
      return [] as string[];
    }
  }, [modelMapping, selectedModelSet, hasMappingJsonIssue]);

  const mappingTargetsOutsideCatalog = useMemo(() => {
    // When the channel type has no catalog we cannot judge target validity.
    if (hasMappingJsonIssue || !modelMapping.trim() || availableModelSet.size === 0) {
      return [] as { source: string; target: string }[];
    }
    try {
      const parsed = JSON.parse(sanitizeJsonInput(modelMapping)) as Record<string, unknown>;
      const offending: { source: string; target: string }[] = [];
      for (const [rawKey, rawValue] of Object.entries(parsed)) {
        if (typeof rawValue !== 'string') continue;
        const target = rawValue.trim();
        if (!target) continue;
        if (!knownTargetSet.has(target.toLowerCase())) {
          offending.push({ source: rawKey.trim(), target });
        }
      }
      return offending;
    } catch (_error) {
      return [] as { source: string; target: string }[];
    }
  }, [modelMapping, hasMappingJsonIssue, availableModelSet, knownTargetSet]);

  const hasUnreachableMappingKeys = mappingKeysOutsideSupported.length > 0;
  const hasUnknownMappingTargets = mappingTargetsOutsideCatalog.length > 0;
  const hasMappingWarning = hasUnreachableMappingKeys || hasUnknownMappingTargets;

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
    const current = form.getValues('model_mapping') ?? '';
    const formatted = formatJSON(current);
    form.setValue('model_mapping', formatted);
  };

  /**
   * formatModelConfigs formats the model_configs JSON for readability and updates the form value.
   * @returns void
   */
  const formatModelConfigs = () => {
    const current = form.getValues('model_configs') ?? '';
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
                  {tr(
                    'validation.hidden_models_not_supported',
                    'These hidden models are not currently supported by the channel: {{models}}',
                    {
                      models: hiddenModelsOutsideSupported.join(', '),
                    }
                  )}
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
                  className={`font-mono text-xs min-h-[150px] ${
                    fieldHasError('model_mapping') || hasMappingJsonIssue
                      ? 'border-destructive focus-visible:ring-destructive'
                      : hasMappingWarning
                        ? 'border-warning-border ring-1 ring-warning-border focus-visible:ring-warning'
                        : ''
                  }`}
                  {...field}
                  value={field.value || ''}
                />
              </FormControl>
              <FormMessage />
              {hasMappingJsonIssue && (
                <div className="mt-2 flex items-start gap-2 rounded-lg border border-destructive bg-destructive/10 p-3">
                  <AlertCircle className="mt-0.5 h-4 w-4 shrink-0 text-destructive" />
                  <span className="text-sm text-destructive">
                    {mappingJsonIssue?.type === 'shape'
                      ? tr('validation.model_mapping_shape_invalid', 'Model Mapping must be a JSON object of the form { "from": "to" }.')
                      : mappingJsonIssue?.type === 'invalid-entries'
                        ? tr(
                            'validation.model_mapping_invalid_entries',
                            'These Model Mapping entries have empty or non-string values: {{keys}}',
                            {
                              keys: mappingJsonIssue.entries.join(', '),
                            }
                          )
                        : tr('validation.model_mapping_parse_error', 'Model Mapping JSON is invalid: {{error}}', {
                            error: mappingJsonIssue && mappingJsonIssue.type === 'parse' ? mappingJsonIssue.message : '',
                          })}
                  </span>
                </div>
              )}
              {!hasMappingJsonIssue && hasUnreachableMappingKeys && (
                <div className="mt-2 flex items-start gap-2 rounded-lg border border-warning-border bg-warning-muted p-3">
                  <AlertCircle className="mt-0.5 h-4 w-4 shrink-0 text-warning" />
                  <span className="text-sm text-warning-foreground">
                    {tr(
                      'validation.mapping_keys_not_supported',
                      'These mapping keys are not in Supported Models, so requests to these aliases will be rejected: {{models}}',
                      {
                        models: mappingKeysOutsideSupported.join(', '),
                      }
                    )}
                  </span>
                </div>
              )}
              {!hasMappingJsonIssue && hasUnknownMappingTargets && (
                <div className="mt-2 flex items-start gap-2 rounded-lg border border-warning-border bg-warning-muted p-3">
                  <AlertCircle className="mt-0.5 h-4 w-4 shrink-0 text-warning" />
                  <span className="text-sm text-warning-foreground">
                    {tr(
                      'validation.mapping_targets_not_recognized',
                      'These mapping targets are not recognized as models for this channel: {{entries}}',
                      {
                        entries: mappingTargetsOutsideCatalog.map((entry) => `${entry.source} → ${entry.target}`).join(', '),
                      }
                    )}
                  </span>
                </div>
              )}
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
