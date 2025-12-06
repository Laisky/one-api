import {
  FormControl,
  FormField,
  FormItem,
  FormMessage,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { AlertCircle, Lock } from "lucide-react";
import { Controller, type UseFormReturn } from "react-hook-form";
import {
  COZE_AUTH_OPTIONS,
  OAUTH_JWT_CONFIG_EXAMPLE,
  OPENAI_COMPATIBLE_API_FORMAT_OPTIONS,
} from "../constants";
import type { ChannelForm } from "../schemas";
import { LabelWithHelp } from "./LabelWithHelp";

interface ChannelSpecificConfigProps {
  form: UseFormReturn<ChannelForm>;
  normalizedChannelType: number | null;
  defaultBaseURL: string;
  baseURLEditable: boolean;
  tr: (
    key: string,
    defaultValue: string,
    options?: Record<string, unknown>
  ) => string;
}

// Channel types that have their own dedicated base_url field in the channel-specific config
// These should not show the common base URL field
const CHANNEL_TYPES_WITH_INTERNAL_BASE_URL = new Set<number>([3, 50, 52]);

export const ChannelSpecificConfig = ({
  form,
  normalizedChannelType,
  defaultBaseURL,
  baseURLEditable,
  tr,
}: ChannelSpecificConfigProps) => {
  const watchConfig = form.watch("config");
  const _watchType = form.watch("type");

  const fieldHasError = (name: string) =>
    !!(form.formState.errors as any)?.[name];
  const errorClass = (name: string) =>
    fieldHasError(name)
      ? "border-destructive focus-visible:ring-destructive"
      : "";

  // Common base URL field - shown for all channel types except those with internal base_url
  const showCommonBaseURL =
    normalizedChannelType !== null &&
    !CHANNEL_TYPES_WITH_INTERNAL_BASE_URL.has(normalizedChannelType);

  const commonBaseURLField = showCommonBaseURL ? (
    <FormField
      control={form.control}
      name="base_url"
      render={({ field }) => (
        <FormItem>
          <div className="flex items-center gap-2">
            <LabelWithHelp
              label={tr("common.base_url.label", "API Base URL")}
              help={
                baseURLEditable
                  ? tr(
                      "common.base_url.help_editable",
                      "Custom API base URL. Leave empty to use the default URL."
                    )
                  : tr(
                      "common.base_url.help_readonly",
                      "The API base URL for this channel type is fixed and cannot be modified."
                    )
              }
            />
            {!baseURLEditable && (
              <Lock className="h-3 w-3 text-muted-foreground" />
            )}
          </div>
          <FormControl>
            <Input
              placeholder={
                defaultBaseURL ||
                tr("common.base_url.placeholder", "https://api.example.com")
              }
              className={`${errorClass("base_url")} ${!baseURLEditable ? "bg-muted cursor-not-allowed" : ""}`}
              disabled={!baseURLEditable}
              readOnly={!baseURLEditable}
              {...field}
              value={
                baseURLEditable ? field.value : defaultBaseURL || field.value
              }
            />
          </FormControl>
          {!baseURLEditable && defaultBaseURL && (
            <span className="text-xs text-muted-foreground">
              {tr("common.base_url.fixed_note", "Using default: {{url}}", {
                url: defaultBaseURL,
              })}
            </span>
          )}
          <FormMessage />
        </FormItem>
      )}
    />
  ) : null;

  // Render channel-specific configuration
  const renderChannelSpecificConfig = () => {
    switch (normalizedChannelType) {
      case 3: // Azure OpenAI
        return (
          <div className="space-y-4 p-4 border rounded-lg bg-blue-50/50">
            <h4 className="font-medium text-blue-900">
              {tr("azure.heading", "Azure OpenAI Configuration")}
            </h4>
            <FormField
              control={form.control}
              name="base_url"
              render={({ field }) => (
                <FormItem>
                  <LabelWithHelp
                    label={tr(
                      "azure.endpoint.label",
                      "Azure OpenAI Endpoint *"
                    )}
                    help={tr(
                      "azure.endpoint.help",
                      "Your resource endpoint, e.g., https://your-resource.openai.azure.com"
                    )}
                  />
                  <FormControl>
                    <Input
                      placeholder={
                        defaultBaseURL ||
                        tr(
                          "azure.endpoint.placeholder",
                          "https://your-resource.openai.azure.com"
                        )
                      }
                      className={errorClass("base_url")}
                      required
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="other"
              render={({ field }) => (
                <FormItem>
                  <LabelWithHelp
                    label={tr("azure.version.label", "API Version")}
                    help={tr(
                      "azure.version.help",
                      "Default API version used when the request does not specify one (e.g., 2024-03-01-preview)."
                    )}
                  />
                  <FormControl>
                    <Input
                      placeholder={tr(
                        "azure.version.placeholder",
                        "2024-03-01-preview"
                      )}
                      className={errorClass("other")}
                      {...field}
                    />
                  </FormControl>
                  <span className="text-xs text-muted-foreground">
                    {tr(
                      "azure.version.note",
                      "Default: 2024-03-01-preview. This can be overridden by request query parameters."
                    )}
                  </span>
                  <FormMessage />
                </FormItem>
              )}
            />
            <div className="p-3 bg-yellow-50 border border-yellow-200 rounded-lg">
              <div className="flex items-center gap-2">
                <AlertCircle className="h-4 w-4 text-yellow-600" />
                <span className="text-sm text-yellow-800">
                  <strong>
                    {tr("azure.version.warning_label", "Important:")}
                  </strong>{" "}
                  {tr(
                    "azure.version.warning_text",
                    "The model name should be your deployment name, not the original model name."
                  )}
                </span>
              </div>
            </div>
          </div>
        );

      case 33: // AWS Bedrock
        return (
          <div className="space-y-4 p-4 border rounded-lg bg-orange-50/50">
            <h4 className="font-medium text-orange-900">
              {tr("aws.heading", "AWS Bedrock Configuration")}
            </h4>
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              <FormField
                control={form.control}
                name="config.region"
                render={({ field }) => (
                  <FormItem>
                    <LabelWithHelp
                      label={tr("aws.region.label", "Region *")}
                      help={tr(
                        "aws.region.help",
                        "AWS region for Bedrock (e.g., us-east-1). Must match where your models/profiles reside."
                      )}
                    />
                    <FormControl>
                      <Input
                        placeholder={tr("aws.region.placeholder", "us-east-1")}
                        className={errorClass("config.region")}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="config.ak"
                render={({ field }) => (
                  <FormItem>
                    <LabelWithHelp
                      label={tr("aws.ak.label", "Access Key *")}
                      help={tr(
                        "aws.ak.help",
                        "AWS Access Key ID with permissions to call Bedrock."
                      )}
                    />
                    <FormControl>
                      <Input
                        placeholder={tr("aws.ak.placeholder", "AKIA...")}
                        className={errorClass("config.ak")}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="config.sk"
                render={({ field }) => (
                  <FormItem>
                    <LabelWithHelp
                      label={tr("aws.sk.label", "Secret Key *")}
                      help={tr(
                        "aws.sk.help",
                        "AWS Secret Access Key for the above Access Key ID."
                      )}
                    />
                    <FormControl>
                      <Input
                        type="password"
                        placeholder={tr("aws.sk.placeholder", "Secret Key")}
                        className={errorClass("config.sk")}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
            <div className="text-xs text-muted-foreground">
              {tr(
                "aws.note",
                "The final API key will be constructed as: AK|SK|Region"
              )}
            </div>
          </div>
        );

      case 34: // Coze
        return (
          <div className="space-y-4 p-4 border rounded-lg bg-blue-50/50">
            <h4 className="font-medium text-blue-900">
              {tr("coze.heading", "Coze Configuration")}
            </h4>
            <Controller
              name="config.auth_type"
              control={form.control}
              render={({ field }) => (
                <FormItem>
                  <LabelWithHelp
                    label={tr("coze.auth_type.label", "Authentication Type")}
                    help={tr(
                      "coze.auth_type.help",
                      "Choose how to authenticate to Coze: Personal Access Token or OAuth JWT."
                    )}
                  />
                  <Select
                    value={field.value ?? ""}
                    onValueChange={(v) => field.onChange(v)}
                  >
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue
                          placeholder={tr(
                            "coze.auth_type.placeholder",
                            "Select authentication type"
                          )}
                        />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      {COZE_AUTH_OPTIONS.map((option) => (
                        <SelectItem key={option.value} value={option.value}>
                          {option.text}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <FormMessage />
                </FormItem>
              )}
            />
            {watchConfig.auth_type === "personal_access_token" ? (
              <FormField
                control={form.control}
                name="key"
                render={({ field }) => (
                  <FormItem>
                    <LabelWithHelp
                      label={tr("coze.pat.label", "Personal Access Token *")}
                      help={tr(
                        "coze.pat.help",
                        "Your Coze Personal Access Token (pat_...)."
                      )}
                    />
                    <FormControl>
                      <Input
                        type="password"
                        placeholder={tr("coze.pat.placeholder", "pat_...")}
                        className={errorClass("key")}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            ) : (
              <FormField
                control={form.control}
                name="key"
                render={({ field }) => (
                  <FormItem>
                    <LabelWithHelp
                      label={tr("coze.jwt.label", "OAuth JWT Configuration *")}
                      help={tr(
                        "coze.jwt.help",
                        "JSON configuration for Coze OAuth JWT: client_type, client_id, coze_www_base, coze_api_base, private_key, public_key_id."
                      )}
                    />
                    <FormControl>
                      <Textarea
                        placeholder={tr(
                          "coze.jwt.placeholder",
                          `OAuth JWT configuration in JSON format:\n${JSON.stringify(OAUTH_JWT_CONFIG_EXAMPLE, null, 2)}`,
                          {
                            example: JSON.stringify(
                              OAUTH_JWT_CONFIG_EXAMPLE,
                              null,
                              2
                            ),
                          }
                        )}
                        className={`font-mono text-sm min-h-[120px] ${errorClass("key")}`}
                        {...field}
                      />
                    </FormControl>
                    <div className="text-xs text-muted-foreground">
                      {tr(
                        "coze.jwt.required",
                        "Required fields: client_type, client_id, coze_www_base, coze_api_base, private_key, public_key_id"
                      )}
                    </div>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}
            <FormField
              control={form.control}
              name="config.user_id"
              render={({ field }) => (
                <FormItem>
                  <LabelWithHelp
                    label={tr("coze.user.label", "User ID")}
                    help={tr(
                      "coze.user.help",
                      "Optional Coze user ID used for bot operations (if required by your setup)."
                    )}
                  />
                  <FormControl>
                    <Input
                      placeholder={tr(
                        "coze.user.placeholder",
                        "User ID for bot operations"
                      )}
                      className={errorClass("config.user_id")}
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>
        );

      case 42: // Vertex AI
        return (
          <div className="space-y-4 p-4 border rounded-lg bg-green-50/50">
            <h4 className="font-medium text-green-900">
              {tr("vertex.heading", "Vertex AI Configuration")}
            </h4>
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              <FormField
                control={form.control}
                name="config.region"
                render={({ field }) => (
                  <FormItem>
                    <LabelWithHelp
                      label={tr("vertex.region.label", "Region *")}
                      help={tr(
                        "vertex.region.help",
                        "Google Cloud region for Vertex AI (e.g., us-central1)."
                      )}
                    />
                    <FormControl>
                      <Input
                        placeholder={tr(
                          "vertex.region.placeholder",
                          "us-central1"
                        )}
                        className={errorClass("config.region")}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="config.vertex_ai_project_id"
                render={({ field }) => (
                  <FormItem>
                    <LabelWithHelp
                      label={tr("vertex.project.label", "Project ID *")}
                      help={tr(
                        "vertex.project.help",
                        "Your GCP Project ID hosting Vertex AI resources."
                      )}
                    />
                    <FormControl>
                      <Input
                        placeholder={tr(
                          "vertex.project.placeholder",
                          "my-project-id"
                        )}
                        className={errorClass("config.vertex_ai_project_id")}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="config.vertex_ai_adc"
                render={({ field }) => (
                  <FormItem>
                    <LabelWithHelp
                      label={tr(
                        "vertex.credentials.label",
                        "Service Account Credentials *"
                      )}
                      help={tr(
                        "vertex.credentials.help",
                        "Paste the JSON of a service account with Vertex AI permissions."
                      )}
                    />
                    <FormControl>
                      <Textarea
                        placeholder={tr(
                          "vertex.credentials.placeholder",
                          "Google service account JSON credentials"
                        )}
                        className={`font-mono text-xs ${errorClass("config.vertex_ai_adc")}`}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
          </div>
        );

      case 18: // iFlytek Spark
        return (
          <Controller
            name="other"
            control={form.control}
            render={({ field }) => (
              <FormItem>
                <LabelWithHelp
                  label={tr("spark.version.label", "Spark Version")}
                  help={tr(
                    "spark.version.help",
                    "Select the API version for iFlytek Spark (e.g., v3.5)."
                  )}
                />
                <Select
                  value={field.value ?? ""}
                  onValueChange={(v) => field.onChange(v)}
                >
                  <FormControl>
                    <SelectTrigger>
                      <SelectValue
                        placeholder={tr(
                          "spark.version.placeholder",
                          "Select Spark version"
                        )}
                      />
                    </SelectTrigger>
                  </FormControl>
                  <SelectContent>
                    <SelectItem value="v1.1">v1.1</SelectItem>
                    <SelectItem value="v2.1">v2.1</SelectItem>
                    <SelectItem value="v3.1">v3.1</SelectItem>
                    <SelectItem value="v3.5">v3.5</SelectItem>
                  </SelectContent>
                </Select>
                <FormMessage />
              </FormItem>
            )}
          />
        );

      case 21: // Knowledge Base: AI Proxy
        return (
          <FormField
            control={form.control}
            name="other"
            render={({ field }) => (
              <FormItem>
                <LabelWithHelp
                  label={tr("ai_proxy.knowledge.label", "Knowledge ID")}
                  help={tr(
                    "ai_proxy.knowledge.help",
                    "Knowledge base identifier for AI Proxy knowledge retrieval."
                  )}
                />
                <FormControl>
                  <Input
                    placeholder={tr(
                      "ai_proxy.knowledge.placeholder",
                      "Knowledge base ID"
                    )}
                    {...field}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        );

      case 17: // Plugin
        return (
          <FormField
            control={form.control}
            name="other"
            render={({ field }) => (
              <FormItem>
                <LabelWithHelp
                  label={tr("plugin.params.label", "Plugin Parameters")}
                  help={tr(
                    "plugin.params.help",
                    "Provider/plugin-specific parameters if required."
                  )}
                />
                <FormControl>
                  <Input
                    placeholder={tr(
                      "plugin.params.placeholder",
                      "Plugin-specific parameters"
                    )}
                    {...field}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        );

      case 37: // Cloudflare
        return (
          <FormField
            control={form.control}
            name="config.user_id"
            render={({ field }) => (
              <FormItem>
                <LabelWithHelp
                  label={tr("cloudflare.account.label", "Account ID")}
                  help={tr(
                    "cloudflare.account.help",
                    "Your Cloudflare account ID for the AI gateway."
                  )}
                />
                <FormControl>
                  <Input
                    placeholder={tr(
                      "cloudflare.account.placeholder",
                      "d8d7c61dbc334c32d3ced580e4bf42b4"
                    )}
                    {...field}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        );

      case 50: // OpenAI Compatible
        return (
          <div className="space-y-4 p-4 border rounded-lg bg-purple-50/50">
            <h4 className="font-medium text-purple-900">
              {tr(
                "openai_compatible.heading",
                "OpenAI Compatible Configuration"
              )}
            </h4>
            <FormField
              control={form.control}
              name="base_url"
              render={({ field }) => (
                <FormItem>
                  <LabelWithHelp
                    label={tr("openai_compatible.base_url.label", "Base URL *")}
                    help={tr(
                      "openai_compatible.base_url.help",
                      "Base URL of the OpenAI-compatible endpoint, e.g., https://api.your-provider.com. /v1 is appended automatically when required."
                    )}
                  />
                  <FormControl>
                    <Input
                      placeholder={
                        defaultBaseURL ||
                        tr(
                          "openai_compatible.base_url.placeholder",
                          "https://api.your-provider.com"
                        )
                      }
                      className={errorClass("base_url")}
                      required
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="config.api_format"
              render={({ field }) => (
                <FormItem>
                  <LabelWithHelp
                    label={tr(
                      "openai_compatible.api_format.label",
                      "Upstream API Format *"
                    )}
                    help={tr(
                      "openai_compatible.api_format.help",
                      "Select which upstream API surface should handle requests. ChatCompletion is the historical default; choose Response when the upstream expects OpenAI Response API payloads."
                    )}
                  />
                  <FormControl>
                    <Select
                      value={field.value ?? "chat_completion"}
                      onValueChange={field.onChange}
                    >
                      <SelectTrigger>
                        <SelectValue
                          placeholder={tr(
                            "openai_compatible.api_format.placeholder",
                            "Select upstream API format"
                          )}
                        />
                      </SelectTrigger>
                      <SelectContent>
                        {OPENAI_COMPATIBLE_API_FORMAT_OPTIONS.map((option) => (
                          <SelectItem key={option.value} value={option.value}>
                            {tr(
                              `openai_compatible.api_format.option.${option.value}`,
                              option.label
                            )}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>
        );

      case 52: // Claude Compatible
        return (
          <div className="space-y-4 p-4 border rounded-lg bg-amber-50/50">
            <h4 className="font-medium text-amber-900">
              {tr(
                "claude_compatible.heading",
                "Claude Compatible Configuration"
              )}
            </h4>
            <FormField
              control={form.control}
              name="base_url"
              render={({ field }) => (
                <FormItem>
                  <LabelWithHelp
                    label={tr("claude_compatible.base_url.label", "Base URL *")}
                    help={tr(
                      "claude_compatible.base_url.help",
                      "Base URL of the Claude Messages compatible endpoint, e.g., https://api.your-claude-provider.com."
                    )}
                  />
                  <FormControl>
                    <Input
                      placeholder={
                        defaultBaseURL ||
                        tr(
                          "claude_compatible.base_url.placeholder",
                          "https://api.your-claude-provider.com"
                        )
                      }
                      className={errorClass("base_url")}
                      required
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>
        );

      default:
        return null;
    }
  };

  const channelSpecificConfig = renderChannelSpecificConfig();

  // Return both the common base URL field and any channel-specific config
  if (!commonBaseURLField && !channelSpecificConfig) {
    return null;
  }

  return (
    <div className="space-y-4">
      {commonBaseURLField}
      {channelSpecificConfig}
    </div>
  );
};
