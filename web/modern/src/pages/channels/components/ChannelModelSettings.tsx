import { useState } from "react";
import type { UseFormReturn } from "react-hook-form";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
	FormControl,
	FormField,
	FormItem,
	FormMessage,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { MODEL_CONFIGS_EXAMPLE, MODEL_MAPPING_EXAMPLE } from "../constants";
import { formatJSON } from "../helpers";
import type { ChannelForm } from "../schemas";
import { LabelWithHelp } from "./LabelWithHelp";

interface ChannelModelSettingsProps {
	form: UseFormReturn<ChannelForm>;
	availableModels: { id: string; name: string }[];
	currentCatalogModels: string[];
	defaultPricing: string;
	tr: (
		key: string,
		defaultValue: string,
		options?: Record<string, unknown>,
	) => string;
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
	const [modelSearchTerm, setModelSearchTerm] = useState("");
	const [customModel, setCustomModel] = useState("");

	const fieldHasError = (name: string) =>
		!!(form.formState.errors as any)?.[name];
	const errorClass = (name: string) =>
		fieldHasError(name)
			? "border-destructive focus-visible:ring-destructive"
			: "";

	const filteredModels = availableModels.filter((model) =>
		model.name.toLowerCase().includes(modelSearchTerm.toLowerCase()),
	);

	const toggleModel = (modelValue: string) => {
		const currentModels = form.getValues("models");
		if (currentModels.includes(modelValue)) {
			form.setValue(
				"models",
				currentModels.filter((m) => m !== modelValue),
			);
		} else {
			form.setValue("models", [...currentModels, modelValue]);
		}
	};

	const addCustomModel = () => {
		if (!customModel.trim()) return;
		const currentModels = form.getValues("models");
		if (currentModels.includes(customModel)) return;

		form.setValue("models", [...currentModels, customModel]);
		setCustomModel("");
	};

	const removeModel = (modelToRemove: string) => {
		const currentModels = form.getValues("models");
		form.setValue(
			"models",
			currentModels.filter((m) => m !== modelToRemove),
		);
	};

	const fillRelatedModels = () => {
		if (currentCatalogModels.length === 0) {
			return;
		}
		const currentModels = form.getValues("models");
		const uniqueModels = [
			...new Set([...currentModels, ...currentCatalogModels]),
		];
		form.setValue("models", uniqueModels);
	};

	const fillAllModels = () => {
		const currentModels = form.getValues("models");
		const allModelIds = availableModels.map((m) => m.id);
		const uniqueModels = [...new Set([...currentModels, ...allModelIds])];
		form.setValue("models", uniqueModels);
	};

	const clearModels = () => {
		form.setValue("models", []);
	};

	const formatModelMapping = () => {
		const current = form.getValues("model_mapping");
		const formatted = formatJSON(current);
		form.setValue("model_mapping", formatted);
	};

	const loadDefaultModelConfigs = () => {
		if (defaultPricing) {
			form.setValue("model_configs", defaultPricing);
		}
	};

	const formatModelConfigs = () => {
		const value = form.getValues("model_configs");
		if (!value) {
			// We need MODEL_CONFIGS_EXAMPLE here, but it's in constants.
			// I'll just use a simple default or import it.
			// Let's import it.
			return;
		}
		try {
			const parsed = JSON.parse(value);
			form.setValue("model_configs", JSON.stringify(parsed, null, 2), {
				shouldDirty: true,
				shouldValidate: true,
			});
		} catch (error) {
			notify({
				type: "error",
				title: tr("validation.invalid_json_title", "Invalid JSON"),
				message: tr(
					"model_configs.format_error",
					"Unable to format model_configs: {{error}}",
					{ error: (error as Error).message },
				),
			});
		}
	};

	return (
		<div className="space-y-6">
			<FormField
				control={form.control}
				name="models"
				render={() => (
					<FormItem>
						<LabelWithHelp
							label={tr("models.label", "Models *")}
							help={tr(
								"models.help",
								"Select the models supported by this channel.",
							)}
						/>
						<div className="space-y-4 border rounded-md p-4">
							<div className="flex flex-wrap gap-2">
								<Button
									type="button"
									variant="outline"
									size="sm"
									onClick={fillRelatedModels}
								>
									{tr("models.fill_related", "Fill Related ({{count}})", { count: currentCatalogModels.length })}
								</Button>
								<Button
									type="button"
									variant="outline"
									size="sm"
									onClick={fillAllModels}
								>
									{tr("models.fill_all", "Fill All ({{count}})", { count: availableModels.length })}
								</Button>
								<Button
									type="button"
									variant="outline"
									size="sm"
									onClick={clearModels}
									className="text-destructive hover:text-destructive"
								>
									{tr("models.clear", "Clear")}
								</Button>
							</div>

							<div className="flex gap-2">
								<Input
									placeholder={tr(
										"models.search_placeholder",
										"Search models...",
									)}
									value={modelSearchTerm}
									onChange={(e) => setModelSearchTerm(e.target.value)}
									className="flex-1"
								/>
								<div className="flex gap-2 flex-1">
									<Input
										placeholder={tr(
											"models.custom_placeholder",
											"Add custom model...",
										)}
										value={customModel}
										onChange={(e) => setCustomModel(e.target.value)}
										onKeyDown={(e) =>
											e.key === "Enter" &&
											(e.preventDefault(), addCustomModel())
										}
									/>
									<Button
										type="button"
										variant="secondary"
										onClick={addCustomModel}
									>
										{tr("common.add", "Add")}
									</Button>
								</div>
							</div>

							<div className="max-h-[200px] overflow-y-auto border rounded p-2 bg-muted/10">
								<div className="flex flex-wrap gap-2">
									{filteredModels.map((model) => {
										const isSelected = form.watch("models").includes(model.id);
										return (
											<Badge
												key={model.id}
												variant={isSelected ? "default" : "outline"}
												className="cursor-pointer hover:bg-primary/90"
												onClick={() => toggleModel(model.id)}
											>
												{model.name}
											</Badge>
										);
									})}
								</div>
							</div>

							<div className="space-y-2">
								<div className="text-sm font-medium text-muted-foreground">
									{tr("models.selected_count", "Selected Models ({{count}})", {
										count: form.watch("models").length,
									})}
								</div>
								<div className="flex flex-wrap gap-2 min-h-[40px] p-2 border rounded bg-background">
									{form.watch("models").length === 0 && (
										<span className="text-sm text-muted-foreground italic p-1">
											{tr("models.no_selection", "No models selected")}
										</span>
									)}
									{form.watch("models").map((model) => (
										<Badge key={model} variant="secondary" className="gap-1">
											{model}
											<span
												className="cursor-pointer ml-1 hover:text-destructive"
												onClick={() => removeModel(model)}
											>
												×
											</span>
										</Badge>
									))}
								</div>
							</div>
						</div>
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
									label={tr("model_mapping.label", "Model Mapping")}
									help={tr(
										"model_mapping.help",
										"Map request model names to upstream model names (JSON).",
									)}
								/>
								<Button
									type="button"
									variant="ghost"
									size="sm"
									className="h-6 text-xs"
									onClick={formatModelMapping}
								>
									{tr("common.format_json", "Format JSON")}
								</Button>
							</div>
							<FormControl>
								<Textarea
									placeholder={tr(
										"model_mapping.placeholder",
										'{"gpt-3.5-turbo-0301": "gpt-3.5-turbo"}',
										{ example: JSON.stringify(MODEL_MAPPING_EXAMPLE, null, 2) },
									)}
									className={`font-mono text-xs min-h-[150px] ${errorClass("model_mapping")}`}
									{...field}
									value={field.value || ""}
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
									label={tr("model_configs.label", "Model Configs")}
									help={tr(
										"model_configs.help",
										"Custom pricing and limits for specific models (JSON).",
									)}
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
										{tr("model_configs.load_default", "Load Default")}
									</Button>
									<Button
										type="button"
										variant="ghost"
										size="sm"
										className="h-6 text-xs"
										onClick={formatModelConfigs}
									>
										{tr("common.format_json", "Format JSON")}
									</Button>
								</div>
							</div>
							<FormControl>
								<Textarea
									placeholder={tr(
										"model_configs.placeholder",
										'{"gpt-4": {"ratio": 0.03, "completion_ratio": 2.0}}',
										{ example: JSON.stringify(MODEL_CONFIGS_EXAMPLE, null, 2) },
									)}
									className={`font-mono text-xs min-h-[150px] ${errorClass("model_configs")}`}
									{...field}
									value={field.value || ""}
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
