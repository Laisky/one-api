import { Button } from '@/components/ui/button';
import { FormControl, FormField, FormItem, FormMessage } from '@/components/ui/form';
import { SelectionListManager } from '@/components/ui/selection-list-manager';
import { Textarea } from '@/components/ui/textarea';
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

      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
        <FormField
          control={form.control}
          name="model_mapping"
          render={({ field }) => (
            <FormItem>
              <div className="flex items-center justify-between">
                <LabelWithHelp
                  label={tr('model_mapping.label', 'Model Mapping')}
                  help={tr('model_mapping.help', 'Map request model names to upstream model names (JSON).')}
                />
                <Button type="button" variant="ghost" size="sm" className="h-6 text-xs" onClick={formatModelMapping}>
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
              <div className="flex items-center justify-between">
                <LabelWithHelp
                  label={tr('model_configs.label', 'Model Configs')}
                  help={tr('model_configs.help', 'Custom pricing and limits for specific models (JSON).')}
                />
                <div className="flex gap-2">
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
