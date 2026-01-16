import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Form } from "@/components/ui/form";
import { TooltipProvider } from "@/components/ui/tooltip";
import { logEditPageLayout } from "@/dev/layout-debug";
import { AlertCircle, Info } from "lucide-react";
import { useEffect } from "react";

import { ChannelAdvancedSettings } from "./components/ChannelAdvancedSettings";
import { ChannelBasicInfo } from "./components/ChannelBasicInfo";
import { ChannelEndpointSettings } from "./components/ChannelEndpointSettings";
import { ChannelMCPSettings } from "./components/ChannelMCPSettings";
import { ChannelModelSettings } from "./components/ChannelModelSettings";
import { ChannelSpecificConfig } from "./components/ChannelSpecificConfig";
import { ChannelToolingSettings } from "./components/ChannelToolingSettings";
import { ChannelTypeChangeDialog } from "./components/ChannelTypeChangeDialog";
import { CHANNEL_TYPES } from "./constants";
import { useChannelForm } from "./hooks/useChannelForm";

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

  const selectedChannelType = CHANNEL_TYPES.find(
    (t) => t.value === normalizedChannelType
  );
  const shouldShowLoading = loading || (isEdit && !formInitialized);

  // Layout diagnostics
  useEffect(() => {
    if (!shouldShowLoading) {
      logEditPageLayout("EditChannelPage");
    }
  }, [shouldShowLoading, watchType]);

  if (shouldShowLoading) {
    return (
      <div className="container mx-auto px-4 py-8">
        <Card>
          <CardContent className="flex items-center justify-center py-12">
            <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
            <span className="ml-3">{tr("loading", "Loading channel...")}</span>
          </CardContent>
        </Card>
      </div>
    );
  }

  const availableModels = (modelsCatalog[normalizedChannelType ?? -1] ?? [])
    .map((model) => ({ id: model, name: model }))
    .sort((a, b) => a.name.localeCompare(b.name));

  const currentCatalogModels = modelsCatalog[normalizedChannelType ?? -1] ?? [];

  // RHF invalid handler
  const onInvalid = (errors: any) => {
    const firstKey = Object.keys(errors)[0];
    const firstMsg =
      errors[firstKey]?.message || "Please correct the highlighted fields.";
    notify({
      type: "error",
      title: tr("validation.error_title", "Validation error"),
      message: String(firstMsg),
    });
    const el = document.querySelector(
      `[name="${firstKey}"]`
    ) as HTMLElement | null;
    if (el) {
      el.scrollIntoView({ behavior: "smooth", block: "center" });
      (el as any).focus?.();
    }
  };

  // Get type names for the confirmation dialog
  const getTypeName = (typeValue: number) => {
    const type = CHANNEL_TYPES.find((t) => t.value === typeValue);
    return type?.text || `Type ${typeValue}`;
  };

  return (
    <div className="container mx-auto px-4 py-6">
      <TooltipProvider>
        {/* Channel Type Change Confirmation Dialog */}
        <ChannelTypeChangeDialog
          open={pendingTypeChange !== null}
          onOpenChange={(open) => {
            if (!open) cancelTypeChange();
          }}
          fromType={
            pendingTypeChange ? getTypeName(pendingTypeChange.fromType) : ""
          }
          toType={
            pendingTypeChange ? getTypeName(pendingTypeChange.toType) : ""
          }
          onConfirm={confirmTypeChange}
          onCancel={cancelTypeChange}
          tr={tr}
        />
        <Card>
          <CardHeader>
            <CardTitle>
              {isEdit
                ? tr("title.edit", "Edit Channel")
                : tr("title.create", "Create Channel")}
            </CardTitle>
            <CardDescription>
              {isEdit
                ? tr("description.edit", "Update channel configuration")
                : tr("description.create", "Create a new API channel")}
            </CardDescription>
            {selectedChannelType?.description && (
              <div className="flex items-center gap-2 p-3 bg-blue-50 border border-blue-200 rounded-lg">
                <Info className="h-4 w-4 text-blue-600" />
                <span className="text-sm text-blue-800">
                  {tr(
                    `channel_type.${selectedChannelType.value}.description`,
                    selectedChannelType.description
                  )}
                </span>
              </div>
            )}
            {selectedChannelType?.tip && (
              <div className="flex items-center gap-2 p-3 bg-yellow-50 border border-yellow-200 rounded-lg">
                <AlertCircle className="h-4 w-4 text-yellow-600" />
                <span
                  className="text-sm text-yellow-800"
                  dangerouslySetInnerHTML={{
                    __html: tr(
                      `channel_type.${selectedChannelType.value}.tip`,
                      selectedChannelType.tip
                    ),
                  }}
                />
              </div>
            )}
          </CardHeader>
          <CardContent>
            <Form {...form}>
              <form
                onSubmit={form.handleSubmit(onSubmit, onInvalid)}
                className="space-y-6"
              >
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

                <ChannelAdvancedSettings
                  form={form}
                  normalizedChannelType={normalizedChannelType}
                  tr={tr}
                />

                <ChannelEndpointSettings
                  form={form}
                  allEndpoints={allEndpoints}
                  defaultEndpoints={defaultEndpoints}
                  tr={tr}
                />

                <ChannelToolingSettings
                  form={form}
                  defaultTooling={defaultTooling}
                  tr={tr}
                  notify={notify}
                />

                <ChannelMCPSettings form={form} tr={tr} />

                {form.formState.errors.root && (
                  <div className="text-sm text-destructive">
                    {form.formState.errors.root.message}
                  </div>
                )}

                <div className="flex gap-2">
                  <Button type="submit" disabled={isSubmitting}>
                    {isSubmitting
                      ? isEdit
                        ? tr("actions.updating", "Updating...")
                        : tr("actions.creating", "Creating...")
                      : isEdit
                        ? tr("actions.update", "Update Channel")
                        : tr("actions.create", "Create Channel")}
                  </Button>
                  {isEdit && (
                    <Button
                      type="button"
                      variant="secondary"
                      onClick={testChannel}
                      disabled={isSubmitting}
                    >
                      {tr("actions.test_channel", "Test Channel")}
                    </Button>
                  )}
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() => window.history.back()}
                  >
                    {tr("actions.cancel", "Cancel")}
                  </Button>
                </div>
              </form>
            </Form>
          </CardContent>
        </Card>
      </TooltipProvider>
    </div>
  );
}

export default EditChannelPage;
