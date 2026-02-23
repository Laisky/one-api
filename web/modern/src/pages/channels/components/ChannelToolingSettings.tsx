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
import { TOOLING_CONFIG_EXAMPLE } from "../constants";
import { useChannelTooling } from "../hooks/useChannelTooling";
import type { ChannelForm } from "../schemas";
import { LabelWithHelp } from "./LabelWithHelp";

interface ChannelToolingSettingsProps {
	form: UseFormReturn<ChannelForm>;
	defaultTooling: string;
	tr: (
		key: string,
		defaultValue: string,
		options?: Record<string, unknown>,
	) => string;
	notify: (options: any) => void;
}

export const ChannelToolingSettings = ({
	form,
	defaultTooling,
	tr,
	notify,
}: ChannelToolingSettingsProps) => {
	const {
		customTool,
		setCustomTool,
		currentToolWhitelist,
		pricedToolSet,
		availableDefaultTools,
		toolEditorDisabled,
		addToolToWhitelist,
		removeToolFromWhitelist,
		formatToolingConfig,
	} = useChannelTooling(form, defaultTooling, notify, tr);

	const fieldHasError = (name: string) =>
		!!(form.formState.errors as any)?.[name];
	const errorClass = (name: string) =>
		fieldHasError(name)
			? "border-destructive focus-visible:ring-destructive"
			: "";

	return (
		<FormField
			control={form.control}
			name="tooling"
			render={({ field }) => (
				<FormItem>
					<div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2">
						<LabelWithHelp
							label={tr("tooling.label", "Tooling Config")}
							help={tr(
								"tooling.help",
								"Configure allowed tools and their pricing (JSON).",
							)}
						/>
						<Button
							type="button"
							variant="ghost"
							size="sm"
							className="h-6 text-xs self-start sm:self-auto"
							onClick={formatToolingConfig}
						>
							{tr("common.format_json", "Format JSON")}
						</Button>
					</div>
					<FormControl>
						<Textarea
							placeholder={tr(
								"tooling.placeholder",
								'{"whitelist": ["web_search"], "pricing": {"web_search": {"usd_per_call": 0.025}}}',
								{ example: JSON.stringify(TOOLING_CONFIG_EXAMPLE, null, 2) },
							)}
							className={`font-mono text-xs min-h-[150px] ${errorClass("tooling")}`}
							{...field}
							value={field.value || ""}
						/>
					</FormControl>
					<FormMessage />

					<div className="mt-4 space-y-4 border rounded-md p-4 bg-muted/5">
						<div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2">
							<h4 className="text-sm font-medium">
								{tr("tooling.whitelist_editor", "Whitelist Editor")}
							</h4>
							{toolEditorDisabled && (
								<span className="text-xs text-destructive">
									{tr(
										"tooling.editor_disabled",
										"Fix JSON error to enable editor",
									)}
								</span>
							)}
						</div>

						<div className="flex gap-2">
							<Input
								placeholder={tr("tooling.add_custom", "Add custom tool...")}
								value={customTool}
								onChange={(e) => setCustomTool(e.target.value)}
								onKeyDown={(e) =>
									e.key === "Enter" &&
									(e.preventDefault(),
									addToolToWhitelist(customTool, { isCustom: true }))
								}
								disabled={toolEditorDisabled}
								className="flex-1"
							/>
							<Button
								type="button"
								variant="secondary"
								onClick={() =>
									addToolToWhitelist(customTool, { isCustom: true })
								}
								disabled={toolEditorDisabled || !customTool.trim()}
							>
								{tr("common.add", "Add")}
							</Button>
						</div>

						<div className="space-y-2">
							<div className="text-xs font-medium text-muted-foreground">
								{tr("tooling.available_tools", "Available Tools")}
							</div>
							<div className="flex flex-wrap gap-2 max-h-[100px] overflow-y-auto p-2 border rounded bg-background">
								{availableDefaultTools.map((tool) => {
									const isSelected = currentToolWhitelist.some(
										(t) => t.toLowerCase() === tool.toLowerCase(),
									);
									const hasPricing = pricedToolSet.has(tool.toLowerCase());
									return (
										<Badge
											key={tool}
											variant={isSelected ? "default" : "outline"}
											className={`cursor-pointer hover:bg-primary/90 ${hasPricing ? "border-blue-200" : ""}`}
											onClick={() => !isSelected && addToolToWhitelist(tool)}
										>
											{tool}
											{hasPricing && (
												<span className="ml-1 text-[10px] text-blue-500">
													($)
												</span>
											)}
										</Badge>
									);
								})}
							</div>
						</div>

						<div className="space-y-2">
							<div className="text-xs font-medium text-muted-foreground">
								{tr(
									"tooling.whitelisted_tools",
									"Whitelisted Tools ({{count}})",
									{ count: currentToolWhitelist.length },
								)}
							</div>
							<div className="flex flex-wrap gap-2 min-h-[40px] p-2 border rounded bg-background">
								{currentToolWhitelist.length === 0 && (
									<span className="text-xs text-muted-foreground italic p-1">
										{tr("tooling.no_whitelist", "No tools whitelisted")}
									</span>
								)}
								{currentToolWhitelist.map((tool) => (
									<Badge key={tool} variant="secondary" className="gap-1 max-w-full">
										<span className="truncate min-w-0" title={tool}>{tool}</span>
										<span
											className="cursor-pointer ml-1 hover:text-destructive shrink-0"
											onClick={() => removeToolFromWhitelist(tool)}
										>
											×
										</span>
									</Badge>
								))}
							</div>
						</div>
					</div>
				</FormItem>
			)}
		/>
	);
};
