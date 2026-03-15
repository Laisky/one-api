import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Form } from '@/components/ui/form';
import { ResponsivePageContainer } from '@/components/ui/responsive-container';
import { TooltipProvider } from '@/components/ui/tooltip';
import { logEditPageLayout } from '@/dev/layout-debug';
import { AlertCircle, Info } from 'lucide-react';
import { useEffect } from 'react';

import { ChannelAdvancedSettings } from './components/ChannelAdvancedSettings';
import { ChannelBasicInfo } from './components/ChannelBasicInfo';
import { ChannelEndpointSettings } from './components/ChannelEndpointSettings';
import { ChannelMCPSettings } from './components/ChannelMCPSettings';
import { ChannelModelSettings } from './components/ChannelModelSettings';
import { ChannelSpecificConfig } from './components/ChannelSpecificConfig';
import { ChannelToolingSettings } from './components/ChannelToolingSettings';
import { ChannelTypeChangeDialog } from './components/ChannelTypeChangeDialog';
import { CHANNEL_TYPES } from './constants';
import { useChannelForm } from './hooks/useChannelForm';

export function EditChannelPage() {
  const {
    form,
    isEdit,
    loading,
    isSubmitting,
    modelsCatalog,
    groups,
    defaultPricing,
    defaultTooling,
    defaultBaseURL,
    baseURLEditable,
    defaultEndpoints,
    allEndpoints,
    formInitialized,
    normalizedChannelType,
    watchType,
    onSubmit,
    testChannel,
    tr,
    notify,
    // Type change handling
    pendingTypeChange,
    requestTypeChange,
    confirmTypeChange,
    cancelTypeChange,
  } = useChannelForm();

  const selectedChannelType = CHANNEL_TYPES.find((t) => t.value === normalizedChannelType);
  const shouldShowLoading = loading || (isEdit && !formInitialized);

  // Layout diagnostics
  useEffect(() => {
    if (!shouldShowLoading) {
      logEditPageLayout('EditChannelPage');
    }
  }, [shouldShowLoading, watchType]);

  if (shouldShowLoading) {
    return (
      <ResponsivePageContainer
        title={isEdit ? tr('title.edit', 'Edit Channel') : tr('title.create', 'Create Channel')}
        description={
          isEdit ? tr('description.edit', 'Update channel configuration') : tr('description.create', 'Create a new API channel')
        }
      >
        <Card className="border-0 shadow-none md:border md:shadow-sm">
          <CardContent className="flex items-center justify-center py-12">
            <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
            <span className="ml-3">{tr('loading', 'Loading channel...')}</span>
          </CardContent>
        </Card>
      </ResponsivePageContainer>
    );
  }

  const availableModels = (modelsCatalog[normalizedChannelType ?? -1] ?? [])
    .map((model) => ({ id: model, name: model }))
    .sort((a, b) => a.name.localeCompare(b.name));

  const currentCatalogModels = modelsCatalog[normalizedChannelType ?? -1] ?? [];

  // RHF invalid handler
  const onInvalid = (errors: any) => {
    const firstKey = Object.keys(errors)[0];
    const firstMsg = errors[firstKey]?.message || 'Please correct the highlighted fields.';
    notify({
      type: 'error',
      title: tr('validation.error_title', 'Validation error'),
      message: String(firstMsg),
    });
    const el = document.querySelector(`[name="${firstKey}"]`) as HTMLElement | null;
    if (el) {
      el.scrollIntoView({ behavior: 'smooth', block: 'center' });
      (el as any).focus?.();
    }
  };

  // Get type names for the confirmation dialog
  const getTypeName = (typeValue: number) => {
    const type = CHANNEL_TYPES.find((t) => t.value === typeValue);
    return type?.text || `Type ${typeValue}`;
  };

  return (
    <ResponsivePageContainer
      title={isEdit ? tr('title.edit', 'Edit Channel') : tr('title.create', 'Create Channel')}
      description={isEdit ? tr('description.edit', 'Update channel configuration') : tr('description.create', 'Create a new API channel')}
    >
      <TooltipProvider>
        {/* Channel Type Change Confirmation Dialog */}
        <ChannelTypeChangeDialog
          open={pendingTypeChange !== null}
          onOpenChange={(open) => {
            if (!open) cancelTypeChange();
          }}
          fromType={pendingTypeChange ? getTypeName(pendingTypeChange.fromType) : ''}
          toType={pendingTypeChange ? getTypeName(pendingTypeChange.toType) : ''}
          onConfirm={confirmTypeChange}
          onCancel={cancelTypeChange}
          tr={tr}
        />
        <Card className="border-0 shadow-none md:border md:shadow-sm">
          <CardContent className="space-y-6 p-4 sm:p-6">
            {selectedChannelType?.description && (
              <div className="flex items-center gap-2 p-3 bg-info-muted border border-info-border rounded-lg">
                <Info className="h-4 w-4 text-info" />
                <span className="text-sm text-info-foreground">
                  {tr(`channel_type.${selectedChannelType.value}.description`, selectedChannelType.description)}
                </span>
              </div>
            )}
            {selectedChannelType?.tip && (
              <div className="flex items-center gap-2 p-3 bg-warning-muted border border-warning-border rounded-lg">
                <AlertCircle className="h-4 w-4 text-warning" />
                <span className="text-sm text-warning-foreground">
                  {tr(`channel_type.${selectedChannelType.value}.tip`, selectedChannelType.tip)}
                </span>
              </div>
            )}
            <Form {...form}>
              <form onSubmit={form.handleSubmit(onSubmit, onInvalid)} className="space-y-6">
                <ChannelBasicInfo
                  form={form}
                  groups={groups}
                  normalizedChannelType={normalizedChannelType}
                  tr={tr}
                  onTypeChange={requestTypeChange}
                />

                <ChannelSpecificConfig
                  form={form}
                  normalizedChannelType={normalizedChannelType}
                  defaultBaseURL={defaultBaseURL}
                  baseURLEditable={baseURLEditable}
                  tr={tr}
                />

                <ChannelModelSettings
                  form={form}
                  availableModels={availableModels}
                  currentCatalogModels={currentCatalogModels}
                  defaultPricing={defaultPricing}
                  tr={tr}
                  notify={notify}
                />

                <ChannelAdvancedSettings form={form} normalizedChannelType={normalizedChannelType} tr={tr} />

                <ChannelEndpointSettings form={form} allEndpoints={allEndpoints} defaultEndpoints={defaultEndpoints} tr={tr} />

                <ChannelToolingSettings form={form} defaultTooling={defaultTooling} tr={tr} notify={notify} />

                <ChannelMCPSettings form={form} tr={tr} />

                {form.formState.errors.root && <div className="text-sm text-destructive">{form.formState.errors.root.message}</div>}

                <div className="flex flex-col-reverse gap-2 sm:flex-row sm:flex-wrap sm:justify-end">
                  <Button type="button" variant="outline" onClick={() => window.history.back()} className="w-full sm:w-auto">
                    {tr('actions.cancel', 'Cancel')}
                  </Button>
                  {isEdit && (
                    <Button type="button" variant="secondary" onClick={testChannel} disabled={isSubmitting} className="w-full sm:w-auto">
                      {tr('actions.test_channel', 'Test Channel')}
                    </Button>
                  )}
                  <Button type="submit" disabled={isSubmitting} className="w-full sm:w-auto">
                    {isSubmitting
                      ? isEdit
                        ? tr('actions.updating', 'Updating...')
                        : tr('actions.creating', 'Creating...')
                      : isEdit
                        ? tr('actions.update', 'Update Channel')
                        : tr('actions.create', 'Create Channel')}
                  </Button>
                </div>
              </form>
            </Form>
          </CardContent>
        </Card>
      </TooltipProvider>
    </ResponsivePageContainer>
  );
}

export default EditChannelPage;
