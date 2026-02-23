import type { UseFormReturn } from "react-hook-form";
import { Button } from "@/components/ui/button";
import {
	FormControl,
	FormField,
	FormItem,
	FormMessage,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { INFERENCE_PROFILE_ARN_MAP_EXAMPLE } from "../constants";
import { formatJSON } from "../helpers";
import type { ChannelForm } from "../schemas";
import { LabelWithHelp } from "./LabelWithHelp";

interface ChannelAdvancedSettingsProps {
	form: UseFormReturn<ChannelForm>;
	normalizedChannelType: number | null;
	tr: (
		key: string,
		defaultValue: string,
		options?: Record<string, unknown>,
	) => string;
}

// AWS Bedrock channel type constant
const AWS_BEDROCK_CHANNEL_TYPE = 33;

export const ChannelAdvancedSettings = ({
	form,
	normalizedChannelType,
	tr,
}: ChannelAdvancedSettingsProps) => {
	const fieldHasError = (name: string) =>
		!!(form.formState.errors as any)?.[name];
	const errorClass = (name: string) =>
		fieldHasError(name)
			? "border-destructive focus-visible:ring-destructive"
			: "";

	const _formatOtherConfig = () => {
		const current = form.getValues("other");
		const formatted = formatJSON(current);
		form.setValue("other", formatted);
	};

	const formatInferenceProfileArnMap = () => {
		const current = form.getValues("inference_profile_arn_map");
		const formatted = formatJSON(current);
		form.setValue("inference_profile_arn_map", formatted);
	};

	return (
		<div className="grid grid-cols-1 md:grid-cols-3 gap-6">
			<FormField
				control={form.control}
				name="priority"
				render={({ field }) => (
					<FormItem>
						<LabelWithHelp
							label={tr("priority.label", "Priority")}
							help={tr(
								"priority.help",
								"Higher priority channels are tried first. Default is 0.",
							)}
						/>
						<FormControl>
							<Input
								type="number"
								className={errorClass("priority")}
								{...field}
							/>
						</FormControl>
						<FormMessage />
					</FormItem>
				)}
			/>

			<FormField
				control={form.control}
				name="weight"
				render={({ field }) => (
					<FormItem>
						<LabelWithHelp
							label={tr("weight.label", "Weight")}
							help={tr(
								"weight.help",
								"Used for load balancing between channels of the same priority. Default is 0.",
							)}
						/>
						<FormControl>
							<Input
								type="number"
								className={errorClass("weight")}
								{...field}
							/>
						</FormControl>
						<FormMessage />
					</FormItem>
				)}
			/>

			<FormField
				control={form.control}
				name="ratelimit"
				render={({ field }) => (
					<FormItem>
						<LabelWithHelp
							label={tr("ratelimit.label", "Rate Limit")}
							help={tr(
								"ratelimit.help",
								"Maximum requests per minute. 0 means unlimited.",
							)}
						/>
						<FormControl>
							<Input
								type="number"
								min="0"
								className={errorClass("ratelimit")}
								{...field}
							/>
						</FormControl>
						<FormMessage />
					</FormItem>
				)}
			/>

			<div className="col-span-1 md:col-span-3">
				<FormField
					control={form.control}
					name="system_prompt"
					render={({ field }) => (
						<FormItem>
							<LabelWithHelp
								label={tr("system_prompt.label", "System Prompt")}
								help={tr(
									"system_prompt.help",
									"Force a system prompt for all requests to this channel.",
								)}
							/>
							<FormControl>
								<Textarea
									placeholder={tr(
										"system_prompt.placeholder",
										"You are a helpful assistant...",
									)}
									className={`min-h-[100px] ${errorClass("system_prompt")}`}
									{...field}
									value={field.value || ""}
								/>
							</FormControl>
							<FormMessage />
						</FormItem>
					)}
				/>
			</div>

			{/* Inference Profile ARN Map - Only show for AWS Bedrock channel type */}
			{normalizedChannelType === AWS_BEDROCK_CHANNEL_TYPE && (
				<div className="col-span-1 md:col-span-3">
					<FormField
						control={form.control}
						name="inference_profile_arn_map"
						render={({ field }) => (
							<FormItem>
								<div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2">
									<LabelWithHelp
										label={tr(
											"inference_profile.label",
											"Inference Profile ARN Map (AWS Bedrock)",
										)}
										help={tr(
											"inference_profile.help",
											"Map model names to AWS Inference Profile ARNs (JSON).",
										)}
									/>
									<Button
										type="button"
										variant="ghost"
										size="sm"
										className="h-6 text-xs self-start sm:self-auto"
										onClick={formatInferenceProfileArnMap}
									>
										{tr("common.format_json", "Format JSON")}
									</Button>
								</div>
								<FormControl>
									<Textarea
										placeholder={tr(
											"inference_profile.placeholder",
											'{"anthropic.claude-3-5-sonnet-20240620-v1:0": "arn:aws:bedrock:..."}',
											{
												example: JSON.stringify(
													INFERENCE_PROFILE_ARN_MAP_EXAMPLE,
													null,
													2,
												),
											},
										)}
										className={`font-mono text-xs min-h-[100px] ${errorClass("inference_profile_arn_map")}`}
										{...field}
										value={field.value || ""}
									/>
								</FormControl>
								<FormMessage />
							</FormItem>
						)}
					/>
				</div>
			)}
		</div>
	);
};
